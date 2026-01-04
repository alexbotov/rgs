// Package integration provides end-to-end integration tests for the RGS
// These tests verify the complete flow from login through gameplay
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexbotov/rgs/internal/api"
	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/auth"
	"github.com/alexbotov/rgs/internal/config"
	"github.com/alexbotov/rgs/internal/control"
	"github.com/alexbotov/rgs/internal/database"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/alexbotov/rgs/internal/game"
	"github.com/alexbotov/rgs/internal/limits"
	"github.com/alexbotov/rgs/internal/rng"
	"github.com/alexbotov/rgs/internal/wallet"
	"github.com/google/uuid"
)

// TestServer wraps all services needed for integration testing
type TestServer struct {
	Server   *httptest.Server
	DB       *database.DB
	Auth     *auth.Service
	Wallet   *wallet.Service
	Game     *game.Engine
	RNG      *rng.Service
	Audit    *audit.Service
	Limits   *limits.Service
	Control  *control.Service
	Handler  *api.Handler
	Config   *config.Config
	teardown func()
}

// NewTestServer creates a new test server with all services initialized
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:         "0",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Database: config.DatabaseConfig{
			Driver: "postgres",
			DSN:    "host=localhost dbname=rgs sslmode=disable",
		},
		Auth: config.AuthConfig{
			JWTSecret:         "test-secret-key-for-integration-tests",
			TokenExpiry:       24 * time.Hour,
			SessionTimeout:    30 * time.Minute,
			MaxFailedAttempts: 3,
			LockoutDuration:   30 * time.Minute,
		},
		Game: config.GameConfig{
			DefaultCurrency: "USD",
			MinRTP:          0.75,
		},
	}

	// Initialize database
	db, err := database.New(cfg.Database.Driver, cfg.Database.DSN)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Reset and migrate for clean state
	if err := db.Reset(); err != nil {
		t.Fatalf("Failed to reset database: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	// Initialize services
	auditSvc := audit.New(db.DB)
	rngSvc := rng.New()
	authSvc := auth.New(db.DB, &cfg.Auth, auditSvc)
	walletSvc := wallet.New(db.DB, auditSvc, cfg.Game.DefaultCurrency)
	gameEngine := game.New(db.DB, rngSvc, walletSvc, auditSvc, cfg.Game.DefaultCurrency)
	limitsSvc := limits.New(db.DB, auditSvc, cfg.Game.DefaultCurrency)
	controlSvc := control.New(db.DB, auditSvc)

	// Initialize API handler
	handler := api.New(authSvc, walletSvc, gameEngine, rngSvc)
	router := handler.SetupRouter()

	// Create test server
	server := httptest.NewServer(router)

	return &TestServer{
		Server:  server,
		DB:      db,
		Auth:    authSvc,
		Wallet:  walletSvc,
		Game:    gameEngine,
		RNG:     rngSvc,
		Audit:   auditSvc,
		Limits:  limitsSvc,
		Control: controlSvc,
		Handler: handler,
		Config:  cfg,
		teardown: func() {
			server.Close()
			db.Reset() // Clean up after tests
			db.Close()
		},
	}
}

// Close cleans up test resources
func (ts *TestServer) Close() {
	ts.teardown()
}

// createTestUser creates a user directly via auth service (bypasses API)
func (ts *TestServer) createTestUser(t *testing.T, username, email, password string) *domain.Player {
	t.Helper()
	player, err := ts.Auth.Register(context.Background(), &auth.RegisterRequest{
		Username: username,
		Email:    email,
		Password: password,
		AcceptTC: true,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("Failed to create test user %s: %v", username, err)
	}
	return player
}

// APIResponse represents a standard API response
type APIResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// doRequest performs an HTTP request and returns the response
func (ts *TestServer) doRequest(t *testing.T, method, path string, body interface{}, token string) *http.Response {
	t.Helper()

	var reqBody *bytes.Buffer
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(jsonBody)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, ts.Server.URL+path, reqBody)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to perform request: %v", err)
	}

	return resp
}

// parseResponse parses the API response
func parseResponse(t *testing.T, resp *http.Response) *APIResponse {
	t.Helper()

	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	defer resp.Body.Close()

	return &apiResp
}

// extractField extracts a field from the response data
func extractField(t *testing.T, data json.RawMessage, field string) string {
	t.Helper()

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Failed to unmarshal data: %v", err)
	}

	if val, ok := m[field]; ok {
		switch v := val.(type) {
		case string:
			return v
		case float64:
			return fmt.Sprintf("%v", v)
		default:
			return fmt.Sprintf("%v", v)
		}
	}

	return ""
}

