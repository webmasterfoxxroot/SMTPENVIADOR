package tracking

import (
	"regexp"
	"strings"
)

// DeviceInfo contains parsed user agent information
type DeviceInfo struct {
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS             string `json:"os"`
	OSVersion      string `json:"os_version"`
	Device         string `json:"device"`
	DeviceType     string `json:"device_type"` // desktop, mobile, tablet, bot
	IsMobile       bool   `json:"is_mobile"`
	IsTablet       bool   `json:"is_tablet"`
	IsDesktop      bool   `json:"is_desktop"`
	IsBot          bool   `json:"is_bot"`
	EmailClient    string `json:"email_client"`
	Raw            string `json:"raw"`
}

// ParseUserAgent parses a user agent string
func ParseUserAgent(ua string) *DeviceInfo {
	info := &DeviceInfo{
		Raw:        ua,
		DeviceType: "desktop",
		IsDesktop:  true,
	}

	if ua == "" {
		info.Browser = "Unknown"
		info.OS = "Unknown"
		info.Device = "Unknown"
		return info
	}

	uaLower := strings.ToLower(ua)

	// Detect email clients first
	info.EmailClient = detectEmailClient(uaLower)

	// Detect if bot
	info.IsBot = detectBot(uaLower)
	if info.IsBot {
		info.DeviceType = "bot"
		info.IsDesktop = false
		info.Browser = detectBotName(uaLower)
		info.OS = "Bot"
		info.Device = "Bot"
		return info
	}

	// Detect device type
	if detectMobile(uaLower) {
		info.IsMobile = true
		info.IsDesktop = false
		info.DeviceType = "mobile"
	}

	if detectTablet(uaLower) {
		info.IsTablet = true
		info.IsMobile = false
		info.IsDesktop = false
		info.DeviceType = "tablet"
	}

	// Detect OS
	info.OS, info.OSVersion = detectOS(ua)

	// Detect browser
	info.Browser, info.BrowserVersion = detectBrowser(ua)

	// Detect device brand
	info.Device = detectDevice(ua)

	return info
}

// detectEmailClient identifies email client from user agent
func detectEmailClient(ua string) string {
	emailClients := map[string]string{
		"thunderbird":      "Mozilla Thunderbird",
		"outlook":          "Microsoft Outlook",
		"microsoft office": "Microsoft Outlook",
		"windowslivemail":  "Windows Live Mail",
		"apple mail":       "Apple Mail",
		"apple-mail":       "Apple Mail",
		"applemail":        "Apple Mail",
		"airmail":          "Airmail",
		"gmail":            "Gmail",
		"googleimageproxy": "Gmail",
		"yahoo":            "Yahoo Mail",
		"ymail":            "Yahoo Mail",
		"aol":              "AOL Mail",
		"mail.ru":          "Mail.ru",
		"yandex":           "Yandex Mail",
		"mailspring":       "Mailspring",
		"em client":        "eM Client",
		"postbox":          "Postbox",
		"mailmate":         "MailMate",
		"spark":            "Spark",
		"newton":           "Newton Mail",
		"protonmail":       "ProtonMail",
		"tutanota":         "Tutanota",
		"zoho":             "Zoho Mail",
		"fastmail":         "FastMail",
		"roundcube":        "Roundcube",
		"squirrelmail":     "SquirrelMail",
		"horde":            "Horde",
		"zimbra":           "Zimbra",
		"evolution":        "Evolution",
		"claws":            "Claws Mail",
		"mutt":             "Mutt",
		"alpine":           "Alpine",
		"kmail":            "KMail",
		"geary":            "Geary",
		"mailbird":         "Mailbird",
		"foxmail":          "Foxmail",
		"the bat":          "The Bat!",
		"mailpro":          "MailPro",
	}

	for key, name := range emailClients {
		if strings.Contains(ua, key) {
			return name
		}
	}

	return ""
}

// detectBot checks if user agent is a known bot
func detectBot(ua string) bool {
	botPatterns := []string{
		// Search engine bots
		"googlebot", "bingbot", "yandexbot", "duckduckbot", "baiduspider",
		"sogou", "exabot", "facebot", "ia_archiver", "alexabot",

		// Social media bots
		"facebookexternalhit", "twitterbot", "linkedinbot", "pinterest",
		"slackbot", "telegrambot", "whatsapp", "discordbot", "skypeuripreview",

		// Email service bots/prefetchers
		"yahoo! slurp", "msnbot", "googleimageproxy", "yahoomailproxy",
		"outlook-ios", "outlookpreview", "mx.yahoo.com",

		// Security scanners and prefetchers
		"barracuda", "proofpoint", "mimecast", "symantec", "mcafee",
		"fortiguard", "websense", "bluecoat", "zscaler", "fireeye",
		"sophos", "kaspersky", "avast", "avg", "bitdefender",

		// Link prefetchers
		"prefetch", "prerender", "headless", "phantom", "selenium",
		"puppeteer", "playwright", "webdriver",

		// Generic bot patterns
		"bot", "crawler", "spider", "scraper", "curl", "wget", "python",
		"java/", "perl", "ruby", "php/", "go-http-client", "axios",
		"node-fetch", "http_request", "libwww", "lwp-",

		// Monitoring and testing
		"pingdom", "uptimerobot", "statuscake", "newrelic", "datadog",
		"gtmetrix", "pagespeed", "lighthouse", "webpagetest",
	}

	for _, pattern := range botPatterns {
		if strings.Contains(ua, pattern) {
			return true
		}
	}

	return false
}

