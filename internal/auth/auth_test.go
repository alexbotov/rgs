package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/config"
	"github.com/alexbotov/rgs/internal/database"
)

func setupTestAuth(t *testing.T) (*Service, func()) {
	t.Helper()

	// Create PostgreSQL connection
	db, err := database.New("postgres", "host=localhost dbname=rgs sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Ensure schema exists (idempotent)
	if err := db.Migrate(); err != nil {
		// Ignore if tables already exist
		t.Logf("Migration note: %v", err)
	}

	// Clean data for fresh test state
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

	svc := New(db.DB, cfg, auditSvc)

	return svc, func() {
		db.CleanData()
		db.Close()
	}
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
	svc, cleanup := setupTestAuth(t)
	defer cleanup()

	ctx := context.Background()

	// Register a user first
	svc.Register(ctx, &RegisterRequest{
		Username: "loginuser",
		Email:    "login@example.com",
		Password: "password123",
		AcceptTC: true,
	}, "127.0.0.1")

	t.Run("SuccessfulLogin", func(t *testing.T) {
		result, err := svc.Login(ctx, &LoginRequest{
			Username: "loginuser",
			Password: "password123",
		}, "127.0.0.1", "TestAgent")

		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		if result.Token == "" {
			t.Error("Expected token")
		}
		if result.Player.Username != "loginuser" {
			t.Errorf("Expected username 'loginuser', got '%s'", result.Player.Username)
		}
	})

	t.Run("InvalidPassword", func(t *testing.T) {
		_, err := svc.Login(ctx, &LoginRequest{
			Username: "loginuser",
			Password: "wrongpassword",
		}, "127.0.0.1", "TestAgent")

		if err == nil {
			t.Error("Expected error for invalid password")
		}
	})

	t.Run("NonexistentUser", func(t *testing.T) {
		_, err := svc.Login(ctx, &LoginRequest{
			Username: "nonexistent",
			Password: "password123",
		}, "127.0.0.1", "TestAgent")

		if err == nil {
			t.Error("Expected error for nonexistent user")
		}
	})
}

func TestValidateToken(t *testing.T) {
	svc, cleanup := setupTestAuth(t)
	defer cleanup()

	ctx := context.Background()

	// Register and login
	svc.Register(ctx, &RegisterRequest{
		Username: "tokenuser",
		Email:    "token@example.com",
		Password: "password123",
		AcceptTC: true,
	}, "127.0.0.1")

	loginResult, _ := svc.Login(ctx, &LoginRequest{
		Username: "tokenuser",
		Password: "password123",
	}, "127.0.0.1", "TestAgent")

	t.Run("ValidToken", func(t *testing.T) {
		session, player, err := svc.ValidateToken(ctx, loginResult.Token)
		if err != nil {
			t.Fatalf("Token validation failed: %v", err)
		}

		if session.PlayerID == "" {
			t.Error("Expected player ID in session")
		}
		if player.Username != "tokenuser" {
			t.Errorf("Expected username 'tokenuser', got '%s'", player.Username)
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
	svc, cleanup := setupTestAuth(t)
	defer cleanup()

	ctx := context.Background()

	// Register and login
	svc.Register(ctx, &RegisterRequest{
		Username: "logoutuser",
		Email:    "logout@example.com",
		Password: "password123",
		AcceptTC: true,
	}, "127.0.0.1")

	loginResult, _ := svc.Login(ctx, &LoginRequest{
		Username: "logoutuser",
		Password: "password123",
	}, "127.0.0.1", "TestAgent")

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
	svc, cleanup := setupTestAuth(t)
	defer cleanup()

	ctx := context.Background()

	// Register a user - check for errors
	_, err := svc.Register(ctx, &RegisterRequest{
		Username: "lockuser",
		Email:    "lock@example.com",
		Password: "password123",
		AcceptTC: true,
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("Failed to register lockuser: %v", err)
	}

	t.Run("FailedLoginRecorded", func(t *testing.T) {
		// Attempt login with wrong password - should fail with invalid credentials
		_, err := svc.Login(ctx, &LoginRequest{
			Username: "lockuser",
			Password: "wrongpassword",
		}, "127.0.0.1", "TestAgent")

		if err != ErrInvalidCredentials {
			t.Errorf("Expected ErrInvalidCredentials, got: %v", err)
		}
	})

	t.Run("CorrectPasswordAfterFailure", func(t *testing.T) {
		// Correct password should still work (only 1 failed attempt)
		result, err := svc.Login(ctx, &LoginRequest{
			Username: "lockuser",
			Password: "password123",
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
