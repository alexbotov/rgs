// Package domain contains core domain models for the RGS
// Based on GLI-19 Standards for Interactive Gaming Systems V3.0
//
// Key GLI-19 References:
//   - §2.5: Player Account Management
//   - §2.5.6/§2.5.7: Financial Transactions
//   - §4.3: Game Session Management
//   - §4.3.3: Game Cycle Requirements
package domain

import (
	"encoding/json"
	"time"
)

// Money represents monetary values with precision (GLI-19 §2.5.6)
type Money struct {
	Amount   int64  `json:"amount"`   // Amount in smallest unit (cents)
	Currency string `json:"currency"` // ISO 4217 currency code
}

// NewMoney creates a new Money value from dollars/major unit
func NewMoney(amount float64, currency string) Money {
	return Money{
		Amount:   int64(amount * 100),
		Currency: currency,
	}
}

// Float64 returns the monetary value as a float
func (m Money) Float64() float64 {
	return float64(m.Amount) / 100.0
}

// Add adds two money values
func (m Money) Add(other Money) Money {
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}
}

// Sub subtracts money value
func (m Money) Sub(other Money) Money {
	return Money{Amount: m.Amount - other.Amount, Currency: m.Currency}
}

// PlayerStatus represents the status of a player account (GLI-19 §2.5)
type PlayerStatus string

const (
	PlayerStatusPending   PlayerStatus = "pending"
	PlayerStatusActive    PlayerStatus = "active"
	PlayerStatusSuspended PlayerStatus = "suspended"
	PlayerStatusExcluded  PlayerStatus = "excluded"
	PlayerStatusClosed    PlayerStatus = "closed"
)

// Player represents a registered player (GLI-19 §2.5.2)
type Player struct {
	ID               string       `json:"id" db:"id"`
	Username         string       `json:"username" db:"username"`
	Email            string       `json:"email" db:"email"`
	PasswordHash     string       `json:"-" db:"password_hash"`
	Status           PlayerStatus `json:"status" db:"status"`
	RegistrationDate time.Time    `json:"registration_date" db:"registration_date"`
	LastLoginAt      *time.Time   `json:"last_login_at" db:"last_login_at"`
	TCAcceptedAt     time.Time    `json:"tc_accepted_at" db:"tc_accepted_at"`
	CreatedAt        time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at" db:"updated_at"`
}

// SessionStatus represents session state (GLI-19 §2.5.3)
type SessionStatus string

const (
	SessionStatusActive       SessionStatus = "active"
	SessionStatusExpired      SessionStatus = "expired"
	SessionStatusLoggedOut    SessionStatus = "logged_out"
	SessionStatusRequiresAuth SessionStatus = "requires_auth"
)

// Session represents a player session (GLI-19 §2.5.3, §2.5.4)
type Session struct {
	ID             string        `json:"id" db:"id"`
	PlayerID       string        `json:"player_id" db:"player_id"`
	Token          string        `json:"-" db:"token"`
	IPAddress      string        `json:"ip_address" db:"ip_address"`
	UserAgent      string        `json:"user_agent" db:"user_agent"`
	CreatedAt      time.Time     `json:"created_at" db:"created_at"`
	LastActivityAt time.Time     `json:"last_activity_at" db:"last_activity_at"`
	ExpiresAt      time.Time     `json:"expires_at" db:"expires_at"`
	Status         SessionStatus `json:"status" db:"status"`
}

// TransactionType represents transaction types
// GLI-19 §2.5.6 - Financial Transactions: All financial transactions must be logged
// GLI-19 §2.5.7 - Transaction Log: Complete record of all transactions required
type TransactionType string

const (
	TxTypeDeposit    TransactionType = "deposit"
	TxTypeWithdrawal TransactionType = "withdrawal"
	TxTypeWager      TransactionType = "wager"
	TxTypeWin        TransactionType = "win"
	TxTypeBonus      TransactionType = "bonus"
	TxTypeAdjustment TransactionType = "adjustment"
	TxTypeRefund     TransactionType = "refund"
	TxTypeJackpot    TransactionType = "jackpot"
)

// TransactionStatus represents transaction state
type TransactionStatus string

const (
	TxStatusPending   TransactionStatus = "pending"
	TxStatusCompleted TransactionStatus = "completed"
	TxStatusFailed    TransactionStatus = "failed"
	TxStatusCancelled TransactionStatus = "cancelled"
)