// detectBotName returns specific bot name
func detectBotName(ua string) string {
	bots := map[string]string{
		"googlebot":           "Googlebot",
		"bingbot":             "Bingbot",
		"yandexbot":           "Yandexbot",
		"duckduckbot":         "DuckDuckBot",
		"facebookexternalhit": "Facebook Bot",
		"twitterbot":          "Twitter Bot",
		"linkedinbot":         "LinkedIn Bot",
		"slackbot":            "Slack Bot",
		"telegrambot":         "Telegram Bot",
		"discordbot":          "Discord Bot",
		"whatsapp":            "WhatsApp",
		"googleimageproxy":    "Gmail Proxy",
		"yahoomailproxy":      "Yahoo Mail Proxy",
		"outlook":             "Outlook Preview",
		"barracuda":           "Barracuda",
		"proofpoint":          "Proofpoint",
		"mimecast":            "Mimecast",
		"curl":                "cURL",
		"wget":                "Wget",
		"python":              "Python",
		"java/":               "Java",
		"go-http-client":      "Go HTTP Client",
		"axios":               "Axios",
		"node-fetch":          "Node Fetch",
		"puppeteer":           "Puppeteer",
		"headless":            "Headless Browser",
		"pingdom":             "Pingdom",
		"uptimerobot":         "UptimeRobot",
	}

	for key, name := range bots {
		if strings.Contains(ua, key) {
			return name
		}
	}

	return "Unknown Bot"
}

// detectMobile checks if device is mobile
func detectMobile(ua string) bool {
	mobilePatterns := []string{
		"mobile", "android", "iphone", "ipod", "blackberry",
		"windows phone", "opera mini", "opera mobi", "iemobile",
		"webos", "palm", "symbian", "nokia", "samsung",
		"lg-", "htc", "mot-", "huawei", "xiaomi", "oppo", "vivo",
	}

	for _, pattern := range mobilePatterns {
		if strings.Contains(ua, pattern) {
			// Exclude tablets
			if strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet") {
				return false
			}
			return true
		}
	}

	return false
}

// detectTablet checks if device is tablet
func detectTablet(ua string) bool {
	tabletPatterns := []string{
		"ipad", "tablet", "kindle", "silk", "playbook",
		"nexus 7", "nexus 9", "nexus 10", "galaxy tab",
		"sm-t", "gt-p", "mediapad",
	}

	for _, pattern := range tabletPatterns {
		if strings.Contains(ua, pattern) {
			return true
		}
	}

	// Android without mobile is usually tablet
	if strings.Contains(ua, "android") && !strings.Contains(ua, "mobile") {
		return true
	}

	return false
}

// detectOS identifies operating system
func detectOS(ua string) (string, string) {
	uaLower := strings.ToLower(ua)

	// iOS
	if strings.Contains(uaLower, "iphone") || strings.Contains(uaLower, "ipad") || strings.Contains(uaLower, "ipod") {
		version := extractVersion(ua, `(?:iPhone|iPad|iPod).*?OS[/ ](\d+[._]\d+(?:[._]\d+)?)`)
		return "iOS", strings.ReplaceAll(version, "_", ".")
	}

	// Android
	if strings.Contains(uaLower, "android") {
		version := extractVersion(ua, `Android[/ ](\d+(?:\.\d+)*)`)
		return "Android", version
	}

	// Windows
	if strings.Contains(uaLower, "windows") {
		if strings.Contains(ua, "Windows NT 10.0") {
			return "Windows", "10/11"
		} else if strings.Contains(ua, "Windows NT 6.3") {
			return "Windows", "8.1"
		} else if strings.Contains(ua, "Windows NT 6.2") {
			return "Windows", "8"
		} else if strings.Contains(ua, "Windows NT 6.1") {
			return "Windows", "7"
		} else if strings.Contains(ua, "Windows NT 6.0") {
			return "Windows", "Vista"
		}
		return "Windows", ""
	}

	// macOS
	if strings.Contains(uaLower, "macintosh") || strings.Contains(uaLower, "mac os x") {
		version := extractVersion(ua, `Mac OS X[/ ](\d+[._]\d+(?:[._]\d+)?)`)
		return "macOS", strings.ReplaceAll(version, "_", ".")
	}

	// Linux
	if strings.Contains(uaLower, "linux") {
		if strings.Contains(uaLower, "ubuntu") {
			return "Ubuntu", ""
		} else if strings.Contains(uaLower, "fedora") {
			return "Fedora", ""
		} else if strings.Contains(uaLower, "debian") {
			return "Debian", ""
		}
		return "Linux", ""
	}

	// Chrome OS
	if strings.Contains(uaLower, "cros") {
		return "Chrome OS", ""
	}

	return "Unknown", ""
}

