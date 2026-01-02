package pateplay

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a Pateplay RGS Wallet API client
type Client struct {
	config     *ClientConfig
	httpClient *http.Client
}

// NewClient creates a new Pateplay API client
func NewClient(config *ClientConfig) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// NewClientWithHTTPClient creates a new Pateplay API client with a custom HTTP client
func NewClientWithHTTPClient(config *ClientConfig, httpClient *http.Client) *Client {
	return &Client{
		config:     config,
		httpClient: httpClient,
	}
}

// computeHMAC computes the HMAC-SHA256 signature for the request body
func (c *Client) computeHMAC(body []byte) string {
	h := hmac.New(sha256.New, []byte(c.config.APISecret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// doRequest performs an HTTP request with HMAC signing
func (c *Client) doRequest(ctx context.Context, endpoint string, reqBody interface{}, result interface{}) error {
	// Marshal request body
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create request
	url := c.config.BaseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("x-api-hmac", c.computeHMAC(bodyBytes))

	// Execute request with retry
	var resp *http.Response
	var lastErr error
	retryCount := c.config.RetryCount
	if retryCount == 0 {
		retryCount = 1
	}

	for i := 0; i < retryCount; i++ {
		resp, err = c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		break
	}

	if resp == nil {
		return fmt.Errorf("request failed after %d retries: %w", retryCount, lastErr)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	if err := json.Unmarshal(respBody, result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}

// Authenticate creates a new session token from a one-time auth token
// This is called when a player opens a Pateplay game
func (c *Client) Authenticate(ctx context.Context, authToken string, deviceType DeviceType) (*AuthenticateResult, error) {
	req := &AuthenticateRequest{
		AuthToken:  authToken,
		SiteCode:   c.config.SiteCode,
		DeviceType: deviceType,
	}

	var resp Response[AuthenticateResult]
	if err := c.doRequest(ctx, "/authenticate", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// GetBalance retrieves the player's current balance
func (c *Client) GetBalance(ctx context.Context, sessionToken, playerID string) (*BalanceResult, error) {
	req := &BalanceRequest{
		SessionToken: sessionToken,
		SiteCode:     c.config.SiteCode,
		PlayerID:     playerID,
	}

	var resp Response[BalanceResult]
	if err := c.doRequest(ctx, "/balance", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// InitGame starts a new game session
// Returns a potentially updated session token for this game session
func (c *Client) InitGame(ctx context.Context, sessionToken, playerID, gameName string) (*InitGameResult, error) {
	req := &InitGameRequest{
		SessionToken: sessionToken,
		SiteCode:     c.config.SiteCode,
		PlayerID:     playerID,
		GameName:     gameName,
	}

	var resp Response[InitGameResult]
	if err := c.doRequest(ctx, "/init-game", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// Withdraw deducts money from the player's balance (for placing bets)
func (c *Client) Withdraw(ctx context.Context, req *WithdrawRequest) (*WithdrawResult, error) {
	// Ensure site code is set
	req.SiteCode = c.config.SiteCode

	var resp Response[WithdrawResult]
	if err := c.doRequest(ctx, "/withdraw", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// Deposit adds money to the player's balance (for wins)
func (c *Client) Deposit(ctx context.Context, req *DepositRequest) (*DepositResult, error) {
	// Ensure site code is set
	req.SiteCode = c.config.SiteCode

	var resp Response[DepositResult]
	if err := c.doRequest(ctx, "/deposit", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// WithdrawAndDeposit performs both withdraw and deposit in a single request
// Used for "no win" rounds where deposit amount is 0
func (c *Client) WithdrawAndDeposit(ctx context.Context, req *WithdrawAndDepositRequest) (*WithdrawAndDepositResult, error) {
	// Ensure site code is set
	req.SiteCode = c.config.SiteCode

	var resp Response[WithdrawAndDepositResult]
	if err := c.doRequest(ctx, "/withdraw-and-deposit", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// Cancel cancels a failed withdraw or deposit transaction
func (c *Client) Cancel(ctx context.Context, sessionToken, playerID, rgsRoundID, rgsTransactionID string) (*CancelResult, error) {
	req := &CancelRequest{
		SessionToken:     sessionToken,
		SiteCode:         c.config.SiteCode,
		PlayerID:         playerID,
		RGSRoundID:       rgsRoundID,
		RGSTransactionID: rgsTransactionID,
	}

	var resp Response[CancelResult]
	if err := c.doRequest(ctx, "/cancel", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// CreateAuthToken creates a test player auth token (DEBUG ONLY - disabled in production)
func (c *Client) CreateAuthToken(ctx context.Context, req *CreateAuthTokenRequest) (*CreateAuthTokenResult, error) {
	// Ensure site code is set
	req.SiteCode = c.config.SiteCode

	var resp Response[CreateAuthTokenResult]
	if err := c.doRequest(ctx, "/auth-token", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}