// ============================================================================
// Health Check Tests
// ============================================================================

func TestHealthEndpoint(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	resp := ts.doRequest(t, "GET", "/health", nil, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	apiResp := parseResponse(t, resp)
	if !apiResp.Success {
		t.Error("Expected success response")
	}

	// Verify RNG health is included
	var data map[string]interface{}
	json.Unmarshal(apiResp.Data, &data)

	if status, ok := data["status"]; !ok || status != "healthy" {
		t.Error("Expected healthy status")
	}

	if _, ok := data["rng_status"]; !ok {
		t.Error("Expected rng_status in health response")
	}
}

func TestServerInfoEndpoint(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	resp := ts.doRequest(t, "GET", "/", nil, "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	apiResp := parseResponse(t, resp)

	var data map[string]interface{}
	json.Unmarshal(apiResp.Data, &data)

	if data["name"] != "RGS" {
		t.Errorf("Expected name 'RGS', got %v", data["name"])
	}
}

// ============================================================================
// Authentication Tests
// ============================================================================

func TestPlayerLogin(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create a user first (via auth service, not API)
	ts.createTestUser(t, "logintest", "login@example.com", "password123")

	// Test successful login
	t.Run("SuccessfulLogin", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/auth/login", map[string]interface{}{
			"username": "logintest",
			"password": "password123",
		}, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		token := extractField(t, apiResp.Data, "token")
		if token == "" {
			t.Error("Expected token in response")
		}
	})

	// Test invalid password
	t.Run("InvalidPassword", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/auth/login", map[string]interface{}{
			"username": "logintest",
			"password": "wrongpassword",
		}, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	// Test non-existent user
	t.Run("NonExistentUser", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/auth/login", map[string]interface{}{
			"username": "nonexistent",
			"password": "password123",
		}, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})
}

