package wallet

import (
	"context"
	"testing"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/database"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/google/uuid"
)

func setupTestWallet(t *testing.T) (*Service, string, func()) {
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
	svc := New(db.DB, auditSvc, "USD")

	// Create a test player
	playerID := uuid.New().String()
	_, err = db.DB.Exec(`
		INSERT INTO players (id, username, email, password_hash, status, registration_date, tc_accepted_at, created_at, updated_at)
		VALUES ($1, 'testplayer', 'test@example.com', 'hash', 'active', NOW(), NOW(), NOW(), NOW())
	`, playerID)
	if err != nil {
		t.Fatalf("Failed to create test player: %v", err)
	}

	// Create balance record
	_, err = db.DB.Exec(`
		INSERT INTO balances (player_id, real_money_amount, real_money_currency, bonus_amount, bonus_currency, updated_at)
		VALUES ($1, 0, 'USD', 0, 'USD', NOW())
	`, playerID)
	if err != nil {
		t.Fatalf("Failed to create balance: %v", err)
	}

	return svc, playerID, func() {
		db.CleanData()
		db.Close()
	}
}

func TestGetBalance(t *testing.T) {
	svc, playerID, cleanup := setupTestWallet(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("InitialBalance", func(t *testing.T) {
		balance, err := svc.GetBalance(ctx, playerID)
		if err != nil {
			t.Fatalf("Failed to get balance: %v", err)
		}

		if balance.Available.Amount != 0 {
			t.Errorf("Expected initial balance 0, got %d", balance.Available.Amount)
		}
	})
}

func TestDeposit(t *testing.T) {
	svc, playerID, cleanup := setupTestWallet(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("SuccessfulDeposit", func(t *testing.T) {
		result, err := svc.Deposit(ctx, playerID, domain.NewMoney(100.00, "USD"), "test-deposit")
		if err != nil {
			t.Fatalf("Deposit failed: %v", err)
		}

		if result.BalanceAfter.Amount != 10000 { // 100.00 * 100 cents
			t.Errorf("Expected balance 10000 cents, got %d", result.BalanceAfter.Amount)
		}

		if result.ID == "" {
			t.Error("Expected transaction ID")
		}
	})

	t.Run("MultipleDeposits", func(t *testing.T) {
		// Make additional deposits
		svc.Deposit(ctx, playerID, domain.NewMoney(50.00, "USD"), "deposit-2")
		result, _ := svc.Deposit(ctx, playerID, domain.NewMoney(25.00, "USD"), "deposit-3")

		// Should be 100 + 50 + 25 = 175
		if result.BalanceAfter.Float64() != 175.00 {
			t.Errorf("Expected balance 175.00, got %f", result.BalanceAfter.Float64())
		}
	})

	t.Run("ZeroAmount", func(t *testing.T) {
		_, err := svc.Deposit(ctx, playerID, domain.NewMoney(0, "USD"), "zero")
		if err == nil {
			t.Error("Expected error for zero deposit")
		}
	})

	t.Run("NegativeAmount", func(t *testing.T) {
		_, err := svc.Deposit(ctx, playerID, domain.NewMoney(-50, "USD"), "negative")
		if err == nil {
			t.Error("Expected error for negative deposit")
		}
	})

	t.Run("InvalidPlayer", func(t *testing.T) {
		_, err := svc.Deposit(ctx, uuid.New().String(), domain.NewMoney(100, "USD"), "invalid")
		if err == nil {
			t.Error("Expected error for invalid player")
		}
	})
}

func TestWithdraw(t *testing.T) {
	svc, playerID, cleanup := setupTestWallet(t)
	defer cleanup()

	ctx := context.Background()

	// Deposit first
	svc.Deposit(ctx, playerID, domain.NewMoney(100.00, "USD"), "initial")

	t.Run("SuccessfulWithdrawal", func(t *testing.T) {
		result, err := svc.Withdraw(ctx, playerID, domain.NewMoney(30.00, "USD"), "withdraw-1")
		if err != nil {
			t.Fatalf("Withdrawal failed: %v", err)
		}

		if result.BalanceAfter.Float64() != 70.00 {
			t.Errorf("Expected balance 70.00, got %f", result.BalanceAfter.Float64())
		}
	})

	t.Run("InsufficientFunds", func(t *testing.T) {
		_, err := svc.Withdraw(ctx, playerID, domain.NewMoney(1000.00, "USD"), "too-much")
		if err == nil {
			t.Error("Expected insufficient funds error")
		}
	})

	t.Run("ExactBalance", func(t *testing.T) {
		balance, _ := svc.GetBalance(ctx, playerID)
		result, err := svc.Withdraw(ctx, playerID, balance.Available, "exact")
		if err != nil {
			t.Fatalf("Should allow withdrawing exact balance: %v", err)
		}

		if result.BalanceAfter.Amount != 0 {
			t.Errorf("Expected 0 balance after exact withdrawal, got %d", result.BalanceAfter.Amount)
		}
	})
}

func TestPlaceWager(t *testing.T) {
	svc, playerID, cleanup := setupTestWallet(t)
	defer cleanup()

	ctx := context.Background()

	// Deposit first
	svc.Deposit(ctx, playerID, domain.NewMoney(100.00, "USD"), "initial")

	t.Run("SuccessfulWager", func(t *testing.T) {
		result, err := svc.PlaceWager(ctx, playerID, domain.NewMoney(10.00, "USD"), "game-1", "cycle-1")
		if err != nil {
			t.Fatalf("Wager failed: %v", err)
		}

		if result.BalanceAfter.Float64() != 90.00 {
			t.Errorf("Expected balance 90.00, got %f", result.BalanceAfter.Float64())
		}
	})

	t.Run("InsufficientFunds", func(t *testing.T) {
		_, err := svc.PlaceWager(ctx, playerID, domain.NewMoney(1000.00, "USD"), "game-1", "cycle-2")
		if err == nil {
			t.Error("Expected insufficient funds error")
		}
	})
}

func TestCreditWin(t *testing.T) {
	svc, playerID, cleanup := setupTestWallet(t)
	defer cleanup()

	ctx := context.Background()

	// Deposit and wager
	svc.Deposit(ctx, playerID, domain.NewMoney(100.00, "USD"), "initial")
	svc.PlaceWager(ctx, playerID, domain.NewMoney(10.00, "USD"), "game-1", "cycle-1")

	t.Run("SuccessfulWin", func(t *testing.T) {
		result, err := svc.CreditWin(ctx, playerID, domain.NewMoney(50.00, "USD"), "game-1", "cycle-1")
		if err != nil {
			t.Fatalf("Win credit failed: %v", err)
		}

		// 100 - 10 + 50 = 140
		if result.BalanceAfter.Float64() != 140.00 {
			t.Errorf("Expected balance 140.00, got %f", result.BalanceAfter.Float64())
		}
	})
}

func TestGetTransactions(t *testing.T) {
	svc, playerID, cleanup := setupTestWallet(t)
	defer cleanup()

	ctx := context.Background()

	// Create some transactions
	svc.Deposit(ctx, playerID, domain.NewMoney(100.00, "USD"), "deposit-1")
	svc.Deposit(ctx, playerID, domain.NewMoney(50.00, "USD"), "deposit-2")
	svc.Withdraw(ctx, playerID, domain.NewMoney(25.00, "USD"), "withdraw-1")

	t.Run("GetAllTransactions", func(t *testing.T) {
		txs, err := svc.GetTransactions(ctx, playerID, 100)
		if err != nil {
			t.Fatalf("Failed to get transactions: %v", err)
		}

		if len(txs) != 3 {
			t.Errorf("Expected 3 transactions, got %d", len(txs))
		}
	})

	t.Run("LimitedTransactions", func(t *testing.T) {
		txs, err := svc.GetTransactions(ctx, playerID, 2)
		if err != nil {
			t.Fatalf("Failed to get transactions: %v", err)
		}

		if len(txs) != 2 {
			t.Errorf("Expected 2 transactions, got %d", len(txs))
		}
	})
}

func TestSequentialWithdrawals(t *testing.T) {
	svc, playerID, cleanup := setupTestWallet(t)
	defer cleanup()

	ctx := context.Background()

	// Deposit initial amount
	svc.Deposit(ctx, playerID, domain.NewMoney(500.00, "USD"), "initial")

	t.Run("MultipleWithdrawals", func(t *testing.T) {
		// Make 5 sequential withdrawals of $100
		for i := 0; i < 5; i++ {
			_, err := svc.Withdraw(ctx, playerID, domain.NewMoney(100.00, "USD"), "withdraw")
			if err != nil {
				t.Fatalf("Withdrawal %d failed: %v", i+1, err)
			}
		}

		// Check final balance is 0
		balance, err := svc.GetBalance(ctx, playerID)
		if err != nil {
			t.Fatalf("Failed to get balance: %v", err)
		}
		if balance.Available.Amount != 0 {
			t.Errorf("Expected final balance 0, got %d", balance.Available.Amount)
		}

		// Next withdrawal should fail
		_, err = svc.Withdraw(ctx, playerID, domain.NewMoney(1.00, "USD"), "overdraft")
		if err == nil {
			t.Error("Expected insufficient funds error")
		}
	})
}
