package limits

import (
	"context"
	"testing"
	"time"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/database"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/google/uuid"
)

func setupTestLimits(t *testing.T) (*Service, string, func()) {
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
	svc := New(db.DB, auditSvc, "USD")

	// Create a test player
	playerID := uuid.New().String()
	_, err = db.DB.Exec(`
		INSERT INTO players (id, username, email, password_hash, status, registration_date, tc_accepted_at, created_at, updated_at)
		VALUES ($1, 'limitsuser', 'limits@example.com', 'hash', 'active', NOW(), NOW(), NOW(), NOW())
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

func TestGetLimits(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("NoLimitsSet", func(t *testing.T) {
		limits, err := svc.GetLimits(ctx, playerID)
		if err != nil {
			t.Fatalf("Failed to get limits: %v", err)
		}

		if limits.PlayerID != playerID {
			t.Errorf("Expected player ID %s, got %s", playerID, limits.PlayerID)
		}

		// All limits should be nil when not set
		if limits.DailyDeposit != nil {
			t.Error("Expected nil daily deposit limit")
		}
		if limits.WeeklyDeposit != nil {
			t.Error("Expected nil weekly deposit limit")
		}
	})
}

func TestSetDepositLimit(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("SetDailyDepositLimit", func(t *testing.T) {
		limits, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
			PlayerID: playerID,
			Period:   "daily",
			Amount:   10000, // $100
		})
		if err != nil {
			t.Fatalf("Failed to set deposit limit: %v", err)
		}

		if limits.DailyDeposit == nil {
			t.Fatal("Expected daily deposit limit to be set")
		}

		if limits.DailyDeposit.Amount != 10000 {
			t.Errorf("Expected limit 10000, got %d", limits.DailyDeposit.Amount)
		}
	})

	t.Run("SetWeeklyDepositLimit", func(t *testing.T) {
		limits, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
			PlayerID: playerID,
			Period:   "weekly",
			Amount:   50000, // $500
		})
		if err != nil {
			t.Fatalf("Failed to set weekly deposit limit: %v", err)
		}

		if limits.WeeklyDeposit == nil {
			t.Fatal("Expected weekly deposit limit to be set")
		}

		if limits.WeeklyDeposit.Amount != 50000 {
			t.Errorf("Expected limit 50000, got %d", limits.WeeklyDeposit.Amount)
		}
	})

	t.Run("SetMonthlyDepositLimit", func(t *testing.T) {
		limits, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
			PlayerID: playerID,
			Period:   "monthly",
			Amount:   100000, // $1000
		})
		if err != nil {
			t.Fatalf("Failed to set monthly deposit limit: %v", err)
		}

		if limits.MonthlyDeposit == nil {
			t.Fatal("Expected monthly deposit limit to be set")
		}

		if limits.MonthlyDeposit.Amount != 100000 {
			t.Errorf("Expected limit 100000, got %d", limits.MonthlyDeposit.Amount)
		}
	})

	t.Run("InvalidPeriod", func(t *testing.T) {
		_, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
			PlayerID: playerID,
			Period:   "invalid",
			Amount:   10000,
		})
		if err == nil {
			t.Error("Expected error for invalid period")
		}
	})

	t.Run("NegativeAmount", func(t *testing.T) {
		_, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
			PlayerID: playerID,
			Period:   "daily",
			Amount:   -1000,
		})
		if err == nil {
			t.Error("Expected error for negative amount")
		}
	})
}

func TestSetWagerLimit(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("SetDailyWagerLimit", func(t *testing.T) {
		limits, err := svc.SetWagerLimit(ctx, &SetWagerLimitRequest{
			PlayerID: playerID,
			Period:   "daily",
			Amount:   5000, // $50
		})
		if err != nil {
			t.Fatalf("Failed to set wager limit: %v", err)
		}

		if limits.DailyWager == nil {
			t.Fatal("Expected daily wager limit to be set")
		}

		if limits.DailyWager.Amount != 5000 {
			t.Errorf("Expected limit 5000, got %d", limits.DailyWager.Amount)
		}
	})

	t.Run("SetWeeklyWagerLimit", func(t *testing.T) {
		limits, err := svc.SetWagerLimit(ctx, &SetWagerLimitRequest{
			PlayerID: playerID,
			Period:   "weekly",
			Amount:   20000, // $200
		})
		if err != nil {
			t.Fatalf("Failed to set weekly wager limit: %v", err)
		}

		if limits.WeeklyWager == nil {
			t.Fatal("Expected weekly wager limit to be set")
		}

		if limits.WeeklyWager.Amount != 20000 {
			t.Errorf("Expected limit 20000, got %d", limits.WeeklyWager.Amount)
		}
	})
}

func TestSetLossLimit(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("SetDailyLossLimit", func(t *testing.T) {
		limits, err := svc.SetLossLimit(ctx, &SetLossLimitRequest{
			PlayerID: playerID,
			Period:   "daily",
			Amount:   2500, // $25
		})
		if err != nil {
			t.Fatalf("Failed to set loss limit: %v", err)
		}

		if limits.DailyLoss == nil {
			t.Fatal("Expected daily loss limit to be set")
		}

		if limits.DailyLoss.Amount != 2500 {
			t.Errorf("Expected limit 2500, got %d", limits.DailyLoss.Amount)
		}
	})

	t.Run("SetWeeklyLossLimit", func(t *testing.T) {
		limits, err := svc.SetLossLimit(ctx, &SetLossLimitRequest{
			PlayerID: playerID,
			Period:   "weekly",
			Amount:   10000, // $100
		})
		if err != nil {
			t.Fatalf("Failed to set weekly loss limit: %v", err)
		}

		if limits.WeeklyLoss == nil {
			t.Fatal("Expected weekly loss limit to be set")
		}

		if limits.WeeklyLoss.Amount != 10000 {
			t.Errorf("Expected limit 10000, got %d", limits.WeeklyLoss.Amount)
		}
	})
}

func TestSelfExclusion(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("SelfExcludeWithDuration", func(t *testing.T) {
		duration := 7 * 24 * time.Hour // 7 days
		exclusion, err := svc.SelfExclude(ctx, playerID, "Taking a break", &duration)
		if err != nil {
			t.Fatalf("Failed to self-exclude: %v", err)
		}

		if exclusion.PlayerID != playerID {
			t.Errorf("Expected player ID %s, got %s", playerID, exclusion.PlayerID)
		}

		if !exclusion.IsActive {
			t.Error("Expected exclusion to be active")
		}

		if exclusion.ExpiresAt == nil {
			t.Error("Expected expiry date to be set")
		}
	})

	t.Run("CheckIsExcluded", func(t *testing.T) {
		excluded, err := svc.IsExcluded(ctx, playerID)
		if err != nil {
			t.Fatalf("Failed to check exclusion: %v", err)
		}

		if !excluded {
			t.Error("Expected player to be excluded")
		}
	})

	t.Run("CheckNonExcludedPlayer", func(t *testing.T) {
		// Create another player who is not excluded
		otherPlayerID := uuid.New().String()

		excluded, err := svc.IsExcluded(ctx, otherPlayerID)
		if err != nil {
			t.Fatalf("Failed to check exclusion: %v", err)
		}

		if excluded {
			t.Error("Expected player to not be excluded")
		}
	})
}

func TestSelfExclusionPermanent(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("PermanentExclusion", func(t *testing.T) {
		exclusion, err := svc.SelfExclude(ctx, playerID, "Permanent exclusion", nil)
		if err != nil {
			t.Fatalf("Failed to self-exclude: %v", err)
		}

		if exclusion.ExpiresAt != nil {
			t.Error("Expected nil expiry for permanent exclusion")
		}

		if !exclusion.IsActive {
			t.Error("Expected exclusion to be active")
		}
	})
}

func TestCheckDepositLimit(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	// Set a daily deposit limit
	_, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
		PlayerID: playerID,
		Period:   "daily",
		Amount:   10000, // $100
	})
	if err != nil {
		t.Fatalf("Failed to set deposit limit: %v", err)
	}

	t.Run("DepositWithinLimit", func(t *testing.T) {
		amount := domain.Money{Amount: 5000, Currency: "USD"} // $50
		err := svc.CheckDepositLimit(ctx, playerID, amount)
		if err != nil {
			t.Errorf("Expected deposit within limit to be allowed: %v", err)
		}
	})

	// Note: Testing exceeding limits would require creating actual deposit transactions
	// This is tested more thoroughly in integration tests
}

func TestCheckWagerLimit(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	// Set a daily wager limit
	_, err := svc.SetWagerLimit(ctx, &SetWagerLimitRequest{
		PlayerID: playerID,
		Period:   "daily",
		Amount:   5000, // $50
	})
	if err != nil {
		t.Fatalf("Failed to set wager limit: %v", err)
	}

	t.Run("WagerWithinLimit", func(t *testing.T) {
		amount := domain.Money{Amount: 1000, Currency: "USD"} // $10
		err := svc.CheckWagerLimit(ctx, playerID, amount)
		if err != nil {
			t.Errorf("Expected wager within limit to be allowed: %v", err)
		}
	})
}

func TestLimitDecreaseTakesEffectImmediately(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	// Set initial limit
	_, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
		PlayerID: playerID,
		Period:   "daily",
		Amount:   10000, // $100
	})
	if err != nil {
		t.Fatalf("Failed to set initial limit: %v", err)
	}

	// Decrease limit (should be immediate)
	limits, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
		PlayerID: playerID,
		Period:   "daily",
		Amount:   5000, // $50 - decrease
	})
	if err != nil {
		t.Fatalf("Failed to decrease limit: %v", err)
	}

	// Effective at should be now (immediate)
	if limits.EffectiveAt.After(time.Now().Add(time.Second)) {
		t.Error("Limit decrease should be effective immediately")
	}
}

func TestLimitIncreaseRequiresCoolingOff(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	// Set initial limit
	_, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
		PlayerID: playerID,
		Period:   "daily",
		Amount:   5000, // $50
	})
	if err != nil {
		t.Fatalf("Failed to set initial limit: %v", err)
	}

	// Increase limit (should require cooling off)
	limits, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
		PlayerID: playerID,
		Period:   "daily",
		Amount:   10000, // $100 - increase
	})
	if err != nil {
		t.Fatalf("Failed to increase limit: %v", err)
	}

	// Effective at should be in the future (after cooling off period)
	expectedEarliest := time.Now().Add(CoolingOffPeriod - time.Minute)
	if limits.EffectiveAt.Before(expectedEarliest) {
		t.Errorf("Limit increase should have cooling off period. Effective at: %v, Expected after: %v",
			limits.EffectiveAt, expectedEarliest)
	}
}

func TestRemoveLimitRequiresCoolingOff(t *testing.T) {
	svc, playerID, cleanup := setupTestLimits(t)
	defer cleanup()

	ctx := context.Background()

	// Set initial limit
	_, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
		PlayerID: playerID,
		Period:   "daily",
		Amount:   5000, // $50
	})
	if err != nil {
		t.Fatalf("Failed to set initial limit: %v", err)
	}

	// Remove limit (amount = 0 should require cooling off)
	limits, err := svc.SetDepositLimit(ctx, &SetDepositLimitRequest{
		PlayerID: playerID,
		Period:   "daily",
		Amount:   0, // Remove limit
	})
	if err != nil {
		t.Fatalf("Failed to remove limit: %v", err)
	}

	// Effective at should be in the future
	expectedEarliest := time.Now().Add(CoolingOffPeriod - time.Minute)
	if limits.EffectiveAt.Before(expectedEarliest) {
		t.Error("Limit removal should have cooling off period")
	}
}