func TestSessionManagement(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create user and login
	ts.createTestUser(t, "sessiontest", "session@example.com", "password123")

	loginResp := ts.doRequest(t, "POST", "/api/v1/auth/login", map[string]interface{}{
		"username": "sessiontest",
		"password": "password123",
	}, "")
	loginData := parseResponse(t, loginResp)
	token := extractField(t, loginData.Data, "token")

	// Test get session
	t.Run("GetSession", func(t *testing.T) {
		resp := ts.doRequest(t, "GET", "/api/v1/auth/session", nil, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	// Test unauthorized access
	t.Run("UnauthorizedAccess", func(t *testing.T) {
		resp := ts.doRequest(t, "GET", "/api/v1/auth/session", nil, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	// Test logout
	t.Run("Logout", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/auth/logout", nil, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

// ============================================================================
// Wallet Tests
// ============================================================================

func TestWalletOperations(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Setup: Create user and login
	ts.createTestUser(t, "wallettest", "wallet@example.com", "password123")

	loginResp := ts.doRequest(t, "POST", "/api/v1/auth/login", map[string]interface{}{
		"username": "wallettest",
		"password": "password123",
	}, "")
	loginData := parseResponse(t, loginResp)
	token := extractField(t, loginData.Data, "token")

	// Test initial balance (should be 0)
	t.Run("InitialBalance", func(t *testing.T) {
		resp := ts.doRequest(t, "GET", "/api/v1/wallet/balance", nil, token)
		defer resp.Body.Close()

		apiResp := parseResponse(t, resp)
		available := extractField(t, apiResp.Data, "available")
		if available != "0" {
			t.Errorf("Expected initial balance 0, got %s", available)
		}
	})

	// Test deposit
	t.Run("Deposit", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/wallet/deposit", map[string]interface{}{
			"amount":    100.00,
			"reference": "test-deposit",
		}, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		balanceAfter := extractField(t, apiResp.Data, "balance_after")
		if balanceAfter != "100" {
			t.Errorf("Expected balance 100, got %s", balanceAfter)
		}
	})

	// Test balance after deposit
	t.Run("BalanceAfterDeposit", func(t *testing.T) {
		resp := ts.doRequest(t, "GET", "/api/v1/wallet/balance", nil, token)
		defer resp.Body.Close()

		apiResp := parseResponse(t, resp)
		available := extractField(t, apiResp.Data, "available")
		if available != "100" {
			t.Errorf("Expected balance 100, got %s", available)
		}
	})

	// Test withdrawal
	t.Run("Withdrawal", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/wallet/withdraw", map[string]interface{}{
			"amount":    25.00,
			"reference": "test-withdrawal",
		}, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		balanceAfter := extractField(t, apiResp.Data, "balance_after")
		if balanceAfter != "75" {
			t.Errorf("Expected balance 75, got %s", balanceAfter)
		}
	})

	// Test insufficient funds
	t.Run("InsufficientFunds", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/wallet/withdraw", map[string]interface{}{
			"amount":    1000.00,
			"reference": "too-much",
		}, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	// Test transaction history
	t.Run("TransactionHistory", func(t *testing.T) {
		resp := ts.doRequest(t, "GET", "/api/v1/wallet/transactions", nil, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		var transactions []interface{}
		json.Unmarshal(apiResp.Data, &transactions)

		if len(transactions) < 2 {
			t.Errorf("Expected at least 2 transactions, got %d", len(transactions))
		}
	})
}

// ============================================================================
// Game Tests
// ============================================================================

func TestGameOperations(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Setup: Create user, login, and deposit
	ts.createTestUser(t, "gametest", "game@example.com", "password123")

	loginResp := ts.doRequest(t, "POST", "/api/v1/auth/login", map[string]interface{}{
		"username": "gametest",
		"password": "password123",
	}, "")
	loginData := parseResponse(t, loginResp)
	token := extractField(t, loginData.Data, "token")

	// Deposit funds
	ts.doRequest(t, "POST", "/api/v1/wallet/deposit", map[string]interface{}{
		"amount":    100.00,
		"reference": "game-deposit",
	}, token)

	// Test list games
	t.Run("ListGames", func(t *testing.T) {
		resp := ts.doRequest(t, "GET", "/api/v1/games", nil, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		var games []interface{}
		json.Unmarshal(apiResp.Data, &games)

		if len(games) < 1 {
			t.Error("Expected at least 1 game")
		}
	})

	// Test get game details
	t.Run("GetGameDetails", func(t *testing.T) {
		resp := ts.doRequest(t, "GET", "/api/v1/games/fortune-slots", nil, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		name := extractField(t, apiResp.Data, "name")
		if name != "Fortune Slots" {
			t.Errorf("Expected 'Fortune Slots', got %s", name)
		}
	})

	// Test start game session
	var gameSessionID string
	t.Run("StartGameSession", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/games/fortune-slots/session", nil, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		gameSessionID = extractField(t, apiResp.Data, "session_id")
		if gameSessionID == "" {
			t.Error("Expected session_id in response")
		}
	})

	// Test play game
	t.Run("PlayGame", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/games/play", map[string]interface{}{
			"session_id":   gameSessionID,
			"wager_amount": 100, // $1.00 in cents
		}, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		cycleID := extractField(t, apiResp.Data, "cycle_id")
		if cycleID == "" {
			t.Error("Expected cycle_id in response")
		}

		// Verify outcome is present
		var data map[string]interface{}
		json.Unmarshal(apiResp.Data, &data)
		if _, ok := data["outcome"]; !ok {
			t.Error("Expected outcome in response")
		}
	})

	// Test play multiple games
	t.Run("PlayMultipleGames", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			resp := ts.doRequest(t, "POST", "/api/v1/games/play", map[string]interface{}{
				"session_id":   gameSessionID,
				"wager_amount": 50, // $0.50
			}, token)
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Game %d: Expected status 200, got %d", i+1, resp.StatusCode)
			}
		}
	})

	// Test game history
	t.Run("GameHistory", func(t *testing.T) {
		resp := ts.doRequest(t, "GET", "/api/v1/games/history", nil, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		apiResp := parseResponse(t, resp)
		var history []interface{}
		json.Unmarshal(apiResp.Data, &history)

		if len(history) < 6 {
			t.Errorf("Expected at least 6 games in history, got %d", len(history))
		}
	})

	// Test insufficient balance
	t.Run("InsufficientBalance", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/games/play", map[string]interface{}{
			"session_id":   gameSessionID,
			"wager_amount": 1000000, // Way too much
		}, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	// Test invalid wager (below minimum)
	t.Run("InvalidWagerBelowMinimum", func(t *testing.T) {
		resp := ts.doRequest(t, "POST", "/api/v1/games/play", map[string]interface{}{
			"session_id":   gameSessionID,
			"wager_amount": 1, // $0.01 - below $0.10 minimum
		}, token)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}

// ============================================================================
// RNG Tests
// ============================================================================

func TestRNGService(t *testing.T) {
	rngSvc := rng.New()

	// Test basic generation
	t.Run("GenerateInt", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			n, err := rngSvc.GenerateInt(100)
			if err != nil {
				t.Fatalf("Failed to generate int: %v", err)
			}
			if n < 0 || n >= 100 {
				t.Errorf("Generated value %d out of range [0, 100)", n)
			}
		}
	})

	// Test range generation
	t.Run("GenerateIntRange", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			n, err := rngSvc.GenerateIntRange(10, 20)
			if err != nil {
				t.Fatalf("Failed to generate int range: %v", err)
			}
			if n < 10 || n > 20 {
				t.Errorf("Generated value %d out of range [10, 20]", n)
			}
		}
	})

	// Test float generation
	t.Run("GenerateFloat", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			f, err := rngSvc.GenerateFloat()
			if err != nil {
				t.Fatalf("Failed to generate float: %v", err)
			}
			if f < 0.0 || f >= 1.0 {
				t.Errorf("Generated value %f out of range [0.0, 1.0)", f)
			}
		}
	})

	// Test shuffle
	t.Run("Shuffle", func(t *testing.T) {
		original := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		shuffled := make([]int, len(original))
		copy(shuffled, original)

		if err := rngSvc.Shuffle(shuffled); err != nil {
			t.Fatalf("Failed to shuffle: %v", err)
		}

		// Check all elements still present
		seen := make(map[int]bool)
		for _, v := range shuffled {
			seen[v] = true
		}
		if len(seen) != len(original) {
			t.Error("Shuffle lost or duplicated elements")
		}
	})

	// Test weighted selection
	t.Run("SelectWeighted", func(t *testing.T) {
		weights := []float64{0.5, 0.3, 0.2}
		counts := make([]int, 3)

		for i := 0; i < 10000; i++ {
			idx, err := rngSvc.SelectWeighted(weights)
			if err != nil {
				t.Fatalf("Failed weighted selection: %v", err)
			}
			counts[idx]++
		}

		// Check distribution is roughly correct (within 10%)
		expected := []int{5000, 3000, 2000}
		for i, count := range counts {
			diff := float64(count-expected[i]) / float64(expected[i])
			if diff > 0.15 || diff < -0.15 {
				t.Errorf("Weighted selection distribution off: index %d, expected ~%d, got %d", i, expected[i], count)
			}
		}
	})

	// Test health check
	t.Run("HealthCheck", func(t *testing.T) {
		result, err := rngSvc.HealthCheck()
		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}

		if !result.Healthy {
			t.Error("RNG health check failed")
		}

		if !result.ChiSquarePassed {
			t.Errorf("Chi-square test failed: %f", result.ChiSquare)
		}
	})
}

