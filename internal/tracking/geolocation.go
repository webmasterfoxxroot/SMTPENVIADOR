package tracking

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// GeoInfo contains geolocation information
type GeoInfo struct {
	IP          string  `json:"ip"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	RegionName  string  `json:"region_name"`
	City        string  `json:"city"`
	Zip         string  `json:"zip"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`
	Org         string  `json:"org"`
	AS          string  `json:"as"`
	Mobile      bool    `json:"mobile"`
	Proxy       bool    `json:"proxy"`
	Hosting     bool    `json:"hosting"`
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	Source      string  `json:"source"` // Which API provided the data
}

// geoCache stores cached results with expiration
type geoCache struct {
	info      *GeoInfo
	expiresAt time.Time
}

// GeoService handles IP geolocation with multiple free APIs
type GeoService struct {
	cache         map[string]*geoCache
	cacheMu       sync.RWMutex
	httpClient    *http.Client
	cacheTTL      time.Duration
	currentAPI    int
	apiMu         sync.Mutex
	lastAPISwitch time.Time
}

// NewGeoService creates a new geolocation service
func NewGeoService() *GeoService {
	gs := &GeoService{
		cache: make(map[string]*geoCache),
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
		cacheTTL:   24 * time.Hour, // Cache for 24 hours
		currentAPI: 0,
	}

	// Start cache cleanup goroutine
	go gs.cleanupCache()

	return gs
}

// cleanupCache periodically removes expired entries
func (g *GeoService) cleanupCache() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		g.cacheMu.Lock()
		now := time.Now()
		for ip, cached := range g.cache {
			if now.After(cached.expiresAt) {
				delete(g.cache, ip)
			}
		}
		g.cacheMu.Unlock()
	}
}

// Lookup gets geolocation for an IP address using multiple free APIs
func (g *GeoService) Lookup(ip string) *GeoInfo {
	// Clean IP
	ip = cleanIP(ip)

	// Check if private/local IP
	if isPrivateIP(ip) {
		return &GeoInfo{
			IP:      ip,
			Country: "Local",
			City:    "Local Network",
			Status:  "success",
			Source:  "local",
		}
	}

	// Check cache
	g.cacheMu.RLock()
	if cached, ok := g.cache[ip]; ok {
		if time.Now().Before(cached.expiresAt) {
			g.cacheMu.RUnlock()
			return cached.info
		}
	}
	g.cacheMu.RUnlock()

	// Try multiple APIs with fallback
	apis := []func(string) *GeoInfo{
		g.lookupIPWhoIs,   // ipwho.is - sem limite oficial, muito generoso
		g.lookupIPAPI,     // ip-api.com - 45/min
		g.lookupIPAPIIs,   // ipapi.is - generoso, sem limite claro
		g.lookupFreeIPAPI, // freeipapi.com - sem limite claro
	}

	var info *GeoInfo
	for _, apiFunc := range apis {
		info = apiFunc(ip)
		if info != nil && info.Status == "success" {
			break
		}
		// Small delay between API calls to be nice
		time.Sleep(100 * time.Millisecond)
	}

	if info == nil {
		info = &GeoInfo{IP: ip, Status: "fail", Message: "All APIs failed"}
	}

	// Cache result
	if info.Status == "success" {
		g.cacheMu.Lock()
		g.cache[ip] = &geoCache{
			info:      info,
			expiresAt: time.Now().Add(g.cacheTTL),
		}
		// Limit cache size to 50k entries
		if len(g.cache) > 50000 {
			count := 0
			for k := range g.cache {
				delete(g.cache, k)
				count++
				if count >= 10000 {
					break
				}
			}
		}
		g.cacheMu.Unlock()
	}

	return info
}

// lookupIPWhoIs uses ipwho.is API (very generous limits)
func (g *GeoService) lookupIPWhoIs(ip string) *GeoInfo {
	url := fmt.Sprintf("https://ipwho.is/%s", ip)

	resp, err := g.httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var data struct {
		IP          string  `json:"ip"`
		Success     bool    `json:"success"`
		Country     string  `json:"country"`
		CountryCode string  `json:"country_code"`
		Region      string  `json:"region"`
		RegionCode  string  `json:"region_code"`
		City        string  `json:"city"`
		Postal      string  `json:"postal"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Timezone    struct {
			ID string `json:"id"`
		} `json:"timezone"`
		Connection struct {
			ASN    int    `json:"asn"`
			Org    string `json:"org"`
			ISP    string `json:"isp"`
			Domain string `json:"domain"`
		} `json:"connection"`
		Security struct {
			Anonymous bool `json:"anonymous"`
			Proxy     bool `json:"proxy"`
			VPN       bool `json:"vpn"`
			Tor       bool `json:"tor"`
			Hosting   bool `json:"hosting"`
		} `json:"security"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}

	if !data.Success {
		return nil
	}

	return &GeoInfo{
		IP:          ip,
		Country:     data.Country,
		CountryCode: data.CountryCode,
		Region:      data.RegionCode,
		RegionName:  data.Region,
		City:        data.City,
		Zip:         data.Postal,
		Lat:         data.Latitude,
		Lon:         data.Longitude,
		Timezone:    data.Timezone.ID,
		ISP:         data.Connection.ISP,
		Org:         data.Connection.Org,
		AS:          fmt.Sprintf("AS%d", data.Connection.ASN),
		Proxy:       data.Security.Proxy || data.Security.VPN || data.Security.Tor,
		Hosting:     data.Security.Hosting,
		Status:      "success",
		Source:      "ipwho.is",
	}
}

// lookupIPAPI uses ip-api.com (45 requests/minute free)
func (g *GeoService) lookupIPAPI(ip string) *GeoInfo {
	// fields=66846719 includes all fields including proxy/hosting detection
	url := fmt.Sprintf("http://ip-api.com/json/%s?fields=66846719", ip)

	resp, err := g.httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var data struct {
		Status      string  `json:"status"`
		Message     string  `json:"message"`
		Country     string  `json:"country"`
		CountryCode string  `json:"countryCode"`
		Region      string  `json:"region"`
		RegionName  string  `json:"regionName"`
		City        string  `json:"city"`
		Zip         string  `json:"zip"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		Timezone    string  `json:"timezone"`
		ISP         string  `json:"isp"`
		Org         string  `json:"org"`
		AS          string  `json:"as"`
		Mobile      bool    `json:"mobile"`
		Proxy       bool    `json:"proxy"`
		Hosting     bool    `json:"hosting"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}

	if data.Status != "success" {
		return nil
	}

	return &GeoInfo{
		IP:          ip,
		Country:     data.Country,
		CountryCode: data.CountryCode,
		Region:      data.Region,
		RegionName:  data.RegionName,
		City:        data.City,
		Zip:         data.Zip,
		Lat:         data.Lat,
		Lon:         data.Lon,
		Timezone:    data.Timezone,
		ISP:         data.ISP,
		Org:         data.Org,
		AS:          data.AS,
		Mobile:      data.Mobile,
		Proxy:       data.Proxy,
		Hosting:     data.Hosting,
		Status:      "success",
		Source:      "ip-api.com",
	}
}

// lookupIPAPIIs uses ipapi.is (generous limits)
func (g *GeoService) lookupIPAPIIs(ip string) *GeoInfo {
	url := fmt.Sprintf("https://api.ipapi.is/?q=%s", ip)

	resp, err := g.httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var data struct {
		IP       string `json:"ip"`
		Location struct {
			Country     string  `json:"country"`
			CountryCode string  `json:"country_code"`
			State       string  `json:"state"`
			City        string  `json:"city"`
			Postal      string  `json:"postal"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
			Timezone    string  `json:"timezone"`
		} `json:"location"`
		ASN struct {
			ASN    int    `json:"asn"`
			Org    string `json:"org"`
			ISP    string `json:"isp"`
			Domain string `json:"domain"`
		} `json:"asn"`
		Company struct {
			Name   string `json:"name"`
			Domain string `json:"domain"`
			Type   string `json:"type"`
		} `json:"company"`
		IsDatacenter bool `json:"is_datacenter"`
		IsProxy      bool `json:"is_proxy"`
		IsVPN        bool `json:"is_vpn"`
		IsTor        bool `json:"is_tor"`
		IsMobile     bool `json:"is_mobile"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}

	if data.IP == "" {
		return nil
	}

	return &GeoInfo{
		IP:          ip,
		Country:     data.Location.Country,
		CountryCode: data.Location.CountryCode,
		Region:      "",
		RegionName:  data.Location.State,
		City:        data.Location.City,
		Zip:         data.Location.Postal,
		Lat:         data.Location.Latitude,
		Lon:         data.Location.Longitude,
		Timezone:    data.Location.Timezone,
		ISP:         data.ASN.ISP,
		Org:         data.ASN.Org,
		AS:          fmt.Sprintf("AS%d", data.ASN.ASN),
		Mobile:      data.IsMobile,
		Proxy:       data.IsProxy || data.IsVPN || data.IsTor,
		Hosting:     data.IsDatacenter,
		Status:      "success",
		Source:      "ipapi.is",
	}
}

// lookupFreeIPAPI uses freeipapi.com (generous limits)
func (g *GeoService) lookupFreeIPAPI(ip string) *GeoInfo {
	url := fmt.Sprintf("https://freeipapi.com/api/json/%s", ip)

	resp, err := g.httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var data struct {
		IPVersion   int     `json:"ipVersion"`
		IPAddress   string  `json:"ipAddress"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		CountryName string  `json:"countryName"`
		CountryCode string  `json:"countryCode"`
		TimeZone    string  `json:"timeZone"`
		ZipCode     string  `json:"zipCode"`
		CityName    string  `json:"cityName"`
		RegionName  string  `json:"regionName"`
		IsProxy     bool    `json:"isProxy"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil
	}

	if data.IPAddress == "" {
		return nil
	}

	return &GeoInfo{
		IP:          ip,
		Country:     data.CountryName,
		CountryCode: data.CountryCode,
		RegionName:  data.RegionName,
		City:        data.CityName,
		Zip:         data.ZipCode,
		Lat:         data.Latitude,
		Lon:         data.Longitude,
		Timezone:    data.TimeZone,
		Proxy:       data.IsProxy,
		Status:      "success",
		Source:      "freeipapi.com",
	}
}

// LookupAsync performs async geolocation lookup
func (g *GeoService) LookupAsync(ip string, callback func(*GeoInfo)) {
	go func() {
		info := g.Lookup(ip)
		if callback != nil {
			callback(info)
		}
	}()
}

// cleanIP extracts clean IP from possible X-Forwarded-For format
func cleanIP(ip string) string {
	// Handle X-Forwarded-For format (comma-separated)
	if strings.Contains(ip, ",") {
		parts := strings.Split(ip, ",")
		ip = strings.TrimSpace(parts[0])
	}

	// Handle IPv6 with port
	if strings.HasPrefix(ip, "[") {
		if idx := strings.Index(ip, "]:"); idx != -1 {
			ip = ip[1:idx]
		}
	}

	// Handle IPv4 with port
	if strings.Contains(ip, ":") && !strings.Contains(ip, "::") {
		ip = strings.Split(ip, ":")[0]
	}

	return strings.TrimSpace(ip)
}

// isPrivateIP checks if IP is private/local
func isPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check for loopback
	if parsedIP.IsLoopback() {
		return true
	}

	// Check for private ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fd00::/8",
		"fe80::/10",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			return true
		}
	}

	return false
}

// IsDatacenterIP checks if IP belongs to known datacenter/cloud providers
func (g *GeoService) IsDatacenterIP(info *GeoInfo) bool {
	if info == nil {
		return false
	}

	// API already provides hosting flag
	if info.Hosting {
		return true
	}

	// Check known datacenter ASNs and organizations
	datacenterKeywords := []string{
		"amazon", "aws", "google", "microsoft", "azure", "digitalocean",
		"linode", "vultr", "ovh", "hetzner", "cloudflare", "fastly",
		"akamai", "leaseweb", "softlayer", "rackspace", "alibaba",
		"tencent", "oracle cloud", "scaleway", "upcloud", "contabo",
		"hostinger", "hostgator", "godaddy", "bluehost", "namecheap",
	}

	orgLower := strings.ToLower(info.Org)
	ispLower := strings.ToLower(info.ISP)
	asLower := strings.ToLower(info.AS)

	for _, keyword := range datacenterKeywords {
		if strings.Contains(orgLower, keyword) ||
			strings.Contains(ispLower, keyword) ||
			strings.Contains(asLower, keyword) {
			return true
		}
	}

	return false
}

// GetCacheStats returns cache statistics
func (g *GeoService) GetCacheStats() (size int, hitRate float64) {
	g.cacheMu.RLock()
	defer g.cacheMu.RUnlock()
	return len(g.cache), 0 // hitRate would need additional tracking
}
