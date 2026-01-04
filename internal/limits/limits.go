// Package limits provides player limit management
// Compliant with GLI-19 §2.5.5: Limitations and Exclusions
//
// Key Requirements:
//   - Players can set deposit, wager, loss, and session duration limits
//   - Limit decreases take effect immediately
//   - Limit increases require a 24-hour cooling-off period
//   - Self-exclusion must be supported with minimum cooling-off before removal
package limits

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/alexbotov/rgs/internal/audit"
	"github.com/alexbotov/rgs/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrLimitNotFound     = errors.New("limit not found")
	ErrPlayerExcluded    = errors.New("player is self-excluded")
	ErrCoolingOffPending = errors.New("limit increase pending cooling-off period")
	ErrInvalidLimit      = errors.New("invalid limit value")
)

// CoolingOffPeriod is the required waiting period for limit increases
// GLI-19 §2.5.5.b - Limit increases require waiting period
const CoolingOffPeriod = 24 * time.Hour

// Service provides player limit management
type Service struct {
	db       *sql.DB
	audit    *audit.Service
	currency string
}

// New creates a new limits service
func New(db *sql.DB, auditSvc *audit.Service, currency string) *Service {
	return &Service{
		db:       db,
		audit:    auditSvc,
		currency: currency,
	}
}