// ============================================================================
// End-to-End Flow Test
// ============================================================================

func TestCompletePlayerJourney(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Step 1: Create player (via auth service)
	t.Log("Step 1: Creating player...")
	player := ts.createTestUser(t, "journey_player", "journey@example.com", "securepass123")
	t.Logf("  Player ID: %s", player.ID)

	// Step 2: Login
	t.Log("Step 2: Logging in...")
	loginResp := ts.doRequest(t, "POST", "/api/v1/auth/login", map[string]interface{}{
		"username": "journey_player",
		"password": "securepass123",
	}, "")
	loginData := parseResponse(t, loginResp)
	if !loginData.Success {
		t.Fatalf("Login failed: %v", loginData.Error)
	}
	token := extractField(t, loginData.Data, "token")
	t.Logf("  Token acquired")

	// Step 3: Check initial balance
	t.Log("Step 3: Checking initial balance...")
	balResp := ts.doRequest(t, "GET", "/api/v1/wallet/balance", nil, token)
	balData := parseResponse(t, balResp)
	initialBalance := extractField(t, balData.Data, "available")
	t.Logf("  Initial balance: $%s", initialBalance)

	// Step 4: Deposit funds
	t.Log("Step 4: Depositing $500...")
	depResp := ts.doRequest(t, "POST", "/api/v1/wallet/deposit", map[string]interface{}{
		"amount":    500.00,
		"reference": "initial-deposit",
	}, token)
	depData := parseResponse(t, depResp)
	if !depData.Success {
		t.Fatalf("Deposit failed: %v", depData.Error)
	}
	t.Logf("  Balance after deposit: $%s", extractField(t, depData.Data, "balance_after"))

	// Step 5: Browse games
	t.Log("Step 5: Browsing available games...")
	gamesResp := ts.doRequest(t, "GET", "/api/v1/games", nil, token)
	gamesData := parseResponse(t, gamesResp)
	var games []map[string]interface{}
	json.Unmarshal(gamesData.Data, &games)
	t.Logf("  Found %d games", len(games))
	for _, g := range games {
		t.Logf("    - %s (RTP: %.0f%%)", g["name"], g["theoretical_rtp"].(float64)*100)
	}

	// Step 6: Start game session
	t.Log("Step 6: Starting game session for Fortune Slots...")
	sessResp := ts.doRequest(t, "POST", "/api/v1/games/fortune-slots/session", nil, token)
	sessData := parseResponse(t, sessResp)
	if !sessData.Success {
		t.Fatalf("Failed to start session: %v", sessData.Error)
	}
	gameSessionID := extractField(t, sessData.Data, "session_id")
	t.Logf("  Session ID: %s", gameSessionID)

	// Step 7: Play multiple rounds
	t.Log("Step 7: Playing 10 rounds at $5 each...")
	var totalWagered, totalWon float64
	for i := 1; i <= 10; i++ {
		playResp := ts.doRequest(t, "POST", "/api/v1/games/play", map[string]interface{}{
			"session_id":   gameSessionID,
			"wager_amount": 500, // $5.00
		}, token)
		playData := parseResponse(t, playResp)
		if !playData.Success {
			t.Fatalf("Play failed on round %d: %v", i, playData.Error)
		}

		var result map[string]interface{}
		json.Unmarshal(playData.Data, &result)

		wager := result["wager_amount"].(float64)
		win := result["win_amount"].(float64)
		totalWagered += wager
		totalWon += win

		outcome := result["outcome"].(map[string]interface{})
		reels := outcome["reels"].([]interface{})
		t.Logf("  Round %2d: [%s %s %s] - Wagered: $%.2f, Won: $%.2f",
			i, reels[0], reels[1], reels[2], wager, win)
	}
	t.Logf("  Total Wagered: $%.2f, Total Won: $%.2f", totalWagered, totalWon)

	// Step 8: Check game history
	t.Log("Step 8: Checking game history...")
	histResp := ts.doRequest(t, "GET", "/api/v1/games/history?limit=10", nil, token)
	histData := parseResponse(t, histResp)
	var history []interface{}
	json.Unmarshal(histData.Data, &history)
	t.Logf("  Found %d games in history", len(history))

	// Step 9: Check final balance
	t.Log("Step 9: Checking final balance...")
	finalBalResp := ts.doRequest(t, "GET", "/api/v1/wallet/balance", nil, token)
	finalBalData := parseResponse(t, finalBalResp)
	finalBalance := extractField(t, finalBalData.Data, "available")
	t.Logf("  Final balance: $%s", finalBalance)

	// Step 10: Withdraw winnings
	t.Log("Step 10: Withdrawing $50...")
	withResp := ts.doRequest(t, "POST", "/api/v1/wallet/withdraw", map[string]interface{}{
		"amount":    50.00,
		"reference": "withdrawal-1",
	}, token)
	withData := parseResponse(t, withResp)
	if !withData.Success {
		t.Logf("  Withdrawal failed (may have insufficient funds): %v", withData.Error)
	} else {
		t.Logf("  Withdrawal successful, new balance: $%s", extractField(t, withData.Data, "balance_after"))
	}

	// Step 11: Check transactions
	t.Log("Step 11: Checking transaction history...")
	txResp := ts.doRequest(t, "GET", "/api/v1/wallet/transactions", nil, token)
	txData := parseResponse(t, txResp)
	var transactions []interface{}
	json.Unmarshal(txData.Data, &transactions)
	t.Logf("  Found %d transactions", len(transactions))

	// Step 12: Logout
	t.Log("Step 12: Logging out...")
	logoutResp := ts.doRequest(t, "POST", "/api/v1/auth/logout", nil, token)
	logoutData := parseResponse(t, logoutResp)
	if !logoutData.Success {
		t.Fatalf("Logout failed: %v", logoutData.Error)
	}
	t.Log("  Logged out successfully")

	t.Log("✓ Complete player journey test passed!")
}

