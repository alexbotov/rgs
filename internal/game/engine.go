// Package game provides the game engine and game implementations
// Compliant with GLI-19 Chapter 4: Game Requirements
package game

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/alexbotov/rgs/internal/rng"
	"github.com/alexbotov/rgs/internal/wallet"
	"github.com/google/uuid"
)

var (
	ErrGameNotFound        = errors.New("game not found")
	ErrGameDisabled        = errors.New("game is disabled")
	ErrSessionNotFound     = errors.New("game session not found")
	ErrSessionNotActive    = errors.New("game session is not active")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrInvalidWager        = errors.New("invalid wager amount")
)

// Engine provides game execution functionality
// GLI-19 §4.1: Game Requirements
type Engine struct {
	db       *sql.DB
	rng      *rng.Service
	wallet   *wallet.Service
	audit    *audit.Service
	games    map[string]*domain.Game
	currency string
}

// New creates a new game engine
func New(db *sql.DB, rngSvc *rng.Service, walletSvc *wallet.Service, auditSvc *audit.Service, currency string) *Engine {
	engine := &Engine{
		db:       db,
		rng:      rngSvc,
		wallet:   walletSvc,
		audit:    auditSvc,
		games:    make(map[string]*domain.Game),
		currency: currency,
	}

	// Register available games
	engine.registerGames()

	return engine
}

// registerGames registers all available games
func (e *Engine) registerGames() {
	// Fortune Slots - A simple 3-reel slot game
	// GLI-19 §4.7.1: Minimum 75% RTP
	e.games["fortune-slots"] = &domain.Game{
		ID:             "fortune-slots",
		Name:           "Fortune Slots",
		Type:           "slots",
		TheoreticalRTP: 0.96, // 96% RTP
		MinBet:         domain.Money{Amount: 10, Currency: e.currency},    // $0.10
		MaxBet:         domain.Money{Amount: 10000, Currency: e.currency}, // $100.00
		Enabled:        true,
	}

	// Lucky Sevens - A classic fruit machine
	e.games["lucky-sevens"] = &domain.Game{
		ID:             "lucky-sevens",
		Name:           "Lucky Sevens",
		Type:           "slots",
		TheoreticalRTP: 0.94, // 94% RTP
		MinBet:         domain.Money{Amount: 25, Currency: e.currency},   // $0.25
		MaxBet:         domain.Money{Amount: 5000, Currency: e.currency}, // $50.00
		Enabled:        true,
	}
}

// GetGames returns all available games
func (e *Engine) GetGames() []*domain.Game {
	games := make([]*domain.Game, 0, len(e.games))
	for _, g := range e.games {
		games = append(games, g)
	}
	return games
}

// GetGame returns a game by ID
func (e *Engine) GetGame(gameID string) (*domain.Game, error) {
	game, ok := e.games[gameID]
	if !ok {
		return nil, ErrGameNotFound
	}
	return game, nil
}