// GetLimits retrieves a player's current limits
// GLI-19 §2.5.5 - Player must be able to view their limits
func (s *Service) GetLimits(ctx context.Context, playerID string) (*domain.PlayerLimits, error) {
	var limits domain.PlayerLimits
	var dailyDep, weeklyDep, monthlyDep sql.NullInt64
	var dailyWager, weeklyWager sql.NullInt64
	var dailyLoss, weeklyLoss sql.NullInt64
	var sessionDur sql.NullInt64
	var coolingOff sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, player_id, daily_deposit, weekly_deposit, monthly_deposit,
		       daily_wager, weekly_wager, daily_loss, weekly_loss,
		       session_duration, cooling_off_until, source, effective_at, updated_at
		FROM player_limits WHERE player_id = $1
	`, playerID).Scan(
		&limits.ID, &limits.PlayerID,
		&dailyDep, &weeklyDep, &monthlyDep,
		&dailyWager, &weeklyWager,
		&dailyLoss, &weeklyLoss,
		&sessionDur, &coolingOff,
		&limits.Source, &limits.EffectiveAt, &limits.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return empty limits if none set
			return &domain.PlayerLimits{
				PlayerID:    playerID,
				Source:      domain.LimitSourcePlayer,
				EffectiveAt: time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}, nil
		}
		return nil, fmt.Errorf("failed to get limits: %w", err)
	}

	// Convert nullable values to Money pointers
	if dailyDep.Valid {
		m := domain.Money{Amount: dailyDep.Int64, Currency: s.currency}
		limits.DailyDeposit = &m
	}
	if weeklyDep.Valid {
		m := domain.Money{Amount: weeklyDep.Int64, Currency: s.currency}
		limits.WeeklyDeposit = &m
	}
	if monthlyDep.Valid {
		m := domain.Money{Amount: monthlyDep.Int64, Currency: s.currency}
		limits.MonthlyDeposit = &m
	}
	if dailyWager.Valid {
		m := domain.Money{Amount: dailyWager.Int64, Currency: s.currency}
		limits.DailyWager = &m
	}
	if weeklyWager.Valid {
		m := domain.Money{Amount: weeklyWager.Int64, Currency: s.currency}
		limits.WeeklyWager = &m
	}
	if dailyLoss.Valid {
		m := domain.Money{Amount: dailyLoss.Int64, Currency: s.currency}
		limits.DailyLoss = &m
	}
	if weeklyLoss.Valid {
		m := domain.Money{Amount: weeklyLoss.Int64, Currency: s.currency}
		limits.WeeklyLoss = &m
	}
	if sessionDur.Valid {
		limits.SessionDuration = &sessionDur.Int64
	}
	if coolingOff.Valid {
		limits.CoolingOffUntil = &coolingOff.Time
	}

	return &limits, nil
}

// SetDepositLimitRequest contains deposit limit update data
type SetDepositLimitRequest struct {
	PlayerID string `json:"player_id"`
	Period   string `json:"period"` // daily, weekly, monthly
	Amount   int64  `json:"amount"` // in cents, 0 to remove limit
}

// SetDepositLimit updates a player's deposit limit
// GLI-19 §2.5.5.a - Deposit limits must be supported
// GLI-19 §2.5.5.b - Decreases immediate, increases require cooling-off
func (s *Service) SetDepositLimit(ctx context.Context, req *SetDepositLimitRequest) (*domain.PlayerLimits, error) {
	if req.Amount < 0 {
		return nil, ErrInvalidLimit
	}

	currentLimits, err := s.GetLimits(ctx, req.PlayerID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	effectiveAt := now

	// Check if this is an increase (requires cooling-off period)
	var currentAmount int64
	switch req.Period {
	case "daily":
		if currentLimits.DailyDeposit != nil {
			currentAmount = currentLimits.DailyDeposit.Amount
		}
	case "weekly":
		if currentLimits.WeeklyDeposit != nil {
			currentAmount = currentLimits.WeeklyDeposit.Amount
		}
	case "monthly":
		if currentLimits.MonthlyDeposit != nil {
			currentAmount = currentLimits.MonthlyDeposit.Amount
		}
	default:
		return nil, fmt.Errorf("invalid period: %s", req.Period)
	}

	// If increasing or removing limit, apply cooling-off period
	if req.Amount > currentAmount || (req.Amount == 0 && currentAmount > 0) {
		effectiveAt = now.Add(CoolingOffPeriod)
	}

	// Upsert limit
	err = s.upsertLimit(ctx, req.PlayerID, req.Period+"_deposit", req.Amount, effectiveAt)
	if err != nil {
		return nil, err
	}

	// Audit log
	s.audit.Log(ctx, "limit_change", domain.SeverityInfo,
		fmt.Sprintf("Deposit limit changed: %s = %d cents (effective: %s)", req.Period, req.Amount, effectiveAt.Format(time.RFC3339)),
		map[string]interface{}{
			"period":       req.Period,
			"amount":       req.Amount,
			"effective_at": effectiveAt,
			"immediate":    effectiveAt.Equal(now),
		},
		audit.WithPlayer(req.PlayerID))

	return s.GetLimits(ctx, req.PlayerID)
}

// SetWagerLimitRequest contains wager limit update data
type SetWagerLimitRequest struct {
	PlayerID string `json:"player_id"`
	Period   string `json:"period"` // daily, weekly
	Amount   int64  `json:"amount"` // in cents
}

// SetWagerLimit updates a player's wager limit
// GLI-19 §2.5.5.a - Wager limits must be supported
func (s *Service) SetWagerLimit(ctx context.Context, req *SetWagerLimitRequest) (*domain.PlayerLimits, error) {
	if req.Amount < 0 {
		return nil, ErrInvalidLimit
	}

	currentLimits, err := s.GetLimits(ctx, req.PlayerID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	effectiveAt := now

	var currentAmount int64
	switch req.Period {
	case "daily":
		if currentLimits.DailyWager != nil {
			currentAmount = currentLimits.DailyWager.Amount
		}
	case "weekly":
		if currentLimits.WeeklyWager != nil {
			currentAmount = currentLimits.WeeklyWager.Amount
		}
	default:
		return nil, fmt.Errorf("invalid period: %s", req.Period)
	}

	if req.Amount > currentAmount || (req.Amount == 0 && currentAmount > 0) {
		effectiveAt = now.Add(CoolingOffPeriod)
	}

	err = s.upsertLimit(ctx, req.PlayerID, req.Period+"_wager", req.Amount, effectiveAt)
	if err != nil {
		return nil, err
	}

	s.audit.Log(ctx, "limit_change", domain.SeverityInfo,
		fmt.Sprintf("Wager limit changed: %s = %d cents", req.Period, req.Amount),
		map[string]interface{}{"period": req.Period, "amount": req.Amount},
		audit.WithPlayer(req.PlayerID))

	return s.GetLimits(ctx, req.PlayerID)
}

// SetLossLimitRequest contains loss limit update data
type SetLossLimitRequest struct {
	PlayerID string `json:"player_id"`
	Period   string `json:"period"` // daily, weekly
	Amount   int64  `json:"amount"` // in cents
}

// SetLossLimit updates a player's loss limit
// GLI-19 §2.5.5.a - Loss limits must be supported
func (s *Service) SetLossLimit(ctx context.Context, req *SetLossLimitRequest) (*domain.PlayerLimits, error) {
	if req.Amount < 0 {
		return nil, ErrInvalidLimit
	}

	currentLimits, err := s.GetLimits(ctx, req.PlayerID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	effectiveAt := now

	var currentAmount int64
	switch req.Period {
	case "daily":
		if currentLimits.DailyLoss != nil {
			currentAmount = currentLimits.DailyLoss.Amount
		}
	case "weekly":
		if currentLimits.WeeklyLoss != nil {
			currentAmount = currentLimits.WeeklyLoss.Amount
		}
	default:
		return nil, fmt.Errorf("invalid period: %s", req.Period)
	}

	if req.Amount > currentAmount || (req.Amount == 0 && currentAmount > 0) {
		effectiveAt = now.Add(CoolingOffPeriod)
	}

	err = s.upsertLimit(ctx, req.PlayerID, req.Period+"_loss", req.Amount, effectiveAt)
	if err != nil {
		return nil, err
	}

	s.audit.Log(ctx, "limit_change", domain.SeverityInfo,
		fmt.Sprintf("Loss limit changed: %s = %d cents", req.Period, req.Amount),
		map[string]interface{}{"period": req.Period, "amount": req.Amount},
		audit.WithPlayer(req.PlayerID))

	return s.GetLimits(ctx, req.PlayerID)
}

// SelfExclude excludes a player from gaming
// GLI-19 §2.5.5.c - Self-exclusion must be supported
func (s *Service) SelfExclude(ctx context.Context, playerID, reason string, duration *time.Duration) (*domain.SelfExclusion, error) {
	now := time.Now().UTC()

	exclusion := &domain.SelfExclusion{
		ID:        uuid.New().String(),
		PlayerID:  playerID,
		Reason:    reason,
		StartedAt: now,
		IsActive:  true,
		CreatedAt: now,
	}

	if duration != nil {
		expiresAt := now.Add(*duration)
		exclusion.ExpiresAt = &expiresAt
	}

	// Insert exclusion record
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO self_exclusions (id, player_id, reason, started_at, expires_at, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, exclusion.ID, exclusion.PlayerID, exclusion.Reason, exclusion.StartedAt,
		exclusion.ExpiresAt, exclusion.IsActive, exclusion.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create self-exclusion: %w", err)
	}

	// Update player status
	_, err = s.db.ExecContext(ctx, `
		UPDATE players SET status = $1, updated_at = $2 WHERE id = $3
	`, domain.PlayerStatusExcluded, now, playerID)
	if err != nil {
		return nil, fmt.Errorf("failed to update player status: %w", err)
	}

	// Audit log - GLI-19 §2.8.8 significant event
	s.audit.Log(ctx, "self_exclusion", domain.SeverityCritical,
		fmt.Sprintf("Player self-excluded: %s", reason),
		map[string]interface{}{
			"exclusion_id": exclusion.ID,
			"expires_at":   exclusion.ExpiresAt,
			"permanent":    exclusion.ExpiresAt == nil,
		},
		audit.WithPlayer(playerID))

	return exclusion, nil
}

// IsExcluded checks if a player is currently self-excluded
// GLI-19 §2.5.5.c - Excluded players cannot access gaming
func (s *Service) IsExcluded(ctx context.Context, playerID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM self_exclusions 
		WHERE player_id = $1 AND is_active = true 
		AND (expires_at IS NULL OR expires_at > $2)
	`, playerID, time.Now().UTC()).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CheckDepositLimit checks if a deposit would exceed limits