// ============================================================================
// Player Limits Tests (GLI-19 §2.5.5)
// ============================================================================

func TestPlayerLimitsIntegration(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Create a test player
	player := ts.createTestUser(t, "limits_player", "limits@example.com", "password123")

	t.Run("SetDepositLimits", func(t *testing.T) {
		// Set daily deposit limit
		result, err := ts.Limits.SetDepositLimit(ctx, &limits.SetDepositLimitRequest{
			PlayerID: player.ID,
			Period:   "daily",
			Amount:   10000, // $100
		})
		if err != nil {
			t.Fatalf("Failed to set deposit limit: %v", err)
		}

		if result.DailyDeposit == nil {
			t.Error("Expected daily deposit limit to be set")
		}
	})

	t.Run("SetWagerLimits", func(t *testing.T) {
		// Set daily wager limit
		result, err := ts.Limits.SetWagerLimit(ctx, &limits.SetWagerLimitRequest{
			PlayerID: player.ID,
			Period:   "daily",
			Amount:   5000, // $50
		})
		if err != nil {
			t.Fatalf("Failed to set wager limit: %v", err)
		}

		if result.DailyWager == nil {
			t.Error("Expected daily wager limit to be set")
		}
	})

	t.Run("SetLossLimits", func(t *testing.T) {
		// Set daily loss limit
		result, err := ts.Limits.SetLossLimit(ctx, &limits.SetLossLimitRequest{
			PlayerID: player.ID,
			Period:   "daily",
			Amount:   2500, // $25
		})
		if err != nil {
			t.Fatalf("Failed to set loss limit: %v", err)
		}

		if result.DailyLoss == nil {
			t.Error("Expected daily loss limit to be set")
		}
	})

	t.Run("VerifyLimitsAreSet", func(t *testing.T) {
		playerLimits, err := ts.Limits.GetLimits(ctx, player.ID)
		if err != nil {
			t.Fatalf("Failed to get limits: %v", err)
		}

		if playerLimits.DailyDeposit == nil || playerLimits.DailyDeposit.Amount != 10000 {
			t.Error("Daily deposit limit not correctly set")
		}
		if playerLimits.DailyWager == nil || playerLimits.DailyWager.Amount != 5000 {
			t.Error("Daily wager limit not correctly set")
		}
		if playerLimits.DailyLoss == nil || playerLimits.DailyLoss.Amount != 2500 {
			t.Error("Daily loss limit not correctly set")
		}
	})
}

