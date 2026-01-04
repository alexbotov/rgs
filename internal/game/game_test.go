package game

import (
	"context"
	"testing"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/database"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/alexbotov/rgs/internal/rng"
	"github.com/alexbotov/rgs/internal/wallet"
	"github.com/google/uuid"
)

func setupTestEngine(t *testing.T) (*Engine, string, func()) {
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

	// Create services
	auditSvc := audit.New(db.DB)
	rngSvc := rng.New()
	walletSvc := wallet.New(db.DB, auditSvc, "USD")

	// Create engine
	engine := New(db.DB, rngSvc, walletSvc, auditSvc, "USD")

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

	// Fund the player
	walletSvc.Deposit(context.Background(), playerID, domain.NewMoney(1000.00, "USD"), "test-funding")

	return engine, playerID, func() {
		db.CleanData()
		db.Close()
	}
}

func TestGetGames(t *testing.T) {
	engine, _, cleanup := setupTestEngine(t)
	defer cleanup()

	games := engine.GetGames()

	if len(games) == 0 {
		t.Error("Expected at least one game")
	}

	// Find Fortune Slots
	found := false
	for _, g := range games {
		if g.ID == "fortune-slots" {
			found = true
			if g.Name != "Fortune Slots" {
				t.Errorf("Expected 'Fortune Slots', got '%s'", g.Name)
			}
			if g.TheoreticalRTP < 0.90 || g.TheoreticalRTP > 1.0 {
				t.Errorf("Unexpected RTP: %f", g.TheoreticalRTP)
			}
			break
		}
	}

	if !found {
		t.Error("Fortune Slots not found in game list")
	}
}

func TestGetGame(t *testing.T) {
	engine, _, cleanup := setupTestEngine(t)
	defer cleanup()

	t.Run("ExistingGame", func(t *testing.T) {
		game, err := engine.GetGame("fortune-slots")
		if err != nil {
			t.Fatalf("Failed to get game: %v", err)
		}

		if game.ID != "fortune-slots" {
			t.Errorf("Expected 'fortune-slots', got '%s'", game.ID)
		}
	})

	t.Run("NonexistentGame", func(t *testing.T) {
		_, err := engine.GetGame("nonexistent-game")
		if err == nil {
			t.Error("Expected error for nonexistent game")
		}
	})
}

func TestStartSession(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("SuccessfulSessionStart", func(t *testing.T) {
		session, err := engine.StartSession(ctx, playerID, "fortune-slots")
		if err != nil {
			t.Fatalf("Failed to start session: %v", err)
		}

		if session.ID == "" {
			t.Error("Expected session ID")
		}
		if session.GameID != "fortune-slots" {
			t.Errorf("Expected game 'fortune-slots', got '%s'", session.GameID)
		}
		if session.Status != domain.GameSessionActive {
			t.Errorf("Expected active status, got '%s'", session.Status)
		}
	})

	t.Run("InvalidGame", func(t *testing.T) {
		_, err := engine.StartSession(ctx, playerID, "nonexistent")
		if err == nil {
			t.Error("Expected error for invalid game")
		}
	})

	t.Run("InvalidPlayer", func(t *testing.T) {
		_, err := engine.StartSession(ctx, uuid.New().String(), "fortune-slots")
		if err == nil {
			t.Error("Expected error for invalid player")
		}
	})
}

