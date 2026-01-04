package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/config"
	"github.com/alexbotov/rgs/internal/database"
	"github.com/alexbotov/rgs/pkg/pateplay"
)

const (
	testAPIKey    = "test-api-key"
	testAPISecret = "test-api-secret"
	testSiteCode  = "testsite"
)

// mockPateplayServer creates a test server that simulates Pateplay API responses
func mockPateplayServer(t *testing.T, authToken string, result *pateplay.AuthenticateResult, apiErr *pateplay.APIError) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse request to check auth token
		var req pateplay.AuthenticateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// Check if the auth token matches what we expect for success
		if req.AuthToken == authToken && result != nil {
			json.NewEncoder(w).Encode(pateplay.Response[pateplay.AuthenticateResult]{
				Result: result,
			})
			return
		}

		// Return error for invalid tokens
		errResp := apiErr
		if errResp == nil {
			errResp = &pateplay.APIError{
				Code:    pateplay.ErrInvalidAuthToken,
				Message: "Invalid auth token",
			}
		}
		json.NewEncoder(w).Encode(pateplay.Response[pateplay.AuthenticateResult]{
			Error: errResp,
		})
	}))
}

// setupTestAuthWithMock creates auth service with a mocked Pateplay server
func setupTestAuthWithMock(t *testing.T, validAuthToken string, authResult *pateplay.AuthenticateResult) (*Service, func()) {
	t.Helper()

	// Create mock Pateplay server
	mockServer := mockPateplayServer(t, validAuthToken, authResult, nil)

	// Create PostgreSQL connection
	db, err := database.New("postgres", "host=localhost dbname=rgs sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	if err := db.Migrate(); err != nil {
		t.Logf("Migration note: %v", err)
	}

	if err := db.CleanData(); err != nil {
		t.Fatalf("Failed to clean data: %v", err)
	}

	auditSvc := audit.New(db.DB)
	cfg := &config.AuthConfig{
		JWTSecret:         "test-secret-key-12345",
		TokenExpiry:       1 * time.Hour,
		SessionTimeout:    30 * time.Minute,
		MaxFailedAttempts: 3,
		LockoutDuration:   15 * time.Minute,
	}

	// Create pateplay client pointing to mock server
	pateplayClient := pateplay.NewClient(&pateplay.ClientConfig{
		BaseURL:   mockServer.URL,
		APIKey:    testAPIKey,
		APISecret: testAPISecret,
		SiteCode:  testSiteCode,
	})

	svc := New(db.DB, cfg, auditSvc, pateplayClient)

	return svc, func() {
		mockServer.Close()
		db.CleanData()
		db.Close()
	}
}

func setupTestAuth(t *testing.T) (*Service, func()) {
	t.Helper()

	// Default mock response for "valid-auth-token" (must be valid UUID)
	authResult := &pateplay.AuthenticateResult{
		SessionToken: "mock-session-token",
		PlayerID:     "00000000-0000-0000-0000-000000000000",
		PlayerName:   "MockPlayer",
		Currency:     "USD",
		Country:      "US",
		Balance:      "1000.00",
	}

	return setupTestAuthWithMock(t, "valid-auth-token", authResult)
}