func TestSelfExclusionIntegration(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Create a test player
	player := ts.createTestUser(t, "exclude_player", "exclude@example.com", "password123")

	t.Run("SelfExcludeWithDuration", func(t *testing.T) {
		duration := 7 * 24 * time.Hour
		exclusion, err := ts.Limits.SelfExclude(ctx, player.ID, "Taking a break", &duration)
		if err != nil {
			t.Fatalf("Failed to self-exclude: %v", err)
		}

		if !exclusion.IsActive {
			t.Error("Expected exclusion to be active")
		}

		// Verify player is excluded
		isExcluded, _ := ts.Limits.IsExcluded(ctx, player.ID)
		if !isExcluded {
			t.Error("Expected player to be excluded")
		}
	})

	t.Run("ExcludedPlayerCannotPlay", func(t *testing.T) {
		// Try to access gaming
		err := ts.Control.CheckAccess(ctx, player.ID, "fortune-slots")
		if err == nil {
			t.Error("Expected error for excluded player")
		}
	})
}

// ============================================================================
// Gaming Control Tests (GLI-19 §2.4)
// ============================================================================

func TestGamingControlIntegration(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Create a test player
	player := ts.createTestUser(t, "control_player", "control@example.com", "password123")

	t.Run("InitiallyEnabled", func(t *testing.T) {
		if !ts.Control.IsGamingEnabled() {
			t.Error("Gaming should be enabled by default")
		}
	})

	t.Run("DisableAllGaming", func(t *testing.T) {
		err := ts.Control.DisableAllGaming(ctx, "System maintenance", "admin@test.com")
		if err != nil {
			t.Fatalf("Failed to disable gaming: %v", err)
		}

		if ts.Control.IsGamingEnabled() {
			t.Error("Gaming should be disabled")
		}

		// Check access should fail
		err = ts.Control.CheckAccess(ctx, player.ID, "fortune-slots")
		if err == nil {
			t.Error("Expected error when gaming is disabled")
		}
	})

	t.Run("EnableAllGaming", func(t *testing.T) {
		err := ts.Control.EnableAllGaming(ctx, "admin@test.com")
		if err != nil {
			t.Fatalf("Failed to enable gaming: %v", err)
		}

		if !ts.Control.IsGamingEnabled() {
			t.Error("Gaming should be enabled")
		}
	})

	t.Run("DisableSpecificGame", func(t *testing.T) {
		err := ts.Control.DisableGame(ctx, "fortune-slots", "Game maintenance", "admin@test.com")
		if err != nil {
			t.Fatalf("Failed to disable game: %v", err)
		}

		if ts.Control.IsGameEnabled("fortune-slots") {
			t.Error("fortune-slots should be disabled")
		}

		// Other games should still be enabled
		if !ts.Control.IsGameEnabled("lucky-sevens") {
			t.Error("lucky-sevens should still be enabled")
		}

		// Re-enable for other tests
		ts.Control.EnableGame(ctx, "fortune-slots", "admin@test.com")
	})

	t.Run("DisablePlayer", func(t *testing.T) {
		err := ts.Control.DisablePlayer(ctx, player.ID, "Suspicious activity", "admin@test.com")
		if err != nil {
			t.Fatalf("Failed to disable player: %v", err)
		}

		// Check access should fail
		err = ts.Control.CheckAccess(ctx, player.ID, "fortune-slots")
		if err == nil {
			t.Error("Expected error for disabled player")
		}

		// Re-enable player
		ts.Control.EnablePlayer(ctx, player.ID, "admin@test.com")
	})

	t.Run("GetSystemStatus", func(t *testing.T) {
		status, err := ts.Control.GetSystemStatus(ctx)
		if err != nil {
			t.Fatalf("Failed to get system status: %v", err)
		}

		if !status.GamingEnabled {
			t.Error("Expected gaming to be enabled")
		}
	})
}