// detectBrowser identifies browser
func detectBrowser(ua string) (string, string) {
	uaLower := strings.ToLower(ua)

	// Order matters - check specific browsers before generic ones

	// Edge (new Chromium-based)
	if strings.Contains(uaLower, "edg/") {
		version := extractVersion(ua, `Edg/(\d+(?:\.\d+)*)`)
		return "Microsoft Edge", version
	}

	// Edge (old)
	if strings.Contains(uaLower, "edge/") {
		version := extractVersion(ua, `Edge/(\d+(?:\.\d+)*)`)
		return "Microsoft Edge", version
	}

	// Opera
	if strings.Contains(uaLower, "opr/") || strings.Contains(uaLower, "opera") {
		version := extractVersion(ua, `(?:OPR|Opera)[/ ](\d+(?:\.\d+)*)`)
		return "Opera", version
	}

	// Samsung Browser
	if strings.Contains(uaLower, "samsungbrowser") {
		version := extractVersion(ua, `SamsungBrowser/(\d+(?:\.\d+)*)`)
		return "Samsung Browser", version
	}

	// UC Browser
	if strings.Contains(uaLower, "ucbrowser") {
		version := extractVersion(ua, `UCBrowser/(\d+(?:\.\d+)*)`)
		return "UC Browser", version
	}

	// Brave
	if strings.Contains(uaLower, "brave") {
		return "Brave", ""
	}

	// Vivaldi
	if strings.Contains(uaLower, "vivaldi") {
		version := extractVersion(ua, `Vivaldi/(\d+(?:\.\d+)*)`)
		return "Vivaldi", version
	}

	// Firefox
	if strings.Contains(uaLower, "firefox") {
		version := extractVersion(ua, `Firefox/(\d+(?:\.\d+)*)`)
		return "Firefox", version
	}

	// Safari (must be before Chrome because Chrome contains Safari)
	if strings.Contains(uaLower, "safari") && !strings.Contains(uaLower, "chrome") && !strings.Contains(uaLower, "chromium") {
		version := extractVersion(ua, `Version/(\d+(?:\.\d+)*)`)
		return "Safari", version
	}

	// Chrome
	if strings.Contains(uaLower, "chrome") || strings.Contains(uaLower, "chromium") {
		version := extractVersion(ua, `(?:Chrome|Chromium)/(\d+(?:\.\d+)*)`)
		return "Chrome", version
	}

	// IE
	if strings.Contains(uaLower, "msie") || strings.Contains(uaLower, "trident") {
		version := extractVersion(ua, `(?:MSIE |rv:)(\d+(?:\.\d+)*)`)
		return "Internet Explorer", version
	}

	return "Unknown", ""
}

// detectDevice identifies device brand
func detectDevice(ua string) string {
	uaLower := strings.ToLower(ua)

	devices := map[string]string{
		"iphone":    "Apple iPhone",
		"ipad":      "Apple iPad",
		"ipod":      "Apple iPod",
		"macintosh": "Apple Mac",
		"samsung":   "Samsung",
		"xiaomi":    "Xiaomi",
		"redmi":     "Xiaomi Redmi",
		"poco":      "Xiaomi POCO",
		"huawei":    "Huawei",
		"honor":     "Honor",
		"oppo":      "OPPO",
		"vivo":      "Vivo",
		"oneplus":   "OnePlus",
		"realme":    "Realme",
		"motorola":  "Motorola",
		"nokia":     "Nokia",
		"lg-":       "LG",
		"htc":       "HTC",
		"sony":      "Sony",
		"asus":      "ASUS",
		"lenovo":    "Lenovo",
		"pixel":     "Google Pixel",
		"nexus":     "Google Nexus",
	}

	for key, name := range devices {
		if strings.Contains(uaLower, key) {
			return name
		}
	}

	if strings.Contains(uaLower, "windows") {
		return "Windows PC"
	}

	if strings.Contains(uaLower, "linux") && !strings.Contains(uaLower, "android") {
		return "Linux PC"
	}

	return "Unknown"
}

// extractVersion extracts version number using regex
func extractVersion(ua string, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(ua)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