func TestRegister(t *testing.T) {
	svc, cleanup := setupTestAuth(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("SuccessfulRegistration", func(t *testing.T) {
		player, err := svc.Register(ctx, &RegisterRequest{
			Username: "testuser",
			Email:    "test@example.com",
			Password: "password123",
			AcceptTC: true,
		}, "127.0.0.1")

		if err != nil {
			t.Fatalf("Registration failed: %v", err)
		}

		if player.ID == "" {
			t.Error("Expected player ID")
		}
		if player.Username != "testuser" {
			t.Errorf("Expected username 'testuser', got '%s'", player.Username)
		}
		if player.Email != "test@example.com" {
			t.Errorf("Expected email 'test@example.com', got '%s'", player.Email)
		}
	})

	t.Run("DuplicateUsername", func(t *testing.T) {
		_, err := svc.Register(ctx, &RegisterRequest{
			Username: "testuser",
			Email:    "other@example.com",
			Password: "password123",
			AcceptTC: true,
		}, "127.0.0.1")

		if err == nil {
			t.Error("Expected error for duplicate username")
		}
	})

	t.Run("DuplicateEmail", func(t *testing.T) {
		_, err := svc.Register(ctx, &RegisterRequest{
			Username: "otheruser",
			Email:    "test@example.com",
			Password: "password123",
			AcceptTC: true,
		}, "127.0.0.1")

		if err == nil {
			t.Error("Expected error for duplicate email")
		}
	})

	t.Run("ShortPassword", func(t *testing.T) {
		_, err := svc.Register(ctx, &RegisterRequest{
			Username: "validuser",
			Email:    "valid@example.com",
			Password: "short",
			AcceptTC: true,
		}, "127.0.0.1")

		if err == nil {
			t.Error("Expected error for short password")
		}
	})

	t.Run("TCNotAccepted", func(t *testing.T) {
		_, err := svc.Register(ctx, &RegisterRequest{
			Username: "tcuser",
			Email:    "tc@example.com",
			Password: "password123",
			AcceptTC: false,
		}, "127.0.0.1")

		if err == nil {
			t.Error("Expected error when T&C not accepted")
		}
	})
}

func TestLogin(t *testing.T) {
	// Create mock with specific player ID (must be valid UUID)
	authResult := &pateplay.AuthenticateResult{
		SessionToken: "mock-session-token",
		PlayerID:     "11111111-1111-1111-1111-111111111111",
		PlayerName:   "LoginPlayer",
		Currency:     "USD",
		Country:      "US",
		Balance:      "1000.00",
	}

	svc, cleanup := setupTestAuthWithMock(t, "valid-auth-token", authResult)
	defer cleanup()

	ctx := context.Background()

	// Insert player with the ID that Pateplay will return
	_, err := svc.db.ExecContext(ctx, `
		INSERT INTO players (id, username, email, password_hash, status, registration_date, tc_accepted_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, authResult.PlayerID, authResult.PlayerName, "login@example.com", "", "active",
		time.Now().UTC(), time.Now().UTC(), time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert test player: %v", err)
	}

	t.Run("SuccessfulLogin", func(t *testing.T) {
		result, err := svc.Login(ctx, &LoginRequest{
			AuthToken:  "valid-auth-token",
			DeviceType: "desktop",
		}, "127.0.0.1", "TestAgent")

		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		if result.Token == "" {
			t.Error("Expected token")
		}
		if result.Player.Username != "LoginPlayer" {
			t.Errorf("Expected username 'LoginPlayer', got '%s'", result.Player.Username)
		}
		if result.Player.ID != "11111111-1111-1111-1111-111111111111" {
			t.Errorf("Expected player ID '11111111-1111-1111-1111-111111111111', got '%s'", result.Player.ID)
		}
	})

	t.Run("InvalidAuthToken", func(t *testing.T) {
		_, err := svc.Login(ctx, &LoginRequest{
			AuthToken:  "invalid-auth-token",
			DeviceType: "desktop",
		}, "127.0.0.1", "TestAgent")

		if err == nil {
			t.Error("Expected error for invalid auth token")
		}
	})

	t.Run("EmptyAuthToken", func(t *testing.T) {
		_, err := svc.Login(ctx, &LoginRequest{
			AuthToken:  "",
			DeviceType: "desktop",
		}, "127.0.0.1", "TestAgent")

		if err == nil {
			t.Error("Expected error for empty auth token")
		}
	})
}

func TestValidateToken(t *testing.T) {
	// Create mock with specific player ID (must be valid UUID)
	authResult := &pateplay.AuthenticateResult{
		SessionToken: "mock-session-token",
		PlayerID:     "22222222-2222-2222-2222-222222222222",
		PlayerName:   "TokenUser",
		Currency:     "USD",
		Country:      "US",
		Balance:      "1000.00",
	}

	svc, cleanup := setupTestAuthWithMock(t, "valid-auth-token", authResult)
	defer cleanup()

	ctx := context.Background()

	// Insert player with the ID that Pateplay will return
	_, err := svc.db.ExecContext(ctx, `
		INSERT INTO players (id, username, email, password_hash, status, registration_date, tc_accepted_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, authResult.PlayerID, authResult.PlayerName, "token@example.com", "", "active",
		time.Now().UTC(), time.Now().UTC(), time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert test player: %v", err)
	}

	loginResult, loginErr := svc.Login(ctx, &LoginRequest{
		AuthToken:  "valid-auth-token",
		DeviceType: "desktop",
	}, "127.0.0.1", "TestAgent")
	if loginErr != nil {
		t.Fatalf("Login failed: %v", loginErr)
	}

	t.Run("ValidToken", func(t *testing.T) {
		session, player, err := svc.ValidateToken(ctx, loginResult.Token)
		if err != nil {
			t.Fatalf("Token validation failed: %v", err)
		}

		if session.PlayerID == "" {
			t.Error("Expected player ID in session")
		}
		if player.Username != "TokenUser" {
			t.Errorf("Expected username 'TokenUser', got '%s'", player.Username)
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		_, _, err := svc.ValidateToken(ctx, "invalid-token")
		if err == nil {
			t.Error("Expected error for invalid token")
		}
	})

	t.Run("TamperedToken", func(t *testing.T) {
		// Tamper with the token
		tampered := loginResult.Token + "tampered"
		_, _, err := svc.ValidateToken(ctx, tampered)
		if err == nil {
			t.Error("Expected error for tampered token")
		}
	})
}

func TestLogout(t *testing.T) {
	// Create mock with specific player ID (must be valid UUID)
	authResult := &pateplay.AuthenticateResult{
		SessionToken: "mock-session-token",
		PlayerID:     "33333333-3333-3333-3333-333333333333",
		PlayerName:   "LogoutUser",
		Currency:     "USD",
		Country:      "US",
		Balance:      "1000.00",
	}

	svc, cleanup := setupTestAuthWithMock(t, "valid-auth-token", authResult)
	defer cleanup()

	ctx := context.Background()

	// Insert player with the ID that Pateplay will return
	_, err := svc.db.ExecContext(ctx, `
		INSERT INTO players (id, username, email, password_hash, status, registration_date, tc_accepted_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, authResult.PlayerID, authResult.PlayerName, "logout@example.com", "", "active",
		time.Now().UTC(), time.Now().UTC(), time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert test player: %v", err)
	}

	loginResult, loginErr := svc.Login(ctx, &LoginRequest{
		AuthToken:  "valid-auth-token",
		DeviceType: "desktop",
	}, "127.0.0.1", "TestAgent")
	if loginErr != nil {
		t.Fatalf("Login failed: %v", loginErr)
	}

	t.Run("SuccessfulLogout", func(t *testing.T) {
		// Validate token first
		session, _, err := svc.ValidateToken(ctx, loginResult.Token)
		if err != nil {
			t.Fatalf("Token should be valid before logout: %v", err)
		}

		// Logout using session ID
		err = svc.Logout(ctx, session.ID)
		if err != nil {
			t.Fatalf("Logout failed: %v", err)
		}

		// Token should be invalid after logout
		_, _, err = svc.ValidateToken(ctx, loginResult.Token)
		if err == nil {
			t.Error("Token should be invalid after logout")
		}
	})
}

func TestAccountLockout(t *testing.T) {
	// Create mock with specific player ID (must be valid UUID)
	authResult := &pateplay.AuthenticateResult{
		SessionToken: "mock-session-token",
		PlayerID:     "44444444-4444-4444-4444-444444444444",
		PlayerName:   "LockoutUser",
		Currency:     "USD",
		Country:      "US",
		Balance:      "1000.00",
	}

	svc, cleanup := setupTestAuthWithMock(t, "valid-auth-token", authResult)
	defer cleanup()

	ctx := context.Background()

	// Insert player with the ID that Pateplay will return
	_, err := svc.db.ExecContext(ctx, `
		INSERT INTO players (id, username, email, password_hash, status, registration_date, tc_accepted_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, authResult.PlayerID, authResult.PlayerName, "lockout@example.com", "", "active",
		time.Now().UTC(), time.Now().UTC(), time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert test player: %v", err)
	}

	t.Run("FailedLoginRecorded", func(t *testing.T) {
		// Attempt login with invalid auth token - should fail
		_, err := svc.Login(ctx, &LoginRequest{
			AuthToken:  "invalid-auth-token",
			DeviceType: "desktop",
		}, "127.0.0.1", "TestAgent")

		if err == nil {
			t.Error("Expected error for invalid auth token")
		}
	})

	t.Run("ValidAuthTokenAfterFailure", func(t *testing.T) {
		// Valid auth token should still work after failed attempt
		result, err := svc.Login(ctx, &LoginRequest{
			AuthToken:  "valid-auth-token",
			DeviceType: "desktop",
		}, "127.0.0.1", "TestAgent")

		if err != nil {
			t.Fatalf("Expected successful login, got: %v", err)
		}
		if result == nil {
			t.Fatal("Expected non-nil result")
		}
		if result.Token == "" {
			t.Error("Expected token")
		}
	})
}