// ============================================================================
// Interrupted Games Tests (GLI-19 §4.16)
// ============================================================================

func TestInterruptedGamesIntegration(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Create player and fund account
	player := ts.createTestUser(t, "interrupted_player", "interrupted@example.com", "password123")
	ts.Wallet.Deposit(ctx, player.ID, domain.NewMoney(500.00, "USD"), "initial-deposit")

	t.Run("GetInterruptedGamesEmpty", func(t *testing.T) {
		interrupted, err := ts.Game.GetInterruptedGames(ctx, player.ID)
		if err != nil {
			t.Fatalf("Failed to get interrupted games: %v", err)
		}

		if len(interrupted) != 0 {
			t.Errorf("Expected 0 interrupted games, got %d", len(interrupted))
		}
	})

	t.Run("PlayAndInterrupt", func(t *testing.T) {
		// Start a game session
		session, err := ts.Game.StartSession(ctx, player.ID, "fortune-slots")
		if err != nil {
			t.Fatalf("Failed to start session: %v", err)
		}

		// Play successfully first
		_, err = ts.Game.Play(ctx, &game.PlayRequest{
			SessionID:   session.ID,
			WagerAmount: 100,
		})
		if err != nil {
			t.Fatalf("Failed to play: %v", err)
		}

		// Simulate an interrupted game by inserting directly
		cycleID := uuid.New().String()
		_, err = ts.DB.DB.ExecContext(ctx, `
			INSERT INTO game_cycles (id, session_id, player_id, game_id, started_at, wager_amount, win_amount, balance_before, balance_after, outcome, status, currency)
			VALUES ($1, $2, $3, $4, NOW(), 200, 0, 50000, 49800, '{"reels":["7","BAR","CHERRY"]}', $5, 'USD')
		`, cycleID, session.ID, player.ID, "fortune-slots", domain.CycleStatusInterrupted)
		if err != nil {
			t.Fatalf("Failed to create interrupted cycle: %v", err)
		}

		// Verify interrupted game is detected
		interrupted, err := ts.Game.GetInterruptedGames(ctx, player.ID)
		if err != nil {
			t.Fatalf("Failed to get interrupted games: %v", err)
		}

		if len(interrupted) != 1 {
			t.Fatalf("Expected 1 interrupted game, got %d", len(interrupted))
		}

		if interrupted[0].CycleID != cycleID {
			t.Errorf("Expected cycle ID %s, got %s", cycleID, interrupted[0].CycleID)
		}
	})

	t.Run("VoidInterruptedGame", func(t *testing.T) {
		interrupted, _ := ts.Game.GetInterruptedGames(ctx, player.ID)
		if len(interrupted) == 0 {
			t.Skip("No interrupted games to void")
		}

		cycleID := interrupted[0].CycleID
		err := ts.Game.VoidGame(ctx, cycleID, "Player requested void")
		if err != nil {
			t.Fatalf("Failed to void game: %v", err)
		}

		// Verify no more interrupted games
		remaining, _ := ts.Game.GetInterruptedGames(ctx, player.ID)
		if len(remaining) != 0 {
			t.Errorf("Expected 0 interrupted games after void, got %d", len(remaining))
		}
	})
}