// Transaction represents a financial transaction (GLI-19 §2.5.6, §2.5.7)
type Transaction struct {
	ID            string            `json:"id" db:"id"`
	PlayerID      string            `json:"player_id" db:"player_id"`
	Type          TransactionType   `json:"type" db:"type"`
	Amount        Money             `json:"amount" db:"amount"`
	BalanceBefore Money             `json:"balance_before" db:"balance_before"`
	BalanceAfter  Money             `json:"balance_after" db:"balance_after"`
	Status        TransactionStatus `json:"status" db:"status"`
	Reference     string            `json:"reference" db:"reference"`
	Description   string            `json:"description" db:"description"`
	CreatedAt     time.Time         `json:"created_at" db:"created_at"`
	CompletedAt   *time.Time        `json:"completed_at" db:"completed_at"`
}

// GameSessionStatus represents game session state (GLI-19 §4.3)
type GameSessionStatus string

const (
	GameSessionActive      GameSessionStatus = "active"
	GameSessionCompleted   GameSessionStatus = "completed"
	GameSessionInterrupted GameSessionStatus = "interrupted"
)

// GameSession represents a gaming session (GLI-19 §4.3)
type GameSession struct {
	ID             string            `json:"id" db:"id"`
	PlayerID       string            `json:"player_id" db:"player_id"`
	GameID         string            `json:"game_id" db:"game_id"`
	StartedAt      time.Time         `json:"started_at" db:"started_at"`
	EndedAt        *time.Time        `json:"ended_at" db:"ended_at"`
	LastActivityAt time.Time         `json:"last_activity_at" db:"last_activity_at"`
	Status         GameSessionStatus `json:"status" db:"status"`
	OpeningBalance Money             `json:"opening_balance" db:"opening_balance"`
	CurrentBalance Money             `json:"current_balance" db:"current_balance"`
	TotalWagered   Money             `json:"total_wagered" db:"total_wagered"`
	TotalWon       Money             `json:"total_won" db:"total_won"`
	GamesPlayed    int               `json:"games_played" db:"games_played"`
}

// GameCycleStatus represents game cycle state (GLI-19 §4.3.3)
type GameCycleStatus string

const (
	CycleStatusPending     GameCycleStatus = "pending"
	CycleStatusInProgress  GameCycleStatus = "in_progress"
	CycleStatusCompleted   GameCycleStatus = "completed"
	CycleStatusVoided      GameCycleStatus = "voided"
	CycleStatusInterrupted GameCycleStatus = "interrupted"
)

// GameCycle represents a single game round (GLI-19 §4.3.3)
type GameCycle struct {
	ID            string          `json:"id" db:"id"`
	SessionID     string          `json:"session_id" db:"session_id"`
	PlayerID      string          `json:"player_id" db:"player_id"`
	GameID        string          `json:"game_id" db:"game_id"`
	StartedAt     time.Time       `json:"started_at" db:"started_at"`
	CompletedAt   *time.Time      `json:"completed_at" db:"completed_at"`
	WagerAmount   Money           `json:"wager_amount" db:"wager_amount"`
	WinAmount     Money           `json:"win_amount" db:"win_amount"`
	BalanceBefore Money           `json:"balance_before" db:"balance_before"`
	BalanceAfter  Money           `json:"balance_after" db:"balance_after"`
	Outcome       json.RawMessage `json:"outcome" db:"outcome"`
	Status        GameCycleStatus `json:"status" db:"status"`
}

// GameRecall provides game history for display (GLI-19 §4.14)
type GameRecall struct {
	CycleID       string          `json:"cycle_id"`
	GameID        string          `json:"game_id"`
	PlayedAt      time.Time       `json:"played_at"`
	WagerAmount   Money           `json:"wager_amount"`
	WinAmount     Money           `json:"win_amount"`
	BalanceBefore Money           `json:"balance_before"`
	BalanceAfter  Money           `json:"balance_after"`
	Outcome       json.RawMessage `json:"outcome"`
}

// Game represents a game definition
type Game struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Type           string  `json:"type"` // slots, table, card
	TheoreticalRTP float64 `json:"theoretical_rtp"`
	MinBet         Money   `json:"min_bet"`
	MaxBet         Money   `json:"max_bet"`
	Enabled        bool    `json:"enabled"`
}

// EventSeverity represents audit event severity
type EventSeverity string

const (
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityError    EventSeverity = "error"
	SeverityCritical EventSeverity = "critical"
)

// AuditEvent represents a significant event
// GLI-19 §2.8.8 - Significant Event Information: System must log all significant events
// including failed logins, program errors, large wins, and configuration changes
type AuditEvent struct {
	ID          string          `json:"id" db:"id"`
	Type        string          `json:"type" db:"type"`
	Severity    EventSeverity   `json:"severity" db:"severity"`
	Timestamp   time.Time       `json:"timestamp" db:"timestamp"`
	PlayerID    *string         `json:"player_id,omitempty" db:"player_id"`
	SessionID   *string         `json:"session_id,omitempty" db:"session_id"`
	Description string          `json:"description" db:"description"`
	Data        json.RawMessage `json:"data,omitempty" db:"data"`
	IPAddress   string          `json:"ip_address" db:"ip_address"`
	Component   string          `json:"component" db:"component"`
}

