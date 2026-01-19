package tracking

import (
	"strings"
	"sync"
	"time"
)

// BotDetectionResult contains the result of bot detection analysis
type BotDetectionResult struct {
	IsBot        bool     `json:"is_bot"`
	IsSuspicious bool     `json:"is_suspicious"`
	IsHuman      bool     `json:"is_human"`
	Confidence   float64  `json:"confidence"` // 0-100 confidence score
	BotType      string   `json:"bot_type"`   // type of bot if detected
	BotName      string   `json:"bot_name"`   // name of specific bot
	Reasons      []string `json:"reasons"`    // reasons for classification
	RiskScore    int      `json:"risk_score"` // 0-100, higher = more likely bot
}

// AntiBotService handles advanced bot detection
type AntiBotService struct {
	geoService     *GeoService
	requestHistory map[string]*requestInfo
	historyMu      sync.RWMutex
}

// requestInfo tracks request patterns
type requestInfo struct {
	count      int
	firstSeen  time.Time
	lastSeen   time.Time
	userAgents map[string]int
	endpoints  map[string]int
}

// NewAntiBotService creates a new anti-bot service
func NewAntiBotService(geoService *GeoService) *AntiBotService {
	abs := &AntiBotService{
		geoService:     geoService,
		requestHistory: make(map[string]*requestInfo),
	}

	// Start cleanup goroutine
	go abs.cleanupHistory()

	return abs
}

// cleanupHistory removes old entries periodically
func (a *AntiBotService) cleanupHistory() {
	ticker := time.NewTicker(1 * time.Hour)
	for range ticker.C {
		a.historyMu.Lock()
		cutoff := time.Now().Add(-24 * time.Hour)
		for ip, info := range a.requestHistory {
			if info.lastSeen.Before(cutoff) {
				delete(a.requestHistory, ip)
			}
		}
		a.historyMu.Unlock()
	}
}

// Analyze performs comprehensive bot detection
func (a *AntiBotService) Analyze(ip, userAgent string, geoInfo *GeoInfo) *BotDetectionResult {
	result := &BotDetectionResult{
		Reasons:    make([]string, 0),
		Confidence: 0,
		RiskScore:  0,
	}

	uaLower := strings.ToLower(userAgent)

	// Track request
	a.trackRequest(ip, userAgent)

	// 1. Known Bot User-Agent Detection (most reliable)
	if botType, botName := detectKnownBot(uaLower); botType != "" {
		result.IsBot = true
		result.BotType = botType
		result.BotName = botName
		result.Confidence = 95
		result.RiskScore = 90
		result.Reasons = append(result.Reasons, "Known bot user-agent: "+botName)
		return result
	}

	// 2. Security Scanner Detection
	if isSecurityScanner(uaLower) {
		result.IsBot = true
		result.BotType = "security_scanner"
		result.BotName = "Security Scanner"
		result.Confidence = 90
		result.RiskScore = 85
		result.Reasons = append(result.Reasons, "Security scanner detected")
		return result
	}

	// 3. Email Prefetch/Preview Detection
	if isEmailPrefetch(uaLower) {
		result.IsBot = true
		result.BotType = "email_prefetch"
		result.BotName = "Email Preview Bot"
		result.Confidence = 85
		result.RiskScore = 70
		result.Reasons = append(result.Reasons, "Email prefetch/preview detected")
		return result
	}

	// 4. Headless Browser Detection
	if isHeadlessBrowser(uaLower) {
		result.IsBot = true
		result.BotType = "headless_browser"
		result.BotName = "Headless Browser"
		result.Confidence = 90
		result.RiskScore = 85
		result.Reasons = append(result.Reasons, "Headless browser detected")
		return result
	}

	// 5. HTTP Library Detection
	if isHTTPLibrary(uaLower) {
		result.IsBot = true
		result.BotType = "http_library"
		result.BotName = "HTTP Library"
		result.Confidence = 85
		result.RiskScore = 80
		result.Reasons = append(result.Reasons, "HTTP library user-agent")
		return result
	}

	// 6. Empty or Suspicious User-Agent
	if userAgent == "" {
		result.IsSuspicious = true
		result.RiskScore += 40
		result.Reasons = append(result.Reasons, "Empty user-agent")
	} else if len(userAgent) < 20 {
		result.IsSuspicious = true
		result.RiskScore += 30
		result.Reasons = append(result.Reasons, "Very short user-agent")
	}

	// 7. Datacenter/Hosting IP Detection
	if geoInfo != nil {
		if geoInfo.Hosting {
			result.IsSuspicious = true
			result.RiskScore += 35
			result.Reasons = append(result.Reasons, "Datacenter/hosting IP")
		}

		if geoInfo.Proxy {
			result.IsSuspicious = true
			result.RiskScore += 25
			result.Reasons = append(result.Reasons, "Proxy/VPN detected")
		}

		// Check if GeoService thinks it's datacenter
		if a.geoService != nil && a.geoService.IsDatacenterIP(geoInfo) {
			result.IsSuspicious = true
			result.RiskScore += 30
			result.Reasons = append(result.Reasons, "Known cloud provider IP")
		}
	}

	// 8. Request Pattern Analysis
	patternScore, patternReasons := a.analyzeRequestPattern(ip)
	result.RiskScore += patternScore
	result.Reasons = append(result.Reasons, patternReasons...)

	// 9. User-Agent Anomaly Detection
	anomalyScore, anomalyReasons := detectUAAnomaly(userAgent)
	result.RiskScore += anomalyScore
	result.Reasons = append(result.Reasons, anomalyReasons...)

	// Calculate final classification
	if result.RiskScore >= 70 {
		result.IsBot = true
		result.BotType = "suspicious"
		result.BotName = "Suspicious Activity"
		result.Confidence = float64(result.RiskScore)
	} else if result.RiskScore >= 40 {
		result.IsSuspicious = true
		result.Confidence = float64(result.RiskScore)
	} else {
		result.IsHuman = true
		result.Confidence = float64(100 - result.RiskScore)
	}

	// Cap risk score at 100
	if result.RiskScore > 100 {
		result.RiskScore = 100
	}

	return result
}

