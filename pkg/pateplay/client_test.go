package pateplay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	testAPIKey    = "test-api-key"
	testAPISecret = "test-api-secret"
	testSiteCode  = "testsite"
)

// mockServer creates a test server that validates HMAC and returns the given response
func mockServer(t *testing.T, expectedPath string, validateBody func(body []byte) error, response interface{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate method
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Validate path
		if r.URL.Path != expectedPath {
			t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		// Validate content type
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Validate API key header
		apiKey := r.Header.Get("x-api-key")
		if apiKey != testAPIKey {
			t.Errorf("Expected API key %s, got %s", testAPIKey, apiKey)
		}

		// Read body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read body: %v", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Validate HMAC
		expectedHMAC := computeTestHMAC(body)
		actualHMAC := r.Header.Get("x-api-hmac")
		if actualHMAC != expectedHMAC {
			t.Errorf("HMAC mismatch: expected %s, got %s", expectedHMAC, actualHMAC)
		}

		// Validate body content if provided
		if validateBody != nil {
			if err := validateBody(body); err != nil {
				t.Errorf("Body validation failed: %v", err)
				http.Error(w, "Bad request", http.StatusBadRequest)
				return
			}
		}

		// Return response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

// computeTestHMAC computes HMAC for testing
func computeTestHMAC(body []byte) string {
	h := hmac.New(sha256.New, []byte(testAPISecret))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// newTestClient creates a client configured for testing
func newTestClient(baseURL string) *Client {
	return NewClient(&ClientConfig{
		BaseURL:    baseURL,
		APIKey:     testAPIKey,
		APISecret:  testAPISecret,
		SiteCode:   testSiteCode,
		Timeout:    5 * time.Second,
		RetryCount: 1,
	})
}

func TestAuthenticate_Success(t *testing.T) {
	expectedResponse := Response[AuthenticateResult]{
		Result: &AuthenticateResult{
			SessionToken: "session-token-123",
			PlayerID:     "player-456",
			PlayerName:   "John Doe",
			Currency:     "USD",
			Country:      "us",
			Balance:      "1000.00",
		},
	}

	server := mockServer(t, "/authenticate", func(body []byte) error {
		var req AuthenticateRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.AuthToken != "auth-token-123" {
			t.Errorf("Expected authToken 'auth-token-123', got '%s'", req.AuthToken)
		}
		if req.SiteCode != testSiteCode {
			t.Errorf("Expected siteCode '%s', got '%s'", testSiteCode, req.SiteCode)
		}
		if req.DeviceType != DeviceTypeDesktop {
			t.Errorf("Expected deviceType 'desktop', got '%s'", req.DeviceType)
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.Authenticate(context.Background(), "auth-token-123", DeviceTypeDesktop)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.SessionToken != "session-token-123" {
		t.Errorf("Expected sessionToken 'session-token-123', got '%s'", result.SessionToken)
	}
	if result.PlayerID != "player-456" {
		t.Errorf("Expected playerID 'player-456', got '%s'", result.PlayerID)
	}
	if result.Balance != "1000.00" {
		t.Errorf("Expected balance '1000.00', got '%s'", result.Balance)
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	expectedResponse := Response[AuthenticateResult]{
		Error: &APIError{
			Code:    ErrInvalidAuthToken,
			Message: "Invalid auth token.",
		},
	}

	server := mockServer(t, "/authenticate", nil, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.Authenticate(context.Background(), "invalid-token", DeviceTypeDesktop)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}

	if apiErr.Code != ErrInvalidAuthToken {
		t.Errorf("Expected error code '%s', got '%s'", ErrInvalidAuthToken, apiErr.Code)
	}
}

func TestGetBalance_Success(t *testing.T) {
	expectedResponse := Response[BalanceResult]{
		Result: &BalanceResult{
			Balance: "5000.50",
		},
	}

	server := mockServer(t, "/balance", func(body []byte) error {
		var req BalanceRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.SessionToken != "session-123" {
			t.Errorf("Expected sessionToken 'session-123', got '%s'", req.SessionToken)
		}
		if req.PlayerID != "player-456" {
			t.Errorf("Expected playerId 'player-456', got '%s'", req.PlayerID)
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.GetBalance(context.Background(), "session-123", "player-456")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Balance != "5000.50" {
		t.Errorf("Expected balance '5000.50', got '%s'", result.Balance)
	}
}

func TestGetBalance_InvalidSession(t *testing.T) {
	expectedResponse := Response[BalanceResult]{
		Error: &APIError{
			Code:    ErrInvalidSessionToken,
			Message: "Invalid session token.",
		},
	}

	server := mockServer(t, "/balance", nil, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.GetBalance(context.Background(), "invalid-session", "player-456")

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}

	if apiErr.Code != ErrInvalidSessionToken {
		t.Errorf("Expected error code '%s', got '%s'", ErrInvalidSessionToken, apiErr.Code)
	}
}

func TestInitGame_Success(t *testing.T) {
	expectedResponse := Response[InitGameResult]{
		Result: &InitGameResult{
			SessionToken: "new-session-token",
			Balance:      "1000.00",
		},
	}

	server := mockServer(t, "/init-game", func(body []byte) error {
		var req InitGameRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.GameName != "fortune-slots" {
			t.Errorf("Expected gameName 'fortune-slots', got '%s'", req.GameName)
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.InitGame(context.Background(), "session-123", "player-456", "fortune-slots")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.SessionToken != "new-session-token" {
		t.Errorf("Expected sessionToken 'new-session-token', got '%s'", result.SessionToken)
	}
}

func TestWithdraw_Success(t *testing.T) {
	expectedResponse := Response[WithdrawResult]{
		Result: &WithdrawResult{
			TransactionID: "tx-withdraw-123",
			Balance:       "900.00",
		},
	}

	server := mockServer(t, "/withdraw", func(body []byte) error {
		var req WithdrawRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.Amount != "100.00" {
			t.Errorf("Expected amount '100.00', got '%s'", req.Amount)
		}
		if req.Reason != WithdrawReasonRoundStart {
			t.Errorf("Expected reason 'round_start', got '%s'", req.Reason)
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.Withdraw(context.Background(), &WithdrawRequest{
		SessionToken:        "session-123",
		PlayerID:            "player-456",
		Currency:            "USD",
		RGSRoundID:          "round-1",
		RGSTransactionID:    "tx-1",
		GameName:            "fortune-slots",
		Amount:              "100.00",
		JackpotContribution: "0.01",
		Reason:              WithdrawReasonRoundStart,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.TransactionID != "tx-withdraw-123" {
		t.Errorf("Expected transactionId 'tx-withdraw-123', got '%s'", result.TransactionID)
	}
	if result.Balance != "900.00" {
		t.Errorf("Expected balance '900.00', got '%s'", result.Balance)
	}
}

func TestWithdraw_InsufficientBalance(t *testing.T) {
	expectedResponse := Response[WithdrawResult]{
		Error: &APIError{
			Code:    ErrInsufficientBalance,
			Message: "Insufficient balance.",
		},
	}

	server := mockServer(t, "/withdraw", nil, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.Withdraw(context.Background(), &WithdrawRequest{
		SessionToken:     "session-123",
		PlayerID:         "player-456",
		Currency:         "USD",
		RGSRoundID:       "round-1",
		RGSTransactionID: "tx-1",
		GameName:         "fortune-slots",
		Amount:           "999999.00",
		Reason:           WithdrawReasonRoundStart,
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}

	if apiErr.Code != ErrInsufficientBalance {
		t.Errorf("Expected error code '%s', got '%s'", ErrInsufficientBalance, apiErr.Code)
	}
}

func TestWithdraw_TransactionAlreadyExists(t *testing.T) {
	expectedResponse := Response[WithdrawResult]{
		Error: &APIError{
			Code:    ErrTransactionAlreadyExists,
			Message: "Transaction already exists.",
			Data: map[string]interface{}{
				"transactionId": "existing-tx-123",
			},
		},
	}

	server := mockServer(t, "/withdraw", nil, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.Withdraw(context.Background(), &WithdrawRequest{
		SessionToken:     "session-123",
		PlayerID:         "player-456",
		Currency:         "USD",
		RGSRoundID:       "round-1",
		RGSTransactionID: "existing-tx",
		GameName:         "fortune-slots",
		Amount:           "100.00",
		Reason:           WithdrawReasonRoundStart,
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}

	if apiErr.Code != ErrTransactionAlreadyExists {
		t.Errorf("Expected error code '%s', got '%s'", ErrTransactionAlreadyExists, apiErr.Code)
	}

	if apiErr.Data["transactionId"] != "existing-tx-123" {
		t.Errorf("Expected transactionId in error data")
	}
}

func TestDeposit_Success(t *testing.T) {
	expectedResponse := Response[DepositResult]{
		Result: &DepositResult{
			TransactionID: "tx-deposit-123",
			Balance:       "1100.00",
		},
	}

	server := mockServer(t, "/deposit", func(body []byte) error {
		var req DepositRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.Amount != "200.00" {
			t.Errorf("Expected amount '200.00', got '%s'", req.Amount)
		}
		if req.Reason != DepositReasonRoundEnd {
			t.Errorf("Expected reason 'round_end', got '%s'", req.Reason)
		}
		if req.IsJackpotWin {
			t.Error("Expected isJackpotWin to be false")
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.Deposit(context.Background(), &DepositRequest{
		SessionToken:     "session-123",
		PlayerID:         "player-456",
		GameName:         "fortune-slots",
		Currency:         "USD",
		RGSRoundID:       "round-1",
		RGSTransactionID: "tx-2",
		Amount:           "200.00",
		IsJackpotWin:     false,
		Reason:           DepositReasonRoundEnd,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.TransactionID != "tx-deposit-123" {
		t.Errorf("Expected transactionId 'tx-deposit-123', got '%s'", result.TransactionID)
	}
	if result.Balance != "1100.00" {
		t.Errorf("Expected balance '1100.00', got '%s'", result.Balance)
	}
}

func TestDeposit_JackpotWin(t *testing.T) {
	expectedResponse := Response[DepositResult]{
		Result: &DepositResult{
			TransactionID: "tx-jackpot-123",
			Balance:       "11000.00",
		},
	}

	server := mockServer(t, "/deposit", func(body []byte) error {
		var req DepositRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if !req.IsJackpotWin {
			t.Error("Expected isJackpotWin to be true")
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.Deposit(context.Background(), &DepositRequest{
		SessionToken:     "session-123",
		PlayerID:         "player-456",
		GameName:         "fortune-slots",
		Currency:         "USD",
		RGSRoundID:       "round-1",
		RGSTransactionID: "tx-jackpot",
		Amount:           "10000.00",
		IsJackpotWin:     true,
		Reason:           DepositReasonRoundEnd,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Balance != "11000.00" {
		t.Errorf("Expected balance '11000.00', got '%s'", result.Balance)
	}
}

func TestWithdrawAndDeposit_Success(t *testing.T) {
	expectedResponse := Response[WithdrawAndDepositResult]{
		Result: &WithdrawAndDepositResult{
			Balance:               "999.00",
			WithdrawTransactionID: "tx-withdraw-456",
			DepositTransactionID:  "tx-deposit-789",
		},
	}

	server := mockServer(t, "/withdraw-and-deposit", func(body []byte) error {
		var req WithdrawAndDepositRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.WithdrawAmount != "1.00" {
			t.Errorf("Expected withdrawAmount '1.00', got '%s'", req.WithdrawAmount)
		}
		if req.DepositAmount != "0.00" {
			t.Errorf("Expected depositAmount '0.00', got '%s'", req.DepositAmount)
		}
		if req.WithdrawReason != WithdrawReasonRoundStart {
			t.Errorf("Expected withdrawReason 'round_start', got '%s'", req.WithdrawReason)
		}
		if req.DepositReason != DepositReasonRoundEnd {
			t.Errorf("Expected depositReason 'round_end', got '%s'", req.DepositReason)
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.WithdrawAndDeposit(context.Background(), &WithdrawAndDepositRequest{
		SessionToken:             "session-123",
		PlayerID:                 "player-456",
		GameName:                 "fortune-slots",
		Currency:                 "USD",
		RGSRoundID:               "round-2",
		RGSWithdrawTransactionID: "tx-w-1",
		RGSDepositTransactionID:  "tx-d-1",
		WithdrawAmount:           "1.00",
		DepositAmount:            "0.00",
		JackpotContribution:      "0.01",
		WithdrawReason:           WithdrawReasonRoundStart,
		DepositReason:            DepositReasonRoundEnd,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Balance != "999.00" {
		t.Errorf("Expected balance '999.00', got '%s'", result.Balance)
	}
	if result.WithdrawTransactionID != "tx-withdraw-456" {
		t.Errorf("Expected withdrawTransactionId 'tx-withdraw-456', got '%s'", result.WithdrawTransactionID)
	}
	if result.DepositTransactionID != "tx-deposit-789" {
		t.Errorf("Expected depositTransactionId 'tx-deposit-789', got '%s'", result.DepositTransactionID)
	}
}

func TestCancel_Success(t *testing.T) {
	expectedResponse := Response[CancelResult]{
		Result: &CancelResult{
			TransactionID: "cancelled-tx-123",
		},
	}

	server := mockServer(t, "/cancel", func(body []byte) error {
		var req CancelRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.RGSRoundID != "round-1" {
			t.Errorf("Expected rgsRoundId 'round-1', got '%s'", req.RGSRoundID)
		}
		if req.RGSTransactionID != "tx-to-cancel" {
			t.Errorf("Expected rgsTransactionId 'tx-to-cancel', got '%s'", req.RGSTransactionID)
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.Cancel(context.Background(), "session-123", "player-456", "round-1", "tx-to-cancel")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.TransactionID != "cancelled-tx-123" {
		t.Errorf("Expected transactionId 'cancelled-tx-123', got '%s'", result.TransactionID)
	}
}

func TestCancel_TransactionNotFound(t *testing.T) {
	expectedResponse := Response[CancelResult]{
		Error: &APIError{
			Code:    ErrTransactionNotFound,
			Message: "Transaction not found.",
		},
	}

	server := mockServer(t, "/cancel", nil, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	_, err := client.Cancel(context.Background(), "session-123", "player-456", "round-1", "nonexistent-tx")

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}

	if apiErr.Code != ErrTransactionNotFound {
		t.Errorf("Expected error code '%s', got '%s'", ErrTransactionNotFound, apiErr.Code)
	}
}

func TestCreateAuthToken_NewPlayer(t *testing.T) {
	expectedResponse := Response[CreateAuthTokenResult]{
		Result: &CreateAuthTokenResult{
			PlayerID:   "new-player-123",
			PlayerName: "Test Player",
			Currency:   "USD",
			Country:    "us",
			Balance:    "10000.00",
			AuthToken:  "new-auth-token-456",
		},
	}

	server := mockServer(t, "/auth-token", func(body []byte) error {
		var req CreateAuthTokenRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.PlayerName != "Test Player" {
			t.Errorf("Expected playerName 'Test Player', got '%s'", req.PlayerName)
		}
		if req.Currency != "USD" {
			t.Errorf("Expected currency 'USD', got '%s'", req.Currency)
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.CreateAuthToken(context.Background(), &CreateAuthTokenRequest{
		PlayerName: "Test Player",
		Currency:   "USD",
		Balance:    "10000.00",
		Country:    "us",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.PlayerID != "new-player-123" {
		t.Errorf("Expected playerId 'new-player-123', got '%s'", result.PlayerID)
	}
	if result.AuthToken != "new-auth-token-456" {
		t.Errorf("Expected authToken 'new-auth-token-456', got '%s'", result.AuthToken)
	}
}

func TestCreateAuthToken_ExistingPlayer(t *testing.T) {
	expectedResponse := Response[CreateAuthTokenResult]{
		Result: &CreateAuthTokenResult{
			PlayerID:   "existing-player-789",
			PlayerName: "Existing Player",
			Currency:   "EUR",
			Country:    "de",
			Balance:    "5000.00",
			AuthToken:  "existing-auth-token-123",
		},
	}

	server := mockServer(t, "/auth-token", func(body []byte) error {
		var req CreateAuthTokenRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		if req.PlayerID != "existing-player-789" {
			t.Errorf("Expected playerId 'existing-player-789', got '%s'", req.PlayerID)
		}
		return nil
	}, expectedResponse)
	defer server.Close()

	client := newTestClient(server.URL)
	result, err := client.CreateAuthToken(context.Background(), &CreateAuthTokenRequest{
		PlayerID: "existing-player-789",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.AuthToken != "existing-auth-token-123" {
		t.Errorf("Expected authToken 'existing-auth-token-123', got '%s'", result.AuthToken)
	}
}

func TestHMACComputation(t *testing.T) {
	client := NewClient(&ClientConfig{
		APISecret: "my-secret-key",
	})

	body := []byte(`{"test":"data"}`)
	hmacResult := client.computeHMAC(body)

	// Compute expected HMAC
	h := hmac.New(sha256.New, []byte("my-secret-key"))
	h.Write(body)
	expected := hex.EncodeToString(h.Sum(nil))

	if hmacResult != expected {
		t.Errorf("HMAC mismatch: expected %s, got %s", expected, hmacResult)
	}
}

func TestClient_NetworkError(t *testing.T) {
	// Use invalid URL to simulate network error
	client := NewClient(&ClientConfig{
		BaseURL:    "http://localhost:99999",
		APIKey:     testAPIKey,
		APISecret:  testAPISecret,
		SiteCode:   testSiteCode,
		Timeout:    1 * time.Second,
		RetryCount: 1,
	})

	_, err := client.Authenticate(context.Background(), "token", DeviceTypeDesktop)

	if err == nil {
		t.Fatal("Expected error for network failure, got nil")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Delay response
	}))
	defer server.Close()

	client := newTestClient(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Authenticate(ctx, "token", DeviceTypeDesktop)

	if err == nil {
		t.Fatal("Expected context deadline error, got nil")
	}
}

func TestAPIError_Error(t *testing.T) {
	apiErr := &APIError{
		Code:    ErrInsufficientBalance,
		Message: "Not enough money",
	}

	if apiErr.Error() != "Not enough money" {
		t.Errorf("Expected error message 'Not enough money', got '%s'", apiErr.Error())
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Timeout != 30*time.Second {
		t.Errorf("Expected default timeout 30s, got %v", config.Timeout)
	}
	if config.RetryCount != 3 {
		t.Errorf("Expected default retry count 3, got %d", config.RetryCount)
	}
}

func TestNewClientWithHTTPClient(t *testing.T) {
	customClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	config := &ClientConfig{
		BaseURL:   "http://localhost:8080",
		APIKey:    "key",
		APISecret: "secret",
		SiteCode:  "site",
	}

	client := NewClientWithHTTPClient(config, customClient)

	if client.httpClient != customClient {
		t.Error("Expected custom HTTP client to be used")
	}
}