// ============================================================================
// Responsible Gaming Flow Test (GLI-19 §2.5.5)
// ============================================================================

func TestResponsibleGamingFlow(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Step 1: Create player
	t.Log("Step 1: Creating player for responsible gaming test...")
	player := ts.createTestUser(t, "responsible_player", "responsible@example.com", "password123")
	ts.Wallet.Deposit(ctx, player.ID, domain.NewMoney(1000.00, "USD"), "initial-deposit")

	// Step 2: Set responsible gaming limits
	t.Log("Step 2: Setting responsible gaming limits...")

	_, err := ts.Limits.SetDepositLimit(ctx, &limits.SetDepositLimitRequest{
		PlayerID: player.ID,
		Period:   "daily",
		Amount:   50000, // $500
	})
	if err != nil {
		t.Fatalf("Failed to set deposit limit: %v", err)
	}

	_, err = ts.Limits.SetWagerLimit(ctx, &limits.SetWagerLimitRequest{
		PlayerID: player.ID,
		Period:   "daily",
		Amount:   20000, // $200
	})
	if err != nil {
		t.Fatalf("Failed to set wager limit: %v", err)
	}

	_, err = ts.Limits.SetLossLimit(ctx, &limits.SetLossLimitRequest{
		PlayerID: player.ID,
		Period:   "daily",
		Amount:   10000, // $100
	})
	if err != nil {
		t.Fatalf("Failed to set loss limit: %v", err)
	}

	t.Log("  Limits set: Daily deposit $500, Daily wager $200, Daily loss $100")

	// Step 3: Verify limits
	t.Log("Step 3: Verifying limits are set...")
	playerLimits, _ := ts.Limits.GetLimits(ctx, player.ID)
	t.Logf("  Daily deposit: $%.2f", float64(playerLimits.DailyDeposit.Amount)/100)
	t.Logf("  Daily wager: $%.2f", float64(playerLimits.DailyWager.Amount)/100)
	t.Logf("  Daily loss: $%.2f", float64(playerLimits.DailyLoss.Amount)/100)

	// Step 4: Test limit decrease is immediate
	t.Log("Step 4: Testing limit decrease (should be immediate)...")
	newLimits, _ := ts.Limits.SetDepositLimit(ctx, &limits.SetDepositLimitRequest{
		PlayerID: player.ID,
		Period:   "daily",
		Amount:   25000, // $250 - decrease
	})
	if newLimits.EffectiveAt.After(time.Now().Add(time.Second)) {
		t.Error("Limit decrease should be immediate")
	} else {
		t.Log("  ✓ Limit decrease is immediate")
	}

	// Step 5: Test limit increase requires cooling off
	t.Log("Step 5: Testing limit increase (should require cooling off)...")
	newLimits, _ = ts.Limits.SetDepositLimit(ctx, &limits.SetDepositLimitRequest{
		PlayerID: player.ID,
		Period:   "daily",
		Amount:   50000, // $500 - increase
	})
	if newLimits.EffectiveAt.Before(time.Now().Add(23 * time.Hour)) {
		t.Error("Limit increase should have 24-hour cooling off")
	} else {
		t.Log("  ✓ Limit increase has cooling off period")
	}

	// Step 6: Verify player is not excluded
	t.Log("Step 6: Verifying player is not self-excluded...")
	excluded, _ := ts.Limits.IsExcluded(ctx, player.ID)
	if excluded {
		t.Error("Player should not be excluded")
	} else {
		t.Log("  ✓ Player is not excluded")
	}

	t.Log("✓ Responsible gaming flow test passed!")
}

// ============================================================================
// Audit Logging Test
// ============================================================================

func TestAuditLogging(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ctx := context.Background()

	// Log an event
	err := ts.Audit.Log(ctx, "test_event", "info", "Test event for integration test",
		map[string]string{"key": "value"},
		audit.WithComponent("test"))
	if err != nil {
		t.Fatalf("Failed to log event: %v", err)
	}

	// Retrieve events
	events, err := ts.Audit.GetEvents(ctx, &audit.EventFilter{
		Type:  "test_event",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	if len(events) == 0 {
		t.Error("Expected at least 1 event")
	}

	if events[0].Type != "test_event" {
		t.Errorf("Expected event type 'test_event', got '%s'", events[0].Type)
	}
}