// trackRequest records request for pattern analysis
func (a *AntiBotService) trackRequest(ip, userAgent string) {
	a.historyMu.Lock()
	defer a.historyMu.Unlock()

	info, exists := a.requestHistory[ip]
	if !exists {
		info = &requestInfo{
			firstSeen:  time.Now(),
			userAgents: make(map[string]int),
			endpoints:  make(map[string]int),
		}
		a.requestHistory[ip] = info
	}

	info.count++
	info.lastSeen = time.Now()
	info.userAgents[userAgent]++
}

// analyzeRequestPattern detects suspicious request patterns
func (a *AntiBotService) analyzeRequestPattern(ip string) (int, []string) {
	a.historyMu.RLock()
	defer a.historyMu.RUnlock()

	info, exists := a.requestHistory[ip]
	if !exists {
		return 0, nil
	}

	score := 0
	reasons := []string{}

	// High request frequency
	duration := info.lastSeen.Sub(info.firstSeen)
	if duration > 0 && info.count > 10 {
		requestsPerMinute := float64(info.count) / duration.Minutes()
		if requestsPerMinute > 60 {
			score += 40
			reasons = append(reasons, "Very high request rate")
		} else if requestsPerMinute > 30 {
			score += 20
			reasons = append(reasons, "High request rate")
		}
	}

	// Multiple user-agents from same IP (fingerprint rotation)
	if len(info.userAgents) > 5 {
		score += 30
		reasons = append(reasons, "Multiple user-agents from same IP")
	}

	return score, reasons
}