// GLI-19 §2.5.5 - Limits must be enforced
func (s *Service) CheckDepositLimit(ctx context.Context, playerID string, amount domain.Money) error {
	limits, err := s.GetLimits(ctx, playerID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	// Get deposits in current periods
	dailyTotal, err := s.getDepositTotal(ctx, playerID, now.Add(-24*time.Hour), now)
	if err != nil {
		return err
	}
	weeklyTotal, err := s.getDepositTotal(ctx, playerID, now.Add(-7*24*time.Hour), now)
	if err != nil {
		return err
	}
	monthlyTotal, err := s.getDepositTotal(ctx, playerID, now.Add(-30*24*time.Hour), now)
	if err != nil {
		return err
	}

	// Check against limits
	if limits.DailyDeposit != nil && limits.EffectiveAt.Before(now) {
		if dailyTotal+amount.Amount > limits.DailyDeposit.Amount {
			return fmt.Errorf("daily deposit limit exceeded")
		}
	}
	if limits.WeeklyDeposit != nil && limits.EffectiveAt.Before(now) {
		if weeklyTotal+amount.Amount > limits.WeeklyDeposit.Amount {
			return fmt.Errorf("weekly deposit limit exceeded")
		}
	}
	if limits.MonthlyDeposit != nil && limits.EffectiveAt.Before(now) {
		if monthlyTotal+amount.Amount > limits.MonthlyDeposit.Amount {
			return fmt.Errorf("monthly deposit limit exceeded")
		}
	}

	return nil
}

// CheckWagerLimit checks if a wager would exceed limits
// GLI-19 §2.5.5 - Limits must be enforced
func (s *Service) CheckWagerLimit(ctx context.Context, playerID string, amount domain.Money) error {
	limits, err := s.GetLimits(ctx, playerID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	dailyTotal, err := s.getWagerTotal(ctx, playerID, now.Add(-24*time.Hour), now)
	if err != nil {
		return err
	}
	weeklyTotal, err := s.getWagerTotal(ctx, playerID, now.Add(-7*24*time.Hour), now)
	if err != nil {
		return err
	}

	if limits.DailyWager != nil && limits.EffectiveAt.Before(now) {
		if dailyTotal+amount.Amount > limits.DailyWager.Amount {
			return fmt.Errorf("daily wager limit exceeded")
		}
	}
	if limits.WeeklyWager != nil && limits.EffectiveAt.Before(now) {
		if weeklyTotal+amount.Amount > limits.WeeklyWager.Amount {
			return fmt.Errorf("weekly wager limit exceeded")
		}
	}

	return nil
}

// upsertLimit inserts or updates a specific limit value
func (s *Service) upsertLimit(ctx context.Context, playerID, limitType string, amount int64, effectiveAt time.Time) error {
	now := time.Now().UTC()

	// Check if limits record exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM player_limits WHERE player_id = $1)", playerID).Scan(&exists)
	if err != nil {
		return err
	}

	if !exists {
		// Create new record
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO player_limits (id, player_id, source, effective_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.New().String(), playerID, domain.LimitSourcePlayer, effectiveAt, now)
		if err != nil {
			return err
		}
	}

	// Update specific limit column
	var query string
	var nullableAmount interface{}
	if amount == 0 {
		nullableAmount = nil
	} else {
		nullableAmount = amount
	}

	switch limitType {
	case "daily_deposit":
		query = "UPDATE player_limits SET daily_deposit = $1, effective_at = $2, updated_at = $3 WHERE player_id = $4"
	case "weekly_deposit":
		query = "UPDATE player_limits SET weekly_deposit = $1, effective_at = $2, updated_at = $3 WHERE player_id = $4"
	case "monthly_deposit":
		query = "UPDATE player_limits SET monthly_deposit = $1, effective_at = $2, updated_at = $3 WHERE player_id = $4"
	case "daily_wager":
		query = "UPDATE player_limits SET daily_wager = $1, effective_at = $2, updated_at = $3 WHERE player_id = $4"
	case "weekly_wager":
		query = "UPDATE player_limits SET weekly_wager = $1, effective_at = $2, updated_at = $3 WHERE player_id = $4"
	case "daily_loss":
		query = "UPDATE player_limits SET daily_loss = $1, effective_at = $2, updated_at = $3 WHERE player_id = $4"
	case "weekly_loss":
		query = "UPDATE player_limits SET weekly_loss = $1, effective_at = $2, updated_at = $3 WHERE player_id = $4"
	default:
		return fmt.Errorf("unknown limit type: %s", limitType)
	}

	_, err = s.db.ExecContext(ctx, query, nullableAmount, effectiveAt, now, playerID)
	return err
}

// getDepositTotal calculates total deposits in a time period
func (s *Service) getDepositTotal(ctx context.Context, playerID string, from, to time.Time) (int64, error) {
	var total sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM transactions 
		WHERE player_id = $1 AND type = 'deposit' AND status = 'completed'
		AND created_at >= $2 AND created_at <= $3
	`, playerID, from, to).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Int64, nil
}

// getWagerTotal calculates total wagers in a time period
func (s *Service) getWagerTotal(ctx context.Context, playerID string, from, to time.Time) (int64, error) {
	var total sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM transactions 
		WHERE player_id = $1 AND type = 'wager' AND status = 'completed'
		AND created_at >= $2 AND created_at <= $3
	`, playerID, from, to).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Int64, nil
}
