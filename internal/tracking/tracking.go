package tracking

// TrackingService provides comprehensive tracking with geolocation and anti-bot
type TrackingService struct {
	Geo     *GeoService
	AntiBot *AntiBotService
}

// NewTrackingService creates a new tracking service
func NewTrackingService() *TrackingService {
	geoService := NewGeoService()
	antiBotService := NewAntiBotService(geoService)

	return &TrackingService{
		Geo:     geoService,
		AntiBot: antiBotService,
	}
}

// TrackingInfo contains all tracking information for an event
type TrackingInfo struct {
	// Geolocation
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	ISP         string  `json:"isp"`

	// Device info
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS             string `json:"os"`
	OSVersion      string `json:"os_version"`
	Device         string `json:"device"`
	DeviceType     string `json:"device_type"`
	EmailClient    string `json:"email_client"`

	// Bot detection
	IsBot        bool   `json:"is_bot"`
	IsSuspicious bool   `json:"is_suspicious"`
	BotType      string `json:"bot_type"`
	BotName      string `json:"bot_name"`
	BotScore     int    `json:"bot_score"`
}

// Analyze performs full analysis on a request
func (t *TrackingService) Analyze(ip, userAgent string) *TrackingInfo {
	info := &TrackingInfo{}

	// Get geolocation
	geoInfo := t.Geo.Lookup(ip)
	if geoInfo != nil && geoInfo.Status == "success" {
		info.Country = geoInfo.Country
		info.CountryCode = geoInfo.CountryCode
		info.Region = geoInfo.RegionName
		info.City = geoInfo.City
		info.Lat = geoInfo.Lat
		info.Lon = geoInfo.Lon
		info.Timezone = geoInfo.Timezone
		info.ISP = geoInfo.ISP
	}

	// Parse user agent
	deviceInfo := ParseUserAgent(userAgent)
	if deviceInfo != nil {
		info.Browser = deviceInfo.Browser
		info.BrowserVersion = deviceInfo.BrowserVersion
		info.OS = deviceInfo.OS
		info.OSVersion = deviceInfo.OSVersion
		info.Device = deviceInfo.Device
		info.DeviceType = deviceInfo.DeviceType
		info.EmailClient = deviceInfo.EmailClient
		info.IsBot = deviceInfo.IsBot
	}

	// Anti-bot analysis
	botResult := t.AntiBot.Analyze(ip, userAgent, geoInfo)
	if botResult != nil {
		// Update bot status from comprehensive analysis
		info.IsBot = botResult.IsBot
		info.IsSuspicious = botResult.IsSuspicious
		info.BotType = botResult.BotType
		info.BotName = botResult.BotName
		info.BotScore = botResult.RiskScore
	}

	return info
}

// IsRealOpen checks if this is a real email open (not a bot)
func (t *TrackingService) IsRealOpen(ip, userAgent string) bool {
	geoInfo := t.Geo.Lookup(ip)
	return t.AntiBot.IsRealOpen(ip, userAgent, geoInfo)
}

// GetBotScore returns just the bot risk score
func (t *TrackingService) GetBotScore(ip, userAgent string) int {
	geoInfo := t.Geo.Lookup(ip)
	return t.AntiBot.GetBotScore(ip, userAgent, geoInfo)
}
