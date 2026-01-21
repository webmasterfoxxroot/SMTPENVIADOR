package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// BrowserResult holds the result of a browser automation operation
type BrowserResult struct {
	Success bool   `json:"success"`
	IP      string `json:"ip,omitempty"`
	Message string `json:"message,omitempty"`
}

// captureDebugInfo captures screenshot and page info for debugging
func captureDebugInfo(ctx context.Context, step string) {
	var buf []byte
	var bodyHTML string

	// Try to get body HTML (more useful than full HTML)
	err := chromedp.Run(ctx, chromedp.OuterHTML("body", &bodyHTML, chromedp.ByQuery))
	if err != nil {
		log.Printf("[Web Browser Debug] %s - Could not get body HTML: %v", step, err)
	} else {
		// Log first 1000 chars of body
		if len(bodyHTML) > 1000 {
			bodyHTML = bodyHTML[:1000] + "..."
		}
		log.Printf("[Web Browser Debug] %s - Body HTML: %s", step, bodyHTML)
	}

	// Also log current URL
	var currentURL string
	chromedp.Run(ctx, chromedp.Location(&currentURL))
	log.Printf("[Web Browser Debug] %s - Current URL: %s", step, currentURL)

	// Try to take screenshot
	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		buf, err = page.CaptureScreenshot().Do(ctx)
		return err
	}))
	if err == nil && len(buf) > 0 {
		// Log base64 encoded screenshot (first 100 chars for reference)
		encoded := base64.StdEncoding.EncodeToString(buf)
		log.Printf("[Web Browser Debug] %s - Screenshot captured (%d bytes)", step, len(buf))
		// Save screenshot to file for debugging
		os.WriteFile(fmt.Sprintf("/tmp/screenshot_%s_%d.png", step, time.Now().Unix()), buf, 0644)
		log.Printf("[Web Browser Debug] Screenshot saved to /tmp/screenshot_%s_%d.png", step, time.Now().Unix())
		_ = encoded // prevent unused variable warning
	}
}