// Balance represents player balance (GLI-19 §2.5.7)
type Balance struct {
	PlayerID     string    `json:"player_id"`
	RealMoney    Money     `json:"real_money"`
	BonusBalance Money     `json:"bonus_balance"`
	Available    Money     `json:"available"`
	Currency     string    `json:"currency"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// LimitSource indicates who set the limit
// GLI-19 §2.5.5 - Limitations and Exclusions
type LimitSource string

const (
	LimitSourcePlayer    LimitSource = "player"    // Self-imposed by player
	LimitSourceOperator  LimitSource = "operator"  // Set by operator
	LimitSourceRegulator LimitSource = "regulator" // Required by regulation
)

// PlayerLimits represents player-imposed responsible gaming limits
// GLI-19 §2.5.5 - Limitations and Exclusions: Players must be able to set
// deposit, wager, loss, and session limits. Limit decreases are immediate;
// limit increases require a cooling-off period.
type PlayerLimits struct {
	ID              string      `json:"id" db:"id"`
	PlayerID        string      `json:"player_id" db:"player_id"`
	DailyDeposit    *Money      `json:"daily_deposit,omitempty" db:"daily_deposit"`
	WeeklyDeposit   *Money      `json:"weekly_deposit,omitempty" db:"weekly_deposit"`
	MonthlyDeposit  *Money      `json:"monthly_deposit,omitempty" db:"monthly_deposit"`
	DailyWager      *Money      `json:"daily_wager,omitempty" db:"daily_wager"`
	WeeklyWager     *Money      `json:"weekly_wager,omitempty" db:"weekly_wager"`
	DailyLoss       *Money      `json:"daily_loss,omitempty" db:"daily_loss"`
	WeeklyLoss      *Money      `json:"weekly_loss,omitempty" db:"weekly_loss"`
	SessionDuration *int64      `json:"session_duration_minutes,omitempty" db:"session_duration"` // in minutes
	CoolingOffUntil *time.Time  `json:"cooling_off_until,omitempty" db:"cooling_off_until"`
	Source          LimitSource `json:"source" db:"source"`
	EffectiveAt     time.Time   `json:"effective_at" db:"effective_at"`
	UpdatedAt       time.Time   `json:"updated_at" db:"updated_at"`
}

// SelfExclusion represents a player's self-exclusion record
// GLI-19 §2.5.5.c - Self-Exclusion: Players must be able to self-exclude
// with minimum cooling-off periods before removal
type SelfExclusion struct {
	ID          string     `json:"id" db:"id"`
	PlayerID    string     `json:"player_id" db:"player_id"`
	Reason      string     `json:"reason" db:"reason"`
	StartedAt   time.Time  `json:"started_at" db:"started_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"` // nil = permanent
	RemovedAt   *time.Time `json:"removed_at,omitempty" db:"removed_at"`
	RemovedBy   *string    `json:"removed_by,omitempty" db:"removed_by"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// InterruptedGame represents a game that was interrupted before completion
// GLI-19 §4.16 - Interrupted Games: System must handle game interruptions
// gracefully and allow resumption or voiding with proper refunds
type InterruptedGame struct {
	CycleID       string          `json:"cycle_id" db:"cycle_id"`
	SessionID     string          `json:"session_id" db:"session_id"`
	PlayerID      string          `json:"player_id" db:"player_id"`
	GameID        string          `json:"game_id" db:"game_id"`
	InterruptedAt time.Time       `json:"interrupted_at" db:"interrupted_at"`
	Reason        string          `json:"reason" db:"reason"`
	WagerHeld     Money           `json:"wager_held" db:"wager_held"`
	GameState     json.RawMessage `json:"game_state" db:"game_state"`
	CanResume     bool            `json:"can_resume" db:"can_resume"`
	ResolvedAt    *time.Time      `json:"resolved_at,omitempty" db:"resolved_at"`
	Resolution    string          `json:"resolution,omitempty" db:"resolution"` // resumed, voided, completed
}

// GamingSystemStatus represents the overall gaming system state
// GLI-19 §2.4 - Gaming Management: Operator must be able to disable gaming on demand
type GamingSystemStatus struct {
	GamingEnabled     bool      `json:"gaming_enabled"`
	DisabledAt        *time.Time `json:"disabled_at,omitempty"`
	DisabledBy        string    `json:"disabled_by,omitempty"`
	DisabledReason    string    `json:"disabled_reason,omitempty"`
	ActiveSessions    int64     `json:"active_sessions"`
	LastStateChange   time.Time `json:"last_state_change"`
}

