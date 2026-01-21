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
	var html string

	// Try to get page HTML
	chromedp.Run(ctx, chromedp.OuterHTML("html", &html))
	if len(html) > 500 {
		html = html[:500] + "..."
	}
	log.Printf("[Web Browser Debug] %s - Page HTML (truncated): %s", step, html)

	// Try to take screenshot
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
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

	// Set timeout
	ctx, cancelTimeout := context.WithTimeout(ctx, 90*time.Second)
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
		chromedp.Sleep(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to navigate to login page: %v", err)
	}

	// Capture debug info after navigation
	captureDebugInfo(ctx, "after_navigation")

	// Wait for email field - try multiple selectors with shorter timeout
	log.Printf("[Web Browser] Waiting for email field...")

	// Create shorter timeout context for element wait
	waitCtx, waitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer waitCancel()

	err = chromedp.Run(waitCtx,
		chromedp.WaitVisible(`#i0116`, chromedp.ByID),
	)
	if err != nil {
		log.Printf("[Web Browser] Email field #i0116 not found, trying alternative selectors...")
		captureDebugInfo(ctx, "email_field_not_found")

		// Try alternative selector
		waitCtx2, waitCancel2 := context.WithTimeout(ctx, 10*time.Second)
		defer waitCancel2()

		err = chromedp.Run(waitCtx2,
			chromedp.WaitVisible(`input[type="email"]`, chromedp.ByQuery),
		)
		if err != nil {
			// Try name attribute
			waitCtx3, waitCancel3 := context.WithTimeout(ctx, 10*time.Second)
			defer waitCancel3()

			err = chromedp.Run(waitCtx3,
				chromedp.WaitVisible(`input[name="loginfmt"]`, chromedp.ByQuery),
			)
			if err != nil {
				captureDebugInfo(ctx, "all_email_selectors_failed")
				return nil, fmt.Errorf("failed to find email field: %v", err)
			}
		}
	}

	// Enter email
	log.Printf("[Web Browser] Entering email: %s", o.Email)
	err = chromedp.Run(ctx,
		chromedp.Clear(`#i0116`, chromedp.ByID),
		chromedp.SendKeys(`#i0116`, o.Email, chromedp.ByID),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter email: %v", err)
	}

	// Click Next button
	log.Printf("[Web Browser] Clicking Next button...")
	err = chromedp.Run(ctx,
		chromedp.Click(`#idSIButton9`, chromedp.ByID),
		chromedp.Sleep(4*time.Second),
	)
	if err != nil {
		// Try alternative selector
		err = chromedp.Run(ctx,
			chromedp.Click(`input[type="submit"]`, chromedp.ByQuery),
			chromedp.Sleep(4*time.Second),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to click next button: %v", err)
		}
	}

	// Check current URL
	chromedp.Run(ctx, chromedp.Location(&currentURL))
	log.Printf("[Web Browser] URL after email: %s", currentURL)

	// Wait for password field
	log.Printf("[Web Browser] Waiting for password field...")
	err = chromedp.Run(ctx,
		chromedp.WaitVisible(`#i0118`, chromedp.ByID),
	)
	if err != nil {
		// Try alternative selector
		err = chromedp.Run(ctx,
			chromedp.WaitVisible(`input[type="password"]`, chromedp.ByQuery),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to find password field: %v", err)
		}
	}

	// Enter password
	log.Printf("[Web Browser] Entering password...")
	err = chromedp.Run(ctx,
		chromedp.Clear(`#i0118`, chromedp.ByID),
		chromedp.SendKeys(`#i0118`, o.Password, chromedp.ByID),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter password: %v", err)
	}

	// Click Sign in button
	log.Printf("[Web Browser] Clicking Sign in button...")
	err = chromedp.Run(ctx,
		chromedp.Click(`#idSIButton9`, chromedp.ByID),
		chromedp.Sleep(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to click sign in button: %v", err)
	}

	// Check current URL for FIDO/Passkey prompt
	chromedp.Run(ctx, chromedp.Location(&currentURL))
	log.Printf("[Web Browser] URL after sign in: %s", currentURL)

	// Handle FIDO/Passkey setup prompt (login.microsoft.com/consumers/fido)
	if strings.Contains(currentURL, "fido") || strings.Contains(currentURL, "passkey") {
		log.Printf("[Web Browser] Found Passkey/FIDO prompt, clicking Cancel...")
		// Try to click "Cancelar" button
		chromedp.Run(ctx,
			chromedp.Click(`button:contains("Cancelar")`, chromedp.ByQuery),
			chromedp.Sleep(2*time.Second),
		)
		// Try alternative selectors
		chromedp.Run(ctx,
			chromedp.Click(`button.secondary`, chromedp.ByQuery),
			chromedp.Sleep(2*time.Second),
		)
		chromedp.Run(ctx,
			chromedp.Click(`#cancelBtn`, chromedp.ByID),
			chromedp.Sleep(2*time.Second),
		)
		// Click any "Cancel" or "Cancelar" text
		chromedp.Run(ctx,
			chromedp.Click(`//button[contains(text(), 'Cancelar')]`, chromedp.BySearch),
			chromedp.Sleep(2*time.Second),
		)
		chromedp.Run(ctx,
			chromedp.Click(`//button[contains(text(), 'Cancel')]`, chromedp.BySearch),
			chromedp.Sleep(2*time.Second),
		)
		chromedp.Run(ctx, chromedp.Location(&currentURL))
		log.Printf("[Web Browser] URL after FIDO cancel: %s", currentURL)
	}

	// Check if we're logged in or if there's an error
	err = chromedp.Run(ctx,
		chromedp.Location(&currentURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get current URL: %v", err)
	}

	log.Printf("[Web Browser] Current URL after login: %s", currentURL)

	// Handle "Stay signed in?" prompt if present
	if strings.Contains(currentURL, "kmsi") {
		log.Printf("[Web Browser] Found 'Stay signed in' prompt, clicking No...")
		chromedp.Run(ctx,
			chromedp.Click(`#idBtn_Back`, chromedp.ByID), // "No" button
			chromedp.Sleep(3*time.Second),
		)
		chromedp.Run(ctx, chromedp.Location(&currentURL))
		log.Printf("[Web Browser] URL after KMSI: %s", currentURL)
	}

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