func TestPlay(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Start a session
	session, _ := engine.StartSession(ctx, playerID, "fortune-slots")

	t.Run("SuccessfulPlay", func(t *testing.T) {
		result, err := engine.Play(ctx, &PlayRequest{
			SessionID:   session.ID,
			WagerAmount: 500, // $5 bet in cents
		})

		if err != nil {
			t.Fatalf("Play failed: %v", err)
		}

		if result.CycleID == "" {
			t.Error("Expected cycle ID")
		}

		if result.WagerAmount.Float64() != 5.00 {
			t.Errorf("Expected wager $5.00, got $%f", result.WagerAmount.Float64())
		}

		// Win amount should be non-negative
		if result.WinAmount.Amount < 0 {
			t.Error("Win amount should not be negative")
		}

		// Outcome should have reels
		if len(result.Outcome.Reels) != 3 {
			t.Errorf("Expected 3 reels, got %d", len(result.Outcome.Reels))
		}
	})

	t.Run("MultiplePlays", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			result, err := engine.Play(ctx, &PlayRequest{
				SessionID:   session.ID,
				WagerAmount: 100, // $1.00
			})

			if err != nil {
				t.Fatalf("Play %d failed: %v", i+1, err)
			}

			if result.Outcome.Reels == nil {
				t.Errorf("Play %d: Missing outcome reels", i+1)
			}
		}
	})

	t.Run("BelowMinimumWager", func(t *testing.T) {
		_, err := engine.Play(ctx, &PlayRequest{
			SessionID:   session.ID,
			WagerAmount: 1, // $0.01 - Below minimum
		})

		if err == nil {
			t.Error("Expected error for below-minimum wager")
		}
	})

	t.Run("AboveMaximumWager", func(t *testing.T) {
		_, err := engine.Play(ctx, &PlayRequest{
			SessionID:   session.ID,
			WagerAmount: 1000000, // $10,000 - Above maximum
		})

		if err == nil {
			t.Error("Expected error for above-maximum wager")
		}
	})

	t.Run("InvalidSession", func(t *testing.T) {
		_, err := engine.Play(ctx, &PlayRequest{
			SessionID:   uuid.New().String(),
			WagerAmount: 100,
		})

		if err == nil {
			t.Error("Expected error for invalid session")
		}
	})
}

func TestEndSession(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Start a session and play some games
	session, _ := engine.StartSession(ctx, playerID, "fortune-slots")
	engine.Play(ctx, &PlayRequest{
		SessionID:   session.ID,
		WagerAmount: 100,
	})

	t.Run("SuccessfulEndSession", func(t *testing.T) {
		endedSession, err := engine.EndSession(ctx, session.ID)
		if err != nil {
			t.Fatalf("Failed to end session: %v", err)
		}

		if endedSession.GamesPlayed < 1 {
			t.Error("Expected at least 1 game played")
		}
		if endedSession.Status != domain.GameSessionCompleted {
			t.Errorf("Expected completed status, got '%s'", endedSession.Status)
		}
	})

	t.Run("PlayAfterEndedSession", func(t *testing.T) {
		_, err := engine.Play(ctx, &PlayRequest{
			SessionID:   session.ID,
			WagerAmount: 100,
		})

		if err == nil {
			t.Error("Expected error for playing on ended session")
		}
	})
}

func TestGetHistory(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Start session and play games
	session, _ := engine.StartSession(ctx, playerID, "fortune-slots")
	for i := 0; i < 5; i++ {
		engine.Play(ctx, &PlayRequest{
			SessionID:   session.ID,
			WagerAmount: 100,
		})
	}

	t.Run("GetHistory", func(t *testing.T) {
		history, err := engine.GetHistory(ctx, playerID, 10)
		if err != nil {
			t.Fatalf("Failed to get history: %v", err)
		}

		if len(history) != 5 {
			t.Errorf("Expected 5 history entries, got %d", len(history))
		}

		// Check history is ordered (most recent first)
		for i := 1; i < len(history); i++ {
			if history[i].PlayedAt.After(history[i-1].PlayedAt) {
				t.Error("History should be ordered most recent first")
			}
		}
	})

	t.Run("LimitHistory", func(t *testing.T) {
		history, _ := engine.GetHistory(ctx, playerID, 3)
		if len(history) != 3 {
			t.Errorf("Expected 3 history entries with limit, got %d", len(history))
		}
	})
}

// ============================================================================
// Interrupted Games Tests (GLI-19 §4.16)
// ============================================================================

func TestGetInterruptedGames(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("NoInterruptedGames", func(t *testing.T) {
		interrupted, err := engine.GetInterruptedGames(ctx, playerID)
		if err != nil {
			t.Fatalf("Failed to get interrupted games: %v", err)
		}

		if len(interrupted) != 0 {
			t.Errorf("Expected 0 interrupted games, got %d", len(interrupted))
		}
	})
}

