package control

import (
	"context"
	"testing"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/database"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/google/uuid"
)

func setupTestControl(t *testing.T) (*Service, string, func()) {
	t.Helper()

	// Create PostgreSQL connection
	db, err := database.New("postgres", "host=localhost dbname=rgs sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Ensure schema exists (idempotent)
	if err := db.Migrate(); err != nil {
		t.Logf("Migration note: %v", err)
	}

	// Clean data for fresh test state
	if err := db.CleanData(); err != nil {
		t.Fatalf("Failed to clean data: %v", err)
	}

	auditSvc := audit.New(db.DB)
	svc := New(db.DB, auditSvc)

	// Create a test player
	playerID := uuid.New().String()
	_, err = db.DB.Exec(`
		INSERT INTO players (id, username, email, password_hash, status, registration_date, tc_accepted_at, created_at, updated_at)
		VALUES ($1, 'controluser', 'control@example.com', 'hash', 'active', NOW(), NOW(), NOW(), NOW())
	`, playerID)
	if err != nil {
		t.Fatalf("Failed to create test player: %v", err)
	}

	// Create balance record
	_, err = db.DB.Exec(`
		INSERT INTO balances (player_id, real_money_amount, real_money_currency, bonus_amount, bonus_currency, updated_at)
		VALUES ($1, 100000, 'USD', 0, 'USD', NOW())
	`, playerID)
	if err != nil {
		t.Fatalf("Failed to create balance: %v", err)
	}

	return svc, playerID, func() {
		db.CleanData()
		db.Close()
	}
}

func TestGamingEnabled(t *testing.T) {
	svc, _, cleanup := setupTestControl(t)
	defer cleanup()

	t.Run("InitiallyEnabled", func(t *testing.T) {
		if !svc.IsGamingEnabled() {
			t.Error("Gaming should be enabled by default")
		}
	})
}

func TestDisableAllGaming(t *testing.T) {
	svc, _, cleanup := setupTestControl(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("DisableGaming", func(t *testing.T) {
		err := svc.DisableAllGaming(ctx, "Maintenance", "admin@example.com")
		if err != nil {
			t.Fatalf("Failed to disable gaming: %v", err)
		}

		if svc.IsGamingEnabled() {
			t.Error("Gaming should be disabled")
		}
	})

	t.Run("EnableGaming", func(t *testing.T) {
		err := svc.EnableAllGaming(ctx, "admin@example.com")
		if err != nil {
			t.Fatalf("Failed to enable gaming: %v", err)
		}

		if !svc.IsGamingEnabled() {
			t.Error("Gaming should be enabled")
		}
	})
}

func TestDisableGame(t *testing.T) {
	svc, _, cleanup := setupTestControl(t)
	defer cleanup()

	ctx := context.Background()
	gameID := "fortune-slots"

	t.Run("InitiallyEnabled", func(t *testing.T) {
		if !svc.IsGameEnabled(gameID) {
			t.Error("Game should be enabled by default")
		}
	})

	t.Run("DisableGame", func(t *testing.T) {
		err := svc.DisableGame(ctx, gameID, "Game maintenance", "admin@example.com")
		if err != nil {
			t.Fatalf("Failed to disable game: %v", err)
		}

		if svc.IsGameEnabled(gameID) {
			t.Error("Game should be disabled")
		}
	})

	t.Run("OtherGamesStillEnabled", func(t *testing.T) {
		otherGameID := "lucky-sevens"
		if !svc.IsGameEnabled(otherGameID) {
			t.Error("Other games should still be enabled")
		}
	})

	t.Run("EnableGame", func(t *testing.T) {
		err := svc.EnableGame(ctx, gameID, "admin@example.com")
		if err != nil {
			t.Fatalf("Failed to enable game: %v", err)
		}

		if !svc.IsGameEnabled(gameID) {
			t.Error("Game should be enabled")
		}
	})
}

func TestDisablePlayer(t *testing.T) {
	svc, playerID, cleanup := setupTestControl(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("DisablePlayer", func(t *testing.T) {
		err := svc.DisablePlayer(ctx, playerID, "Suspicious activity", "admin@example.com")
		if err != nil {
			t.Fatalf("Failed to disable player: %v", err)
		}

		// Verify player status is suspended
		err = svc.CheckAccess(ctx, playerID, "fortune-slots")
		if err == nil {
			t.Error("Expected error for disabled player")
		}
		if err != ErrPlayerDisabled {
			t.Errorf("Expected ErrPlayerDisabled, got: %v", err)
		}
	})

	t.Run("EnablePlayer", func(t *testing.T) {
		err := svc.EnablePlayer(ctx, playerID, "admin@example.com")
		if err != nil {
			t.Fatalf("Failed to enable player: %v", err)
		}

		// Verify player can access again
		err = svc.CheckAccess(ctx, playerID, "fortune-slots")
		if err != nil {
			t.Errorf("Expected no error for enabled player, got: %v", err)
		}
	})
}

func TestCheckAccess(t *testing.T) {
	svc, playerID, cleanup := setupTestControl(t)
	defer cleanup()

	ctx := context.Background()
	gameID := "fortune-slots"

	t.Run("AllEnabled", func(t *testing.T) {
		err := svc.CheckAccess(ctx, playerID, gameID)
		if err != nil {
			t.Errorf("Expected no error when all enabled: %v", err)
		}
	})

	t.Run("GamingDisabled", func(t *testing.T) {
		svc.DisableAllGaming(ctx, "Test", "admin")

		err := svc.CheckAccess(ctx, playerID, gameID)
		if err == nil {
			t.Error("Expected error when gaming disabled")
		}
		if err != ErrGamingDisabled {
			t.Errorf("Expected ErrGamingDisabled, got: %v", err)
		}

		svc.EnableAllGaming(ctx, "admin")
	})

	t.Run("GameDisabled", func(t *testing.T) {
		svc.DisableGame(ctx, gameID, "Test", "admin")

		err := svc.CheckAccess(ctx, playerID, gameID)
		if err == nil {
			t.Error("Expected error when game disabled")
		}
		if err != ErrGameDisabled {
			t.Errorf("Expected ErrGameDisabled, got: %v", err)
		}

		svc.EnableGame(ctx, gameID, "admin")
	})

	t.Run("PlayerDisabled", func(t *testing.T) {
		svc.DisablePlayer(ctx, playerID, "Test", "admin")

		err := svc.CheckAccess(ctx, playerID, gameID)
		if err == nil {
			t.Error("Expected error when player disabled")
		}
		if err != ErrPlayerDisabled {
			t.Errorf("Expected ErrPlayerDisabled, got: %v", err)
		}

		svc.EnablePlayer(ctx, playerID, "admin")
	})
}

func TestGetSystemStatus(t *testing.T) {
	svc, _, cleanup := setupTestControl(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("GetStatus", func(t *testing.T) {
		status, err := svc.GetSystemStatus(ctx)
		if err != nil {
			t.Fatalf("Failed to get system status: %v", err)
		}

		if !status.GamingEnabled {
			t.Error("Expected gaming to be enabled")
		}

		if status.ActiveSessions < 0 {
			t.Error("Active sessions should be non-negative")
		}
	})

	t.Run("StatusAfterDisable", func(t *testing.T) {
		svc.DisableAllGaming(ctx, "Test reason", "admin")

		status, err := svc.GetSystemStatus(ctx)
		if err != nil {
			t.Fatalf("Failed to get system status: %v", err)
		}

		if status.GamingEnabled {
			t.Error("Expected gaming to be disabled")
		}

		if status.DisabledReason != "Test reason" {
			t.Errorf("Expected reason 'Test reason', got '%s'", status.DisabledReason)
		}

		if status.DisabledBy != "admin" {
			t.Errorf("Expected disabled by 'admin', got '%s'", status.DisabledBy)
		}
	})
}

func TestCannotEnableExcludedPlayer(t *testing.T) {
	svc, playerID, cleanup := setupTestControl(t)
	defer cleanup()

	ctx := context.Background()

	// Create a self-exclusion record for the player
	_, err := svc.db.ExecContext(ctx, `
		INSERT INTO self_exclusions (id, player_id, reason, started_at, is_active, created_at)
		VALUES ($1, $2, 'Self excluded', NOW(), true, NOW())
	`, uuid.New().String(), playerID)
	if err != nil {
		t.Fatalf("Failed to create self-exclusion: %v", err)
	}

	// Update player status to excluded
	_, err = svc.db.ExecContext(ctx, `
		UPDATE players SET status = $1 WHERE id = $2
	`, domain.PlayerStatusExcluded, playerID)
	if err != nil {
		t.Fatalf("Failed to update player status: %v", err)
	}

	t.Run("CannotEnableExcludedPlayer", func(t *testing.T) {
		err := svc.EnablePlayer(ctx, playerID, "admin")
		if err == nil {
			t.Error("Expected error when trying to enable excluded player")
		}
	})
}

func TestLoadState(t *testing.T) {
	svc, _, cleanup := setupTestControl(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("LoadState", func(t *testing.T) {
		// Disable gaming
		svc.DisableAllGaming(ctx, "Test", "admin")

		// Create a new service instance (simulating restart)
		svc2 := New(svc.db, svc.audit)

		// Load state
		err := svc2.LoadState(ctx)
		if err != nil {
			t.Fatalf("Failed to load state: %v", err)
		}

		// Check that gaming is still disabled
		if svc2.IsGamingEnabled() {
			t.Error("Gaming should still be disabled after loading state")
		}
	})
}

func TestMultipleGamesDisabled(t *testing.T) {
	svc, _, cleanup := setupTestControl(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("DisableMultipleGames", func(t *testing.T) {
		err := svc.DisableGame(ctx, "fortune-slots", "Maintenance", "admin")
		if err != nil {
			t.Fatalf("Failed to disable fortune-slots: %v", err)
		}

		err = svc.DisableGame(ctx, "lucky-sevens", "Bug fix", "admin")
		if err != nil {
			t.Fatalf("Failed to disable lucky-sevens: %v", err)
		}

		if svc.IsGameEnabled("fortune-slots") {
			t.Error("fortune-slots should be disabled")
		}
		if svc.IsGameEnabled("lucky-sevens") {
			t.Error("lucky-sevens should be disabled")
		}
	})

	t.Run("EnableOneGame", func(t *testing.T) {
		err := svc.EnableGame(ctx, "fortune-slots", "admin")
		if err != nil {
			t.Fatalf("Failed to enable fortune-slots: %v", err)
		}

		if !svc.IsGameEnabled("fortune-slots") {
			t.Error("fortune-slots should be enabled")
		}
		if svc.IsGameEnabled("lucky-sevens") {
			t.Error("lucky-sevens should still be disabled")
		}
	})
}