// StartSession creates a new game session (GLI-19 §4.3)
func (e *Engine) StartSession(ctx context.Context, playerID, gameID string) (*domain.GameSession, error) {
	game, err := e.GetGame(gameID)
	if err != nil {
		return nil, err
	}
	if !game.Enabled {
		return nil, ErrGameDisabled
	}

	// Get player balance
	balance, err := e.wallet.GetBalance(ctx, playerID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session := &domain.GameSession{
		ID:             uuid.New().String(),
		PlayerID:       playerID,
		GameID:         gameID,
		StartedAt:      now,
		LastActivityAt: now,
		Status:         domain.GameSessionActive,
		OpeningBalance: balance.Available,
		CurrentBalance: balance.Available,
		TotalWagered:   domain.Money{Amount: 0, Currency: e.currency},
		TotalWon:       domain.Money{Amount: 0, Currency: e.currency},
		GamesPlayed:    0,
	}

	// Store session
	_, err = e.db.ExecContext(ctx, `
		INSERT INTO game_sessions (id, player_id, game_id, started_at, last_activity_at, status, opening_balance, current_balance, total_wagered, total_won, games_played, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, session.ID, session.PlayerID, session.GameID, session.StartedAt, session.LastActivityAt,
		session.Status, session.OpeningBalance.Amount, session.CurrentBalance.Amount,
		session.TotalWagered.Amount, session.TotalWon.Amount, session.GamesPlayed, e.currency)
	if err != nil {
		return nil, fmt.Errorf("failed to create game session: %w", err)
	}

	// Audit log
	e.audit.Log(ctx, audit.EventGameSessionStart, domain.SeverityInfo,
		fmt.Sprintf("Game session started: %s", game.Name),
		map[string]string{"session_id": session.ID, "game_id": gameID},
		audit.WithPlayer(playerID), audit.WithSession(session.ID))

	return session, nil
}

// GetSession retrieves a game session
func (e *Engine) GetSession(ctx context.Context, sessionID string) (*domain.GameSession, error) {
	var session domain.GameSession
	var endedAt sql.NullTime
	var openingBal, currentBal, wagered, won int64
	var currency string

	err := e.db.QueryRowContext(ctx, `
		SELECT id, player_id, game_id, started_at, ended_at, last_activity_at, status, 
		       opening_balance, current_balance, total_wagered, total_won, games_played, currency
		FROM game_sessions WHERE id = $1
	`, sessionID).Scan(
		&session.ID, &session.PlayerID, &session.GameID, &session.StartedAt, &endedAt,
		&session.LastActivityAt, &session.Status, &openingBal, &currentBal, &wagered, &won,
		&session.GamesPlayed, &currency)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	if endedAt.Valid {
		session.EndedAt = &endedAt.Time
	}
	session.OpeningBalance = domain.Money{Amount: openingBal, Currency: currency}
	session.CurrentBalance = domain.Money{Amount: currentBal, Currency: currency}
	session.TotalWagered = domain.Money{Amount: wagered, Currency: currency}
	session.TotalWon = domain.Money{Amount: won, Currency: currency}

	return &session, nil
}

// EndSession closes a game session (GLI-19 §4.3)
func (e *Engine) EndSession(ctx context.Context, sessionID string) (*domain.GameSession, error) {
	session, err := e.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	session.EndedAt = &now
	session.Status = domain.GameSessionCompleted

	_, err = e.db.ExecContext(ctx, `
		UPDATE game_sessions SET ended_at = $1, status = $2 WHERE id = $3
	`, now, session.Status, sessionID)
	if err != nil {
		return nil, err
	}

	// Audit log
	e.audit.Log(ctx, audit.EventGameSessionEnd, domain.SeverityInfo,
		fmt.Sprintf("Game session ended: %d games played", session.GamesPlayed),
		map[string]interface{}{
			"session_id":    session.ID,
			"games_played":  session.GamesPlayed,
			"total_wagered": session.TotalWagered.Float64(),
			"total_won":     session.TotalWon.Float64(),
		},
		audit.WithPlayer(session.PlayerID), audit.WithSession(sessionID))

	return session, nil
}

// PlayRequest contains the data for playing a game
type PlayRequest struct {
	SessionID   string `json:"session_id"`
	WagerAmount int64  `json:"wager_amount"` // In cents
}

// PlayResult contains the result of a game cycle
type PlayResult struct {
	CycleID     string       `json:"cycle_id"`
	Outcome     *SlotOutcome `json:"outcome"`
	WagerAmount domain.Money `json:"wager_amount"`
	WinAmount   domain.Money `json:"win_amount"`
	Balance     domain.Money `json:"balance"`
}

// Play executes a game cycle (GLI-19 §4.3.3, §4.5)
func (e *Engine) Play(ctx context.Context, req *PlayRequest) (*PlayResult, error) {
	// Get session
	session, err := e.GetSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != domain.GameSessionActive {
		return nil, ErrSessionNotActive
	}

	// Get game
	game, err := e.GetGame(session.GameID)
	if err != nil {
		return nil, err
	}
	if !game.Enabled {
		return nil, ErrGameDisabled
	}

	// Validate wager (GLI-19 §4.3.3.b)
	wager := domain.Money{Amount: req.WagerAmount, Currency: e.currency}
	if wager.Amount < game.MinBet.Amount || wager.Amount > game.MaxBet.Amount {
		return nil, ErrInvalidWager
	}

	// Get current balance
	balance, err := e.wallet.GetBalance(ctx, session.PlayerID)
	if err != nil {
		return nil, err
	}
	if balance.Available.Amount < wager.Amount {
		return nil, ErrInsufficientBalance
	}

	now := time.Now().UTC()
	cycleID := uuid.New().String()

	// Deduct wager (GLI-19 §4.3.3.b)
	_, err = e.wallet.PlaceWager(ctx, session.PlayerID, wager, session.GameID, cycleID)
	if err != nil {
		return nil, err
	}

	// Generate outcome using RNG (GLI-19 §4.5)
	outcome, err := e.generateSlotOutcome(game)
	if err != nil {
		// Refund on error
		e.wallet.CreditWin(ctx, session.PlayerID, wager, session.GameID, cycleID)
		return nil, fmt.Errorf("failed to generate outcome: %w", err)
	}

	// Calculate win based on outcome
	winAmount := e.calculateWin(outcome, wager)

	// Credit win if any (GLI-19 §4.3.3.d)
	if winAmount.Amount > 0 {
		_, err = e.wallet.CreditWin(ctx, session.PlayerID, winAmount, session.GameID, cycleID)
		if err != nil {
			return nil, err
		}
	}

	// Get updated balance
	newBalance, _ := e.wallet.GetBalance(ctx, session.PlayerID)

	// Store game cycle (GLI-19 §2.8.2)
	outcomeJSON, _ := json.Marshal(outcome)
	completedAt := now
	cycle := &domain.GameCycle{
		ID:            cycleID,
		SessionID:     session.ID,
		PlayerID:      session.PlayerID,
		GameID:        session.GameID,
		StartedAt:     now,
		CompletedAt:   &completedAt,
		WagerAmount:   wager,
		WinAmount:     winAmount,
		BalanceBefore: balance.Available,
		BalanceAfter:  newBalance.Available,
		Outcome:       outcomeJSON,
		Status:        domain.CycleStatusCompleted,
	}

	_, err = e.db.ExecContext(ctx, `
		INSERT INTO game_cycles (id, session_id, player_id, game_id, started_at, completed_at, wager_amount, win_amount, balance_before, balance_after, outcome, status, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, cycle.ID, cycle.SessionID, cycle.PlayerID, cycle.GameID, cycle.StartedAt, cycle.CompletedAt,
		cycle.WagerAmount.Amount, cycle.WinAmount.Amount, cycle.BalanceBefore.Amount, cycle.BalanceAfter.Amount,
		string(outcomeJSON), cycle.Status, e.currency)
	if err != nil {
		return nil, err
	}

	// Update session stats
	_, err = e.db.ExecContext(ctx, `
		UPDATE game_sessions SET 
			last_activity_at = $1,
			current_balance = $2,
			total_wagered = total_wagered + $3,
			total_won = total_won + $4,
			games_played = games_played + 1
		WHERE id = $5
	`, now, newBalance.Available.Amount, wager.Amount, winAmount.Amount, session.ID)
	if err != nil {
		return nil, err
	}

	// Audit log for large wins (GLI-19 §2.8.8)
	if winAmount.Amount >= 10000 { // $100+ wins
		e.audit.Log(ctx, audit.EventLargeWin, domain.SeverityInfo,
			fmt.Sprintf("Large win: %.2f %s", winAmount.Float64(), winAmount.Currency),
			map[string]interface{}{
				"cycle_id": cycleID,
				"win":      winAmount.Float64(),
				"wager":    wager.Float64(),
				"game_id":  session.GameID,
			},
			audit.WithPlayer(session.PlayerID), audit.WithSession(session.ID))
	}

	return &PlayResult{
		CycleID:     cycleID,
		Outcome:     outcome,
		WagerAmount: wager,
		WinAmount:   winAmount,
		Balance:     newBalance.Available,
	}, nil
}

// GetHistory retrieves game history (GLI-19 §4.14)
func (e *Engine) GetHistory(ctx context.Context, playerID string, limit int) ([]*domain.GameRecall, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := e.db.QueryContext(ctx, `
		SELECT id, game_id, started_at, wager_amount, win_amount, balance_before, balance_after, outcome, currency
		FROM game_cycles WHERE player_id = $1 ORDER BY started_at DESC LIMIT $2
	`, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []*domain.GameRecall
	for rows.Next() {
		var recall domain.GameRecall
		var wager, win, balBefore, balAfter int64
		var outcome, currency string

		err := rows.Scan(&recall.CycleID, &recall.GameID, &recall.PlayedAt,
			&wager, &win, &balBefore, &balAfter, &outcome, &currency)
		if err != nil {
			return nil, err
		}

		recall.WagerAmount = domain.Money{Amount: wager, Currency: currency}
		recall.WinAmount = domain.Money{Amount: win, Currency: currency}
		recall.BalanceBefore = domain.Money{Amount: balBefore, Currency: currency}
		recall.BalanceAfter = domain.Money{Amount: balAfter, Currency: currency}
		recall.Outcome = json.RawMessage(outcome)

		history = append(history, &recall)
	}

	return history, nil
}