func TestMarkInterrupted(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Start session
	session, err := engine.StartSession(ctx, playerID, "fortune-slots")
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Create a game cycle in progress (simulate by inserting directly)
	cycleID := uuid.New().String()
	_, err = engine.db.ExecContext(ctx, `
		INSERT INTO game_cycles (id, session_id, player_id, game_id, started_at, wager_amount, win_amount, balance_before, balance_after, outcome, status, currency)
		VALUES ($1, $2, $3, $4, NOW(), 100, 0, 100000, 99900, '{"reels":["7","7","7"]}', $5, 'USD')
	`, cycleID, session.ID, playerID, "fortune-slots", domain.CycleStatusInProgress)
	if err != nil {
		t.Fatalf("Failed to create in-progress cycle: %v", err)
	}

	t.Run("MarkAsInterrupted", func(t *testing.T) {
		err := engine.MarkInterrupted(ctx, cycleID, "Connection lost")
		if err != nil {
			t.Fatalf("Failed to mark as interrupted: %v", err)
		}

		// Verify cycle is now interrupted
		var status domain.GameCycleStatus
		engine.db.QueryRowContext(ctx, "SELECT status FROM game_cycles WHERE id = $1", cycleID).Scan(&status)

		if status != domain.CycleStatusInterrupted {
			t.Errorf("Expected status 'interrupted', got '%s'", status)
		}
	})

	t.Run("GetInterruptedGames", func(t *testing.T) {
		interrupted, err := engine.GetInterruptedGames(ctx, playerID)
		if err != nil {
			t.Fatalf("Failed to get interrupted games: %v", err)
		}

		if len(interrupted) != 1 {
			t.Errorf("Expected 1 interrupted game, got %d", len(interrupted))
		}

		if interrupted[0].CycleID != cycleID {
			t.Errorf("Expected cycle ID %s, got %s", cycleID, interrupted[0].CycleID)
		}
	})
}

func TestVoidGame(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Start session
	session, _ := engine.StartSession(ctx, playerID, "fortune-slots")

	// Create an interrupted game cycle
	cycleID := uuid.New().String()
	_, err := engine.db.ExecContext(ctx, `
		INSERT INTO game_cycles (id, session_id, player_id, game_id, started_at, wager_amount, win_amount, balance_before, balance_after, outcome, status, currency)
		VALUES ($1, $2, $3, $4, NOW(), 500, 0, 100000, 99500, '{"reels":["7","BAR","CHERRY"]}', $5, 'USD')
	`, cycleID, session.ID, playerID, "fortune-slots", domain.CycleStatusInterrupted)
	if err != nil {
		t.Fatalf("Failed to create interrupted cycle: %v", err)
	}

	// Deduct the wager from balance (simulate what happened before interruption)
	_, err = engine.db.ExecContext(ctx, `
		UPDATE balances SET real_money_amount = real_money_amount - 500 WHERE player_id = $1
	`, playerID)
	if err != nil {
		t.Fatalf("Failed to deduct balance: %v", err)
	}

	t.Run("VoidAndRefund", func(t *testing.T) {
		// Get balance before void
		balBefore, _ := engine.wallet.GetBalance(ctx, playerID)

		err := engine.VoidGame(ctx, cycleID, "Player requested void")
		if err != nil {
			t.Fatalf("Failed to void game: %v", err)
		}

		// Verify cycle is now voided
		var status domain.GameCycleStatus
		engine.db.QueryRowContext(ctx, "SELECT status FROM game_cycles WHERE id = $1", cycleID).Scan(&status)

		if status != domain.CycleStatusVoided {
			t.Errorf("Expected status 'voided', got '%s'", status)
		}

		// Verify balance was refunded
		balAfter, _ := engine.wallet.GetBalance(ctx, playerID)
		if balAfter.RealMoney.Amount != balBefore.RealMoney.Amount+500 {
			t.Errorf("Expected balance refund of 500 cents, before: %d, after: %d",
				balBefore.RealMoney.Amount, balAfter.RealMoney.Amount)
		}
	})

	t.Run("VoidAlreadyVoided", func(t *testing.T) {
		err := engine.VoidGame(ctx, cycleID, "Try again")
		if err == nil {
			t.Error("Expected error when voiding already voided game")
		}
	})

	t.Run("VoidNonexistent", func(t *testing.T) {
		err := engine.VoidGame(ctx, uuid.New().String(), "Not found")
		if err == nil {
			t.Error("Expected error when voiding nonexistent game")
		}
	})
}