// getChromePath returns the path to Chrome/Chromium executable
func getChromePath() string {
	// Check environment variable first
	if path := os.Getenv("CHROME_PATH"); path != "" {
		return path
	}

	// Common paths to check
	paths := []string{
		"/usr/bin/chromium-browser",  // Alpine Linux
		"/usr/bin/chromium",          // Some Linux distros
		"/usr/bin/google-chrome",     // Google Chrome
		"/usr/bin/google-chrome-stable",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Default fallback
	return "google-chrome"
}

// OutlookWebAutomation handles Outlook web interface automation
type OutlookWebAutomation struct {
	Email    string
	Password string
	Proxy    *ProxyConfig
}

// ProxyConfig holds proxy configuration
type ProxyConfig struct {
	Host     string
	Port     string
	Username string
	Password string
}

// NewOutlookWebAutomation creates a new Outlook web automation instance
func NewOutlookWebAutomation(email, password string, proxy *ProxyConfig) *OutlookWebAutomation {
	return &OutlookWebAutomation{
		Email:    email,
		Password: password,
		Proxy:    proxy,
	}
}

// createBrowserContext creates a chromedp context with optional proxy
func (o *OutlookWebAutomation) createBrowserContext(parentCtx context.Context) (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(getChromePath()),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	// Add proxy if configured
	if o.Proxy != nil && o.Proxy.Host != "" && o.Proxy.Port != "" {
		proxyURL := fmt.Sprintf("http://%s:%s", o.Proxy.Host, o.Proxy.Port)
		opts = append(opts, chromedp.ProxyServer(proxyURL))
		log.Printf("[Web Browser] Using proxy: %s:%s", o.Proxy.Host, o.Proxy.Port)
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(parentCtx, opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	// Set up proxy authentication if needed
	if o.Proxy != nil && o.Proxy.Username != "" && o.Proxy.Password != "" {
		chromedp.ListenTarget(ctx, func(ev interface{}) {
			switch e := ev.(type) {
			case *fetch.EventAuthRequired:
				go func() {
					chromedp.Run(ctx,
						fetch.ContinueWithAuth(e.RequestID, &fetch.AuthChallengeResponse{
							Response: fetch.AuthChallengeResponseResponseProvideCredentials,
							Username: o.Proxy.Username,
							Password: o.Proxy.Password,
						}),
					)
				}()
			case *fetch.EventRequestPaused:
				go func() {
					chromedp.Run(ctx, fetch.ContinueRequest(e.RequestID))
				}()
			}
		})

		// Enable fetch domain to intercept auth challenges
		chromedp.Run(ctx, fetch.Enable().WithHandleAuthRequests(true))
	}

	return ctx, func() {
		cancel()
		allocCancel()
	}
}

// TestLogin tests if the credentials work for Outlook web login
func (o *OutlookWebAutomation) TestLogin(parentCtx context.Context) (*BrowserResult, error) {
	log.Printf("[Web Browser] Testing login for %s", o.Email)
	log.Printf("[Web Browser] Chrome path: %s", getChromePath())

	ctx, cancel := o.createBrowserContext(parentCtx)
	defer cancel()

	// Set timeout - 3 minutes for the whole operation
	ctx, cancelTimeout := context.WithTimeout(ctx, 180*time.Second)
	defer cancelTimeout()

	var currentURL string
	var proxyIP string

	// First, get the proxy IP by visiting a simple IP check service
	if o.Proxy != nil && o.Proxy.Host != "" {
		log.Printf("[Web Browser] Getting proxy IP...")
		err := chromedp.Run(ctx,
			chromedp.Navigate("https://api.ipify.org"),
			chromedp.Sleep(3*time.Second),
			chromedp.Text("body", &proxyIP, chromedp.NodeVisible),
		)
		if err != nil {
			log.Printf("[Web Browser] Could not get proxy IP: %v", err)
		} else {
			proxyIP = strings.TrimSpace(proxyIP)
			log.Printf("[Web Browser] Proxy IP: %s", proxyIP)
		}
	}

	// Navigate to Outlook login
	log.Printf("[Web Browser] Navigating to login.live.com...")
	err := chromedp.Run(ctx,
		chromedp.Navigate("https://login.live.com/"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate to login page: %v", err)
	}

	// Wait for page to fully load
	log.Printf("[Web Browser] Waiting for page to load...")
	err = chromedp.Run(ctx,
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		log.Printf("[Web Browser] Warning: WaitReady failed: %v", err)
	}

	// Capture debug info after navigation
	captureDebugInfo(ctx, "after_navigation")

	// Wait for email field - try multiple selectors
	log.Printf("[Web Browser] Waiting for email field...")

	// List of selectors to try for email field
	emailSelectors := []string{
		`input[name="loginfmt"]`,
		`#i0116`,
		`input[type="email"]`,
		`input[type="text"]`,
	}

	var emailSelector string
	for _, sel := range emailSelectors {
		waitCtx, waitCancel := context.WithTimeout(ctx, 3*time.Second)
		err = chromedp.Run(waitCtx,
			chromedp.WaitVisible(sel, chromedp.ByQuery),
		)
		waitCancel()
		if err == nil {
			emailSelector = sel
			log.Printf("[Web Browser] Found email field with selector: %s", sel)
			break
		}
		log.Printf("[Web Browser] Selector %s not found, trying next...", sel)
	}

	if emailSelector == "" {
		captureDebugInfo(ctx, "all_email_selectors_failed")
		return nil, fmt.Errorf("failed to find email field with any selector")
	}

	// Enter email using the selector that worked
	log.Printf("[Web Browser] Entering email: %s", o.Email)
	err = chromedp.Run(ctx,
		chromedp.Clear(emailSelector, chromedp.ByQuery),
		chromedp.SendKeys(emailSelector, o.Email, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter email: %v", err)
	}

	// Click Next button - try multiple approaches
	log.Printf("[Web Browser] Clicking Next button...")
	buttonSelectors := []string{`#idSIButton9`, `input[type="submit"]`, `button[type="submit"]`}

	// First try: Press Enter key (most reliable)
	err = chromedp.Run(ctx,
		chromedp.SendKeys(emailSelector, "\n", chromedp.ByQuery),
	)
	if err != nil {
		log.Printf("[Web Browser] Enter key failed: %v, trying click...", err)

		// Second try: Click with short timeout
		for _, sel := range buttonSelectors {
			clickCtx, clickCancel := context.WithTimeout(ctx, 3*time.Second)
			err = chromedp.Run(clickCtx, chromedp.Click(sel, chromedp.ByQuery))
			clickCancel()
			if err == nil {
				log.Printf("[Web Browser] Clicked button with selector: %s", sel)
				break
			}
		}
	} else {
		log.Printf("[Web Browser] Pressed Enter key to submit")
	}

	chromedp.Run(ctx, chromedp.Sleep(5*time.Second))

	// Check current URL
	chromedp.Run(ctx, chromedp.Location(&currentURL))
	log.Printf("[Web Browser] URL after email: %s", currentURL)

	// Capture debug after clicking next
	captureDebugInfo(ctx, "after_next_click")

	// Wait for password field - try multiple selectors
	log.Printf("[Web Browser] Waiting for password field...")
	passwordSelectors := []string{`input[name="passwd"]`, `#i0118`, `input[type="password"]`}
	var passwordSelector string
	for _, sel := range passwordSelectors {
		waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
		err = chromedp.Run(waitCtx, chromedp.WaitVisible(sel, chromedp.ByQuery))
		waitCancel()
		if err == nil {
			passwordSelector = sel
			log.Printf("[Web Browser] Found password field with selector: %s", sel)
			break
		}
		log.Printf("[Web Browser] Password selector %s not found, trying next...", sel)
	}

	if passwordSelector == "" {
		captureDebugInfo(ctx, "password_field_not_found")
		return nil, fmt.Errorf("failed to find password field with any selector")
	}

	// Enter password using the selector that worked
	log.Printf("[Web Browser] Entering password...")
	err = chromedp.Run(ctx,
		chromedp.Clear(passwordSelector, chromedp.ByQuery),
		chromedp.SendKeys(passwordSelector, o.Password, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter password: %v", err)
	}

	// Click Sign in button - use Enter key (more reliable)
	log.Printf("[Web Browser] Clicking Sign in button...")
	err = chromedp.Run(ctx,
		chromedp.SendKeys(passwordSelector, "\n", chromedp.ByQuery),
	)
	if err != nil {
		log.Printf("[Web Browser] Enter key for sign in failed: %v, trying click...", err)
		for _, sel := range buttonSelectors {
			clickCtx, clickCancel := context.WithTimeout(ctx, 3*time.Second)
			err = chromedp.Run(clickCtx, chromedp.Click(sel, chromedp.ByQuery))
			clickCancel()
			if err == nil {
				log.Printf("[Web Browser] Clicked sign in with selector: %s", sel)
				break
			}
		}
	} else {
		log.Printf("[Web Browser] Pressed Enter key to sign in")
	}
	chromedp.Run(ctx, chromedp.Sleep(5*time.Second))

	// Check current URL for FIDO/Passkey prompt
	chromedp.Run(ctx, chromedp.Location(&currentURL))
	log.Printf("[Web Browser] URL after sign in: %s", currentURL)

	// Handle multiple Microsoft prompts after login
	for i := 0; i < 5; i++ {
		chromedp.Run(ctx, chromedp.Location(&currentURL))
		log.Printf("[Web Browser] Post-login check %d, URL: %s", i+1, currentURL)

		// Check if we're already logged in
		if strings.Contains(currentURL, "outlook.live.com") ||
			strings.Contains(currentURL, "outlook.office.com") ||
			strings.Contains(currentURL, "mail.live.com") {
			log.Printf("[Web Browser] Already logged in!")
			break
		}

		// Handle FIDO/Passkey setup prompt (can appear as modal on ppsecure page too)
		if strings.Contains(currentURL, "fido") || strings.Contains(currentURL, "passkey") || strings.Contains(currentURL, "ppsecure") {
			log.Printf("[Web Browser] Found Passkey/FIDO/ppsecure page, trying to dismiss modal...")

			// Try pressing ESC via JavaScript (more reliable)
			chromedp.Run(ctx, chromedp.Evaluate(`
				document.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', code: 'Escape', keyCode: 27, which: 27, bubbles: true}));
				document.dispatchEvent(new KeyboardEvent('keyup', {key: 'Escape', code: 'Escape', keyCode: 27, which: 27, bubbles: true}));
			`, nil))
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))

			// Also try chromedp.KeyEvent
			chromedp.Run(ctx, chromedp.KeyEvent("\x1b")) // ESC key
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))

			// Check if modal closed
			chromedp.Run(ctx, chromedp.Location(&currentURL))
			log.Printf("[Web Browser] URL after ESC attempts: %s", currentURL)
			if !strings.Contains(currentURL, "ppsecure") && !strings.Contains(currentURL, "fido") {
				log.Printf("[Web Browser] ESC key worked, modal closed")
				continue
			}

			// Try clicking cancel buttons with short timeouts
			clickWithTimeout := func(sel string) {
				clickCtx, clickCancel := context.WithTimeout(ctx, 2*time.Second)
				chromedp.Run(clickCtx, chromedp.Click(sel, chromedp.ByQuery))
				clickCancel()
			}

			log.Printf("[Web Browser] Trying to click Cancelar buttons...")
			clickWithTimeout(`button:has-text("Cancelar")`)
			clickWithTimeout(`//button[contains(text(), 'Cancelar')]`)
			clickWithTimeout(`#CancelNo`)
			clickWithTimeout(`#idBtn_Back`)
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			continue
		}

		// Handle "Stay signed in?" / "Continuar conectado?" prompt
		if strings.Contains(currentURL, "kmsi") || strings.Contains(currentURL, "login.srf") {
			log.Printf("[Web Browser] Found 'Stay signed in' prompt, clicking No...")
			chromedp.Run(ctx, chromedp.Click(`//button[contains(text(), 'Não')]`, chromedp.BySearch))
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			chromedp.Run(ctx, chromedp.Click(`#idBtn_Back`, chromedp.ByID))
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			continue
		}

		// Handle "Protect your account" / "Proteger sua conta" prompt
		if strings.Contains(currentURL, "proofs") || strings.Contains(currentURL, "security") {
			log.Printf("[Web Browser] Found security prompt, clicking Skip...")
			chromedp.Run(ctx, chromedp.Click(`//a[contains(text(), 'Ignorar')]`, chromedp.BySearch))
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			chromedp.Run(ctx, chromedp.Click(`//button[contains(text(), 'Ignorar')]`, chromedp.BySearch))
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			chromedp.Run(ctx, chromedp.Click(`#iCancel`, chromedp.ByID))
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			continue
		}

		// Generic: try clicking any skip/cancel/no buttons
		log.Printf("[Web Browser] Trying generic skip/cancel buttons...")
		chromedp.Run(ctx, chromedp.Click(`//a[contains(text(), 'Ignorar por enquanto')]`, chromedp.BySearch))
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
		chromedp.Run(ctx, chromedp.Click(`//button[contains(text(), 'Não')]`, chromedp.BySearch))
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
		chromedp.Run(ctx, chromedp.Click(`//button[contains(text(), 'Cancelar')]`, chromedp.BySearch))
		chromedp.Run(ctx, chromedp.Sleep(1*time.Second))

		// Check if still on login page
		if strings.Contains(currentURL, "login.live.com") || strings.Contains(currentURL, "login.microsoft.com") {
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
		} else {
			break
		}
	}

	// Check if we're logged in or if there's an error
	err = chromedp.Run(ctx,
		chromedp.Location(&currentURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get current URL: %v", err)
	}

	log.Printf("[Web Browser] Current URL after login: %s", currentURL)

	// Check for successful login indicators
	if strings.Contains(currentURL, "outlook.live.com") ||
		strings.Contains(currentURL, "outlook.office.com") ||
		strings.Contains(currentURL, "mail.live.com") ||
		strings.Contains(currentURL, "office.com") ||
		strings.Contains(currentURL, "microsoftonline.com") {
		log.Printf("[Web Browser] Login successful!")
		return &BrowserResult{
			Success: true,
			IP:      proxyIP,
			Message: "Login successful",
		}, nil
	}

	// Check for error indicators
	if strings.Contains(currentURL, "login.live.com") {
		// Still on login page, check for error message
		var errorText string

		// Try multiple error selectors
		selectors := []string{
			"#usernameError",
			"#passwordError",
			"#errorText",
			".alert-error",
			"#error",
		}

		for _, sel := range selectors {
			chromedp.Run(ctx, chromedp.Text(sel, &errorText, chromedp.ByQuery))
			if errorText != "" {
				break
			}
		}

		if errorText != "" {
			log.Printf("[Web Browser] Login error: %s", errorText)
			return nil, fmt.Errorf("login failed: %s", errorText)
		}

		log.Printf("[Web Browser] Still on login page, no specific error found")
		return nil, fmt.Errorf("login failed: still on login page after password entry")
	}

	// If we got here with an unknown URL, consider it a success
	log.Printf("[Web Browser] Login completed, final URL: %s", currentURL)
	return &BrowserResult{
		Success: true,
		IP:      proxyIP,
		Message: fmt.Sprintf("Login completed, redirected to: %s", currentURL),
	}, nil
}

// SendEmail sends an email through Outlook web interface
func (o *OutlookWebAutomation) SendEmail(parentCtx context.Context, toEmail, subject, body string) (*BrowserResult, error) {
	log.Printf("[Web Browser] Sending email from %s to %s", o.Email, toEmail)

	ctx, cancel := o.createBrowserContext(parentCtx)
	defer cancel()

	// Set timeout
	ctx, cancelTimeout := context.WithTimeout(ctx, 120*time.Second)
	defer cancelTimeout()

	var proxyIP string

	// Get proxy IP first
	if o.Proxy != nil && o.Proxy.Host != "" {
		chromedp.Run(ctx,
			chromedp.Navigate("https://api.ipify.org"),
			chromedp.Sleep(2*time.Second),
			chromedp.Text("body", &proxyIP, chromedp.NodeVisible),
		)
		proxyIP = strings.TrimSpace(proxyIP)
	}

	// Navigate to Outlook login
	err := chromedp.Run(ctx,
		chromedp.Navigate("https://login.live.com/"),
		chromedp.WaitVisible(`input[type="email"]`, chromedp.ByQuery),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load login page: %v", err)
	}

	// Login
	err = chromedp.Run(ctx,
		chromedp.SendKeys(`input[type="email"]`, o.Email, chromedp.ByQuery),
		chromedp.Click(`input[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		chromedp.WaitVisible(`input[type="password"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[type="password"]`, o.Password, chromedp.ByQuery),
		chromedp.Click(`input[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to login: %v", err)
	}

	// Handle "Stay signed in?" prompt if it appears
	chromedp.Run(ctx,
		chromedp.Click(`input[type="submit"][value="No"]`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
	)

	// Navigate to Outlook mail
	err = chromedp.Run(ctx,
		chromedp.Navigate("https://outlook.live.com/mail/0/"),
		chromedp.Sleep(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load mail: %v", err)
	}

	// Click "New message" button
	err = chromedp.Run(ctx,
		chromedp.WaitVisible(`button[aria-label="New mail"]`, chromedp.ByQuery),
		chromedp.Click(`button[aria-label="New mail"]`, chromedp.ByQuery),
		chromedp.Sleep(2*time.Second),
	)
	if err != nil {
		// Try alternative selector
		err = chromedp.Run(ctx,
			chromedp.Click(`button[data-testid="newMessage"]`, chromedp.ByQuery),
			chromedp.Sleep(2*time.Second),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to click new message: %v", err)
		}
	}

	// Fill in recipient
	err = chromedp.Run(ctx,
		chromedp.WaitVisible(`input[aria-label="To"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[aria-label="To"]`, toEmail, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter recipient: %v", err)
	}

	// Fill in subject
	err = chromedp.Run(ctx,
		chromedp.SendKeys(`input[aria-label="Add a subject"]`, subject, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter subject: %v", err)
	}

	// Fill in body
	err = chromedp.Run(ctx,
		chromedp.Click(`div[aria-label="Message body"]`, chromedp.ByQuery),
		chromedp.SendKeys(`div[aria-label="Message body"]`, body, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter body: %v", err)
	}

	// Click Send button
	err = chromedp.Run(ctx,
		chromedp.Click(`button[aria-label="Send"]`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send email: %v", err)
	}

	log.Printf("[Web Browser] Email sent successfully from %s to %s", o.Email, toEmail)

	return &BrowserResult{
		Success: true,
		IP:      proxyIP,
	}, nil
}

// Update testOutlookWebLogin to use the new automation
func (s *Server) testOutlookWebLogin(ctx context.Context, email, password string, settings *WebWarmupSettings) (*BrowserResult, error) {
	var proxy *ProxyConfig
	if settings.UseProxy && settings.ProxyHost != "" {
		proxy = &ProxyConfig{
			Host:     settings.ProxyHost,
			Port:     settings.ProxyPort,
			Username: settings.ProxyUser,
			Password: settings.ProxyPass,
		}
	}

	automation := NewOutlookWebAutomation(email, password, proxy)
	return automation.TestLogin(ctx)
}

// Update sendOutlookWebEmail to use the new automation
func (s *Server) sendOutlookWebEmail(ctx context.Context, email, password, toEmail, subject, body string, settings *WebWarmupSettings) (*BrowserResult, error) {
	var proxy *ProxyConfig
	if settings.UseProxy && settings.ProxyHost != "" {
		proxy = &ProxyConfig{
			Host:     settings.ProxyHost,
			Port:     settings.ProxyPort,
			Username: settings.ProxyUser,
			Password: settings.ProxyPass,
		}
	}

	automation := NewOutlookWebAutomation(email, password, proxy)
	return automation.SendEmail(ctx, toEmail, subject, body)
}
