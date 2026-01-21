package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

// GraphAPIClient handles Microsoft Graph API operations
type GraphAPIClient struct {
	Email        string
	RefreshToken string
	ClientID     string
	AccessToken  string
	ExpiresAt    time.Time
	Proxy        *ProxyConfig
}

// TokenResponse represents the OAuth2 token response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// GraphEmailMessage represents an email message for Graph API
type GraphEmailMessage struct {
	Message         GraphMessage `json:"message"`
	SaveToSentItems bool         `json:"saveToSentItems"`
}

// GraphMessage represents the message structure
type GraphMessage struct {
	Subject      string              `json:"subject"`
	Body         GraphBody           `json:"body"`
	ToRecipients []GraphRecipient    `json:"toRecipients"`
	From         *GraphRecipientAddr `json:"from,omitempty"`
}

// GraphBody represents the email body
type GraphBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

// GraphRecipient represents an email recipient
type GraphRecipient struct {
	EmailAddress GraphEmailAddress `json:"emailAddress"`
}

// GraphEmailAddress represents an email address
type GraphEmailAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// GraphRecipientAddr for from field
type GraphRecipientAddr struct {
	EmailAddress GraphEmailAddress `json:"emailAddress"`
}

// GraphAPIResult holds the result of a Graph API operation
type GraphAPIResult struct {
	Success bool   `json:"success"`
	IP      string `json:"ip,omitempty"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// NewGraphAPIClient creates a new Graph API client
func NewGraphAPIClient(email, refreshToken, clientID string, proxyConfig *ProxyConfig) *GraphAPIClient {
	return &GraphAPIClient{
		Email:        email,
		RefreshToken: refreshToken,
		ClientID:     clientID,
		Proxy:        proxyConfig,
	}
}

// createHTTPClient creates an HTTP client with optional proxy support
func (g *GraphAPIClient) createHTTPClient() (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Configure proxy if provided
	if g.Proxy != nil && g.Proxy.Host != "" && g.Proxy.Port != "" {
		proxyURL := fmt.Sprintf("http://%s:%s", g.Proxy.Host, g.Proxy.Port)

		// Add authentication if provided
		if g.Proxy.Username != "" && g.Proxy.Password != "" {
			proxyURL = fmt.Sprintf("http://%s:%s@%s:%s",
				url.QueryEscape(g.Proxy.Username),
				url.QueryEscape(g.Proxy.Password),
				g.Proxy.Host,
				g.Proxy.Port)
		}

		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %v", err)
		}

		transport.Proxy = http.ProxyURL(parsedURL)
		log.Printf("[Graph API] Using HTTP proxy: %s:%s", g.Proxy.Host, g.Proxy.Port)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

// createSOCKS5Client creates an HTTP client with SOCKS5 proxy support
func (g *GraphAPIClient) createSOCKS5Client() (*http.Client, error) {
	if g.Proxy == nil || g.Proxy.Host == "" {
		return g.createHTTPClient()
	}

	var auth *proxy.Auth
	if g.Proxy.Username != "" {
		auth = &proxy.Auth{
			User:     g.Proxy.Username,
			Password: g.Proxy.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("%s:%s", g.Proxy.Host, g.Proxy.Port), auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS5 dialer: %v", err)
	}

	transport := &http.Transport{
		Dial:                dialer.Dial,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}, nil
}

// RefreshAccessToken exchanges the refresh token for a new access token
func (g *GraphAPIClient) RefreshAccessToken(ctx context.Context) error {
	log.Printf("[Graph API] Refreshing access token for %s", g.Email)

	client, err := g.createHTTPClient()
	if err != nil {
		return fmt.Errorf("failed to create HTTP client: %v", err)
	}

	// Microsoft OAuth2 token endpoint (common for all account types)
	tokenURL := "https://login.microsoftonline.com/common/oauth2/v2.0/token"

	// Build the form data - don't specify scope to use the original token's scopes
	data := url.Values{}
	data.Set("client_id", g.ClientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", g.RefreshToken)
	// No scope specified - uses original scopes from token

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send token request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %v", err)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %v", err)
	}

	if tokenResp.Error != "" {
		return fmt.Errorf("token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("no access token in response")
	}

	g.AccessToken = tokenResp.AccessToken
	g.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Update refresh token if a new one was provided
	if tokenResp.RefreshToken != "" {
		g.RefreshToken = tokenResp.RefreshToken
	}

	log.Printf("[Graph API] Access token obtained, expires at %s", g.ExpiresAt.Format(time.RFC3339))
	return nil
}

// EnsureValidToken ensures we have a valid access token
func (g *GraphAPIClient) EnsureValidToken(ctx context.Context) error {
	// Check if token is still valid (with 5 minute buffer)
	if g.AccessToken != "" && time.Now().Add(5*time.Minute).Before(g.ExpiresAt) {
		return nil
	}

	return g.RefreshAccessToken(ctx)
}

// GetProxyIP returns the current proxy IP address
func (g *GraphAPIClient) GetProxyIP(ctx context.Context) (string, error) {
	client, err := g.createHTTPClient()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.ipify.org", nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

// SendEmail sends an email using Microsoft Graph API
func (g *GraphAPIClient) SendEmail(ctx context.Context, toEmail, subject, bodyContent string) (*GraphAPIResult, error) {
	log.Printf("[Graph API] Sending email from %s to %s", g.Email, toEmail)

	// Get proxy IP for logging
	var proxyIP string
	if g.Proxy != nil && g.Proxy.Host != "" {
		ip, err := g.GetProxyIP(ctx)
		if err != nil {
			log.Printf("[Graph API] Could not get proxy IP: %v", err)
		} else {
			proxyIP = ip
			log.Printf("[Graph API] Using proxy IP: %s", proxyIP)
		}
	}

	// Ensure we have a valid access token
	if err := g.EnsureValidToken(ctx); err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to get access token: %v", err),
		}, err
	}

	client, err := g.createHTTPClient()
	if err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to create HTTP client: %v", err),
		}, err
	}

	// Build the email message
	emailMsg := GraphEmailMessage{
		Message: GraphMessage{
			Subject: subject,
			Body: GraphBody{
				ContentType: "HTML",
				Content:     bodyContent,
			},
			ToRecipients: []GraphRecipient{
				{
					EmailAddress: GraphEmailAddress{
						Address: toEmail,
					},
				},
			},
		},
		SaveToSentItems: true,
	}

	jsonData, err := json.Marshal(emailMsg)
	if err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to marshal email: %v", err),
		}, err
	}

	// Microsoft Graph API sendMail endpoint
	graphURL := "https://graph.microsoft.com/v1.0/me/sendMail"

	req, err := http.NewRequestWithContext(ctx, "POST", graphURL, bytes.NewReader(jsonData))
	if err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to create request: %v", err),
		}, err
	}

	req.Header.Set("Authorization", "Bearer "+g.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to send request: %v", err),
		}, err
	}
	defer resp.Body.Close()

	// Graph API returns 202 Accepted for successful sendMail
	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
		log.Printf("[Graph API] Email sent successfully from %s to %s", g.Email, toEmail)
		return &GraphAPIResult{
			Success: true,
			IP:      proxyIP,
			Message: "Email sent successfully via Graph API",
		}, nil
	}

	// Read error response
	body, _ := io.ReadAll(resp.Body)
	errMsg := fmt.Sprintf("Graph API error: status %d - %s", resp.StatusCode, string(body))
	log.Printf("[Graph API] %s", errMsg)

	return &GraphAPIResult{
		Success: false,
		IP:      proxyIP,
		Error:   errMsg,
	}, fmt.Errorf(errMsg)
}

// TestConnection tests if the Graph API credentials work
func (g *GraphAPIClient) TestConnection(ctx context.Context) (*GraphAPIResult, error) {
	log.Printf("[Graph API] Testing connection for %s", g.Email)

	// Get proxy IP for logging
	var proxyIP string
	if g.Proxy != nil && g.Proxy.Host != "" {
		ip, err := g.GetProxyIP(ctx)
		if err != nil {
			log.Printf("[Graph API] Could not get proxy IP: %v", err)
		} else {
			proxyIP = ip
			log.Printf("[Graph API] Using proxy IP: %s", proxyIP)
		}
	}

	// Try to get access token
	if err := g.RefreshAccessToken(ctx); err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to authenticate: %v", err),
		}, err
	}

	// Try to get user profile to verify the token works
	client, err := g.createHTTPClient()
	if err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to create HTTP client: %v", err),
		}, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://graph.microsoft.com/v1.0/me", nil)
	if err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to create request: %v", err),
		}, err
	}

	req.Header.Set("Authorization", "Bearer "+g.AccessToken)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return &GraphAPIResult{
			Success: false,
			IP:      proxyIP,
			Error:   fmt.Sprintf("Failed to send request: %v", err),
		}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var profile map[string]interface{}
		json.Unmarshal(body, &profile)

		displayName := ""
		if name, ok := profile["displayName"].(string); ok {
			displayName = name
		}

		log.Printf("[Graph API] Connection successful for %s (Display: %s)", g.Email, displayName)
		return &GraphAPIResult{
			Success: true,
			IP:      proxyIP,
			Message: fmt.Sprintf("Connected as %s (%s)", g.Email, displayName),
		}, nil
	}

	body, _ := io.ReadAll(resp.Body)
	errMsg := fmt.Sprintf("Graph API error: status %d - %s", resp.StatusCode, string(body))
	return &GraphAPIResult{
		Success: false,
		IP:      proxyIP,
		Error:   errMsg,
	}, fmt.Errorf(errMsg)
}

// Server methods for Graph API

// testGraphAPIConnection tests Graph API connection for Web Warmup
func (s *Server) testGraphAPIConnection(ctx context.Context, email, refreshToken, clientID string, settings *WebWarmupSettings) (*GraphAPIResult, error) {
	var proxyConfig *ProxyConfig
	if settings.UseProxy && settings.ProxyHost != "" {
		proxyConfig = &ProxyConfig{
			Host:     settings.ProxyHost,
			Port:     settings.ProxyPort,
			Username: settings.ProxyUser,
			Password: settings.ProxyPass,
		}
	}

	client := NewGraphAPIClient(email, refreshToken, clientID, proxyConfig)
	return client.TestConnection(ctx)
}

// sendGraphAPIEmail sends email via Graph API for Web Warmup
func (s *Server) sendGraphAPIEmail(ctx context.Context, email, refreshToken, clientID, toEmail, subject, body string, settings *WebWarmupSettings) (*GraphAPIResult, error) {
	var proxyConfig *ProxyConfig
	if settings.UseProxy && settings.ProxyHost != "" {
		proxyConfig = &ProxyConfig{
			Host:     settings.ProxyHost,
			Port:     settings.ProxyPort,
			Username: settings.ProxyUser,
			Password: settings.ProxyPass,
		}
	}

	client := NewGraphAPIClient(email, refreshToken, clientID, proxyConfig)
	return client.SendEmail(ctx, toEmail, subject, body)
}
