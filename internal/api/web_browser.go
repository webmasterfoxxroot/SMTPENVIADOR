package api

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

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
			if _, ok := ev.(*network.EventAuthRequired); ok {
				go func() {
					chromedp.Run(ctx,
						network.ContinueInterceptedRequest(network.AuthChallengeResponse{
							Response: network.AuthChallengeResponseResponseProvideCredentials,
							Username: o.Proxy.Username,
							Password: o.Proxy.Password,
						}),
					)
				}()
			}
		})
	}

	return ctx, func() {
		cancel()
		allocCancel()
	}
}

// TestLogin tests if the credentials work for Outlook web login
func (o *OutlookWebAutomation) TestLogin(parentCtx context.Context) (*BrowserResult, error) {
	log.Printf("[Web Browser] Testing login for %s", o.Email)

	ctx, cancel := o.createBrowserContext(parentCtx)
	defer cancel()

	// Set timeout
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	defer cancelTimeout()

	var currentURL string
	var proxyIP string

	// First, get the proxy IP by visiting a simple IP check service
	if o.Proxy != nil && o.Proxy.Host != "" {
		err := chromedp.Run(ctx,
			chromedp.Navigate("https://api.ipify.org"),
			chromedp.Sleep(2*time.Second),
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
	err := chromedp.Run(ctx,
		chromedp.Navigate("https://login.live.com/"),
		chromedp.WaitVisible(`input[type="email"]`, chromedp.ByQuery),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load login page: %v", err)
	}

	// Enter email
	err = chromedp.Run(ctx,
		chromedp.SendKeys(`input[type="email"]`, o.Email, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Click(`input[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter email: %v", err)
	}

	// Wait for password field and enter password
	err = chromedp.Run(ctx,
		chromedp.WaitVisible(`input[type="password"]`, chromedp.ByQuery),
		chromedp.SendKeys(`input[type="password"]`, o.Password, chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Click(`input[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enter password: %v", err)
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
		strings.Contains(currentURL, "kmsi") { // "Keep me signed in" page
		return &BrowserResult{
			Success: true,
			IP:      proxyIP,
		}, nil
	}

	// Check for error indicators
	if strings.Contains(currentURL, "login.live.com") {
		// Still on login page, check for error message
		var errorText string
		chromedp.Run(ctx,
			chromedp.Text(`#usernameError, #passwordError, .alert`, &errorText, chromedp.ByQuery),
		)
		if errorText != "" {
			return nil, fmt.Errorf("login failed: %s", errorText)
		}
		return nil, fmt.Errorf("login failed: still on login page")
	}

	return &BrowserResult{
		Success: true,
		IP:      proxyIP,
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