// detectKnownBot checks for known bot user-agents
func detectKnownBot(ua string) (botType, botName string) {
	// Search engine bots
	searchBots := map[string]string{
		"googlebot":    "Googlebot",
		"bingbot":      "Bingbot",
		"yandexbot":    "Yandexbot",
		"duckduckbot":  "DuckDuckBot",
		"baiduspider":  "Baiduspider",
		"sogou":        "Sogou",
		"exabot":       "Exabot",
		"yahoo! slurp": "Yahoo Slurp",
		"ia_archiver":  "Internet Archive",
		"archive.org":  "Internet Archive",
		"applebot":     "Applebot",
		"petalbot":     "PetalBot",
		"seznambot":    "SeznamBot",
		"mail.ru":      "Mail.RU Bot",
		"ahrefsbot":    "AhrefsBot",
		"semrushbot":   "SemrushBot",
		"mj12bot":      "Majestic Bot",
		"dotbot":       "DotBot",
		"rogerbot":     "RogerBot",
	}

	for key, name := range searchBots {
		if strings.Contains(ua, key) {
			return "search_engine", name
		}
	}

	// Social media bots
	socialBots := map[string]string{
		"facebookexternalhit": "Facebook Bot",
		"facebot":             "Facebook Bot",
		"twitterbot":          "Twitter Bot",
		"linkedinbot":         "LinkedIn Bot",
		"pinterest":           "Pinterest Bot",
		"slackbot":            "Slack Bot",
		"telegrambot":         "Telegram Bot",
		"discordbot":          "Discord Bot",
		"whatsapp":            "WhatsApp",
		"skypeuripreview":     "Skype Preview",
		"vkshare":             "VKontakte",
		"viber":               "Viber",
		"line-poker":          "LINE",
	}

	for key, name := range socialBots {
		if strings.Contains(ua, key) {
			return "social_media", name
		}
	}

	// Monitoring bots
	monitorBots := map[string]string{
		"pingdom":         "Pingdom",
		"uptimerobot":     "UptimeRobot",
		"statuscake":      "StatusCake",
		"newrelic":        "New Relic",
		"datadog":         "Datadog",
		"site24x7":        "Site24x7",
		"gtmetrix":        "GTmetrix",
		"pagespeed":       "PageSpeed",
		"lighthouse":      "Lighthouse",
		"webpagetest":     "WebPageTest",
		"insites.com":     "Insites",
		"deadlinkchecker": "Dead Link Checker",
	}

	for key, name := range monitorBots {
		if strings.Contains(ua, key) {
			return "monitoring", name
		}
	}

	return "", ""
}

// isSecurityScanner detects security scanning tools
func isSecurityScanner(ua string) bool {
	scanners := []string{
		// Email security gateways (2024-2025 updated list)
		"barracuda", "proofpoint", "mimecast", "symantec", "mcafee",
		"fortiguard", "websense", "bluecoat", "zscaler", "fireeye",
		"sophos", "kaspersky", "avast", "avg", "bitdefender",
		"trendmicro", "trend micro", "eset", "f-secure", "gdata",
		"panda", "malwarebytes", "webroot", "cylance", "crowdstrike",
		"carbonblack", "sentinelone", "cybereason", "darktrace",

		// Enterprise email security
		"ironport", "cisco ironport", "cisco esa", "fortimail",
		"mailguard", "spamhaus", "spamassassin", "spamexperts",
		"messagelabs", "broadcom", "messagegate", "mailcontrol",

		// Microsoft/Google security
		"microsoft url reputation", "safelinks", "advanced threat",
		"atp", "defender", "google safe browsing", "safebrowsing",

		// Cloud security
		"cloudflare", "incapsula", "imperva", "sucuri", "akamai",
		"cloudfront", "fastly",

		// General scanners
		"nessus", "nikto", "nmap", "burp", "acunetix", "qualys",
		"openvas", "w3af", "sqlmap", "metasploit", "wpscan",
		"nuclei", "httpx", "masscan", "shodan", "censys",
		"zgrab", "zmap", "gobuster", "dirbuster", "feroxbuster",
	}

	for _, scanner := range scanners {
		if strings.Contains(ua, scanner) {
			return true
		}
	}

	return false
}

// isEmailPrefetch detects email preview/prefetch services (NOT email clients)
// Important: We should NOT mark legitimate email clients (Gmail, Outlook, etc) as bots
// We only want to detect automated prefetch/preview mechanisms
func isEmailPrefetch(ua string) bool {
	// Only detect PREFETCH/PROXY services, NOT regular email clients
	prefetchers := []string{
		// Google image proxy (automated prefetch, not user action)
		"googleimageproxy", "google-image-proxy", "google image proxy",
		"googleimages",

		// Yahoo mail proxy (automated prefetch)
		"yahoomailproxy", "yahoo mail proxy", "yahoo-mail-proxy",

		// Microsoft prefetch (link scanner, not user viewing)
		"outlooksafebrowsing", "safelinks.protection",
		"microsoft-azure-atp", "atp.azure.com",

		// Generic link prefetchers (automated scanning)
		"link prefetch", "linkpreview", "link-preview",
		"urlpreview", "url-preview",

		// Email security link scanners (click before user)
		"emailsecurity", "email-security",
		"linkchecker", "link-checker",
	}

	for _, pf := range prefetchers {
		if strings.Contains(ua, pf) {
			return true
		}
	}

	return false
}