func TestResumeGame(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Start session
	session, _ := engine.StartSession(ctx, playerID, "fortune-slots")

	// Create an interrupted game cycle with a winning outcome
	cycleID := uuid.New().String()
	_, err := engine.db.ExecContext(ctx, `
		INSERT INTO game_cycles (id, session_id, player_id, game_id, started_at, wager_amount, win_amount, balance_before, balance_after, outcome, status, currency)
		VALUES ($1, $2, $3, $4, NOW(), 100, 0, 100000, 99900, '{"reels":["7","7","7"],"win_lines":[{"line":1,"symbols":["7","7","7"],"win":500}]}', $5, 'USD')
	`, cycleID, session.ID, playerID, "fortune-slots", domain.CycleStatusInterrupted)
	if err != nil {
		t.Fatalf("Failed to create interrupted cycle: %v", err)
	}

	t.Run("ResumeInterruptedGame", func(t *testing.T) {
		result, err := engine.ResumeGame(ctx, cycleID)
		if err != nil {
			t.Fatalf("Failed to resume game: %v", err)
		}

		if result.CycleID != cycleID {
			t.Errorf("Expected cycle ID %s, got %s", cycleID, result.CycleID)
		}

		if result.Outcome == nil {
			t.Error("Expected outcome to be present")
		}

		// Verify cycle is now completed
		var status domain.GameCycleStatus
		engine.db.QueryRowContext(ctx, "SELECT status FROM game_cycles WHERE id = $1", cycleID).Scan(&status)

		if status != domain.CycleStatusCompleted {
			t.Errorf("Expected status 'completed', got '%s'", status)
		}
	})

	t.Run("ResumeAlreadyCompleted", func(t *testing.T) {
		_, err := engine.ResumeGame(ctx, cycleID)
		if err == nil {
			t.Error("Expected error when resuming already completed game")
		}
	})

	t.Run("ResumeNonexistent", func(t *testing.T) {
		_, err := engine.ResumeGame(ctx, uuid.New().String())
		if err == nil {
			t.Error("Expected error when resuming nonexistent game")
		}
	})
}

func TestInterruptedGameFlow(t *testing.T) {
	engine, playerID, cleanup := setupTestEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Start session
	session, _ := engine.StartSession(ctx, playerID, "fortune-slots")

	// Play a game successfully
	result, err := engine.Play(ctx, &PlayRequest{
		SessionID:   session.ID,
		WagerAmount: 100,
	})
	if err != nil {
		t.Fatalf("Failed to play game: %v", err)
	}

	t.Log("Initial play completed successfully")

	// Simulate creating an interrupted game
	interruptedCycleID := uuid.New().String()
	_, err = engine.db.ExecContext(ctx, `
		INSERT INTO game_cycles (id, session_id, player_id, game_id, started_at, wager_amount, win_amount, balance_before, balance_after, outcome, status, currency)
		VALUES ($1, $2, $3, $4, NOW(), 200, 0, $5, $6, '{"reels":["CHERRY","BAR","7"]}', $7, 'USD')
	`, interruptedCycleID, session.ID, playerID, "fortune-slots", 
		result.Balance.Amount, result.Balance.Amount-200, domain.CycleStatusInterrupted)
	if err != nil {
		t.Fatalf("Failed to create interrupted cycle: %v", err)
	}

	t.Run("VerifyInterruptedGameExists", func(t *testing.T) {
		interrupted, err := engine.GetInterruptedGames(ctx, playerID)
		if err != nil {
			t.Fatalf("Failed to get interrupted games: %v", err)
		}

		if len(interrupted) != 1 {
			t.Fatalf("Expected 1 interrupted game, got %d", len(interrupted))
		}

		if interrupted[0].WagerHeld.Amount != 200 {
			t.Errorf("Expected wager held 200, got %d", interrupted[0].WagerHeld.Amount)
		}
	})

	t.Run("ResolveByVoiding", func(t *testing.T) {
		err := engine.VoidGame(ctx, interruptedCycleID, "Test resolution")
		if err != nil {
			t.Fatalf("Failed to void: %v", err)
		}

		// Verify no more interrupted games
		interrupted, _ := engine.GetInterruptedGames(ctx, playerID)
		if len(interrupted) != 0 {
			t.Errorf("Expected 0 interrupted games after void, got %d", len(interrupted))
		}
	})
}