// isHeadlessBrowser detects headless browsers
func isHeadlessBrowser(ua string) bool {
	headless := []string{
		// Explicit headless indicators
		"headlesschrome", "headless chrome", "headless",
		"phantomjs", "phantom.js",
		"puppeteer", "playwright", "selenium",
		"webdriver", "chromedriver", "geckodriver",
		"chromium-browser/headless", "chrome/headless",
		"htmlunit", "splinter", "casperjs",
		"nightmare", "zombie.js", "jsdom",
		"slimerjs", "watir", "capybara",
	}

	for _, h := range headless {
		if strings.Contains(ua, h) {
			return true
		}
	}

	// Note: We removed the Chrome without Safari detection as it caused false positives
	// Real headless Chrome includes "HeadlessChrome" in the UA which is already detected above

	return false
}

// isHTTPLibrary detects HTTP client libraries
func isHTTPLibrary(ua string) bool {
	libraries := []string{
		// General
		"curl", "wget", "httpie", "postman", "insomnia",

		// Programming languages
		"python", "python-requests", "python-urllib", "aiohttp",
		"java/", "java http", "apache-httpclient", "okhttp",
		"go-http-client", "go http", "golang",
		"ruby", "faraday", "rest-client", "httparty",
		"perl", "lwp-", "libwww-perl",
		"php/", "guzzle", "curl_",
		"node-fetch", "axios", "got/", "superagent",
		"request/", "needle", "undici",
		"rust", "reqwest", "hyper",
		".net", "httpclient",
		"deno/",

		// Generic patterns
		"http_request", "http-request", "httprequest",
		"api-client", "rest-client", "restclient",
		"client/", "sdk/", "lib/",
	}

	for _, lib := range libraries {
		if strings.Contains(ua, lib) {
			return true
		}
	}

	return false
}

// detectUAAnomaly detects anomalies in user-agent string
func detectUAAnomaly(ua string) (int, []string) {
	score := 0
	reasons := []string{}

	// Very old browser versions
	if strings.Contains(ua, "MSIE 6") || strings.Contains(ua, "MSIE 7") ||
		strings.Contains(ua, "MSIE 8") || strings.Contains(ua, "MSIE 9") {
		score += 30
		reasons = append(reasons, "Very old browser version")
	}

	// Contradictory user-agent
	uaLower := strings.ToLower(ua)
	if strings.Contains(uaLower, "windows") && strings.Contains(uaLower, "mac os") {
		score += 40
		reasons = append(reasons, "Contradictory OS in user-agent")
	}

	// Missing standard UA components
	if !strings.Contains(uaLower, "mozilla") && len(ua) > 10 {
		score += 15
		reasons = append(reasons, "Missing Mozilla component")
	}

	// Generic/fake user-agents
	genericUAs := []string{
		"mozilla/5.0",
		"mozilla/4.0",
		"user-agent",
		"useragent",
		"test",
		"hello",
		"hi",
	}

	if len(ua) < 50 {
		for _, generic := range genericUAs {
			if strings.ToLower(strings.TrimSpace(ua)) == generic {
				score += 35
				reasons = append(reasons, "Generic/test user-agent")
				break
			}
		}
	}

	return score, reasons
}

// IsRealOpen checks if this appears to be a real email open
func (a *AntiBotService) IsRealOpen(ip, userAgent string, geoInfo *GeoInfo) bool {
	result := a.Analyze(ip, userAgent, geoInfo)
	return result.IsHuman || (result.IsSuspicious && result.RiskScore < 50)
}

// GetBotScore returns just the risk score for quick checks
func (a *AntiBotService) GetBotScore(ip, userAgent string, geoInfo *GeoInfo) int {
	result := a.Analyze(ip, userAgent, geoInfo)
	return result.RiskScore
}
