// Package wallet provides balance and transaction management
// Compliant with GLI-19 §2.5.6: Financial Transactions, §2.5.7: Transaction Log
package wallet

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
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("invalid amount")
	ErrPlayerNotFound    = errors.New("player not found")
)

// Service provides wallet functionality
type Service struct {
	db       *sql.DB
	audit    *audit.Service
	currency string
}

// New creates a new wallet service
func New(db *sql.DB, auditSvc *audit.Service, currency string) *Service {
	return &Service{
		db:       db,
		audit:    auditSvc,
		currency: currency,
	}
}

// GetBalance retrieves the current balance for a player (GLI-19 §2.5.7)
func (s *Service) GetBalance(ctx context.Context, playerID string) (*domain.Balance, error) {
	var realAmount, bonusAmount int64
	var realCurrency, bonusCurrency string
	var updatedAt time.Time

	err := s.db.QueryRowContext(ctx, `
		SELECT real_money_amount, real_money_currency, bonus_amount, bonus_currency, updated_at
		FROM balances WHERE player_id = $1
	`, playerID).Scan(&realAmount, &realCurrency, &bonusAmount, &bonusCurrency, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPlayerNotFound
		}
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	realMoney := domain.Money{Amount: realAmount, Currency: realCurrency}
	bonus := domain.Money{Amount: bonusAmount, Currency: bonusCurrency}

	return &domain.Balance{
		PlayerID:     playerID,
		RealMoney:    realMoney,
		BonusBalance: bonus,
		Available:    realMoney.Add(bonus),
		Currency:     realCurrency,
		UpdatedAt:    updatedAt,
	}, nil
}

// Deposit adds funds to a player's account (GLI-19 §2.5.6)
func (s *Service) Deposit(ctx context.Context, playerID string, amount domain.Money, reference string) (*domain.Transaction, error) {
	if amount.Amount <= 0 {
		return nil, ErrInvalidAmount
	}

	// Get current balance
	balance, err := s.GetBalance(ctx, playerID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	newBalance := balance.RealMoney.Add(amount)

	// Create transaction record
	tx := &domain.Transaction{
		ID:            uuid.New().String(),
		PlayerID:      playerID,
		Type:          domain.TxTypeDeposit,
		Amount:        amount,
		BalanceBefore: balance.RealMoney,
		BalanceAfter:  newBalance,
		Status:        domain.TxStatusCompleted,
		Reference:     reference,
		Description:   "Deposit",
		CreatedAt:     now,
		CompletedAt:   &now,
	}

	// Update balance and record transaction atomically
	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer dbTx.Rollback()

	_, err = dbTx.ExecContext(ctx, `
		UPDATE balances SET real_money_amount = $1, updated_at = $2 WHERE player_id = $3
	`, newBalance.Amount, now, playerID)
	if err != nil {
		return nil, err
	}

	_, err = dbTx.ExecContext(ctx, `
		INSERT INTO transactions (id, player_id, type, amount, currency, balance_before, balance_after, status, reference, description, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, tx.ID, tx.PlayerID, tx.Type, tx.Amount.Amount, tx.Amount.Currency,
		tx.BalanceBefore.Amount, tx.BalanceAfter.Amount, tx.Status, tx.Reference, tx.Description, tx.CreatedAt, tx.CompletedAt)
	if err != nil {
		return nil, err
	}

	if err := dbTx.Commit(); err != nil {
		return nil, err
	}

	// Audit log
	s.audit.Log(ctx, audit.EventDeposit, domain.SeverityInfo,
		fmt.Sprintf("Deposit of %.2f %s", amount.Float64(), amount.Currency),
		map[string]interface{}{
			"transaction_id": tx.ID,
			"amount":         amount.Float64(),
			"currency":       amount.Currency,
		},
		audit.WithPlayer(playerID))

	return tx, nil
}

// Withdraw removes funds from a player's account (GLI-19 §2.5.6)
func (s *Service) Withdraw(ctx context.Context, playerID string, amount domain.Money, reference string) (*domain.Transaction, error) {
	if amount.Amount <= 0 {
		return nil, ErrInvalidAmount
	}

	// Get current balance
	balance, err := s.GetBalance(ctx, playerID)
	if err != nil {
		return nil, err
	}

	// Check sufficient funds (GLI-19 §2.5.6 - no negative balance)
	if balance.RealMoney.Amount < amount.Amount {
		return nil, ErrInsufficientFunds
	}

	now := time.Now().UTC()
	newBalance := balance.RealMoney.Sub(amount)

	// Create transaction record
	tx := &domain.Transaction{
		ID:            uuid.New().String(),
		PlayerID:      playerID,
		Type:          domain.TxTypeWithdrawal,
		Amount:        amount,
		BalanceBefore: balance.RealMoney,
		BalanceAfter:  newBalance,
		Status:        domain.TxStatusCompleted,
		Reference:     reference,
		Description:   "Withdrawal",
		CreatedAt:     now,
		CompletedAt:   &now,
	}

	// Update balance and record transaction atomically
	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer dbTx.Rollback()

	_, err = dbTx.ExecContext(ctx, `
		UPDATE balances SET real_money_amount = $1, updated_at = $2 WHERE player_id = $3
	`, newBalance.Amount, now, playerID)
	if err != nil {
		return nil, err
	}

	_, err = dbTx.ExecContext(ctx, `
		INSERT INTO transactions (id, player_id, type, amount, currency, balance_before, balance_after, status, reference, description, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, tx.ID, tx.PlayerID, tx.Type, tx.Amount.Amount, tx.Amount.Currency,
		tx.BalanceBefore.Amount, tx.BalanceAfter.Amount, tx.Status, tx.Reference, tx.Description, tx.CreatedAt, tx.CompletedAt)
	if err != nil {
		return nil, err
	}

	if err := dbTx.Commit(); err != nil {
		return nil, err
	}

	// Audit log
	s.audit.Log(ctx, audit.EventWithdrawal, domain.SeverityInfo,
		fmt.Sprintf("Withdrawal of %.2f %s", amount.Float64(), amount.Currency),
		map[string]interface{}{
			"transaction_id": tx.ID,
			"amount":         amount.Float64(),
			"currency":       amount.Currency,
		},
		audit.WithPlayer(playerID))

	return tx, nil
}

// PlaceWager deducts wager amount for a game (GLI-19 §4.3.3)
func (s *Service) PlaceWager(ctx context.Context, playerID string, amount domain.Money, gameID, cycleID string) (*domain.Transaction, error) {
	if amount.Amount <= 0 {
		return nil, ErrInvalidAmount
	}

	// Get current balance
	balance, err := s.GetBalance(ctx, playerID)
	if err != nil {
		return nil, err
	}

	// Check sufficient funds
	if balance.Available.Amount < amount.Amount {
		return nil, ErrInsufficientFunds
	}

	now := time.Now().UTC()
	newBalance := balance.RealMoney.Sub(amount)

	// Create transaction record
	tx := &domain.Transaction{
		ID:            uuid.New().String(),
		PlayerID:      playerID,
		Type:          domain.TxTypeWager,
		Amount:        amount,
		BalanceBefore: balance.RealMoney,
		BalanceAfter:  newBalance,
		Status:        domain.TxStatusCompleted,
		Reference:     cycleID,
		Description:   fmt.Sprintf("Wager on %s", gameID),
		CreatedAt:     now,
		CompletedAt:   &now,
	}

	// Update balance and record transaction
	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer dbTx.Rollback()

	_, err = dbTx.ExecContext(ctx, `
		UPDATE balances SET real_money_amount = $1, updated_at = $2 WHERE player_id = $3
	`, newBalance.Amount, now, playerID)
	if err != nil {
		return nil, err
	}

	_, err = dbTx.ExecContext(ctx, `
		INSERT INTO transactions (id, player_id, type, amount, currency, balance_before, balance_after, status, reference, description, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, tx.ID, tx.PlayerID, tx.Type, tx.Amount.Amount, tx.Amount.Currency,
		tx.BalanceBefore.Amount, tx.BalanceAfter.Amount, tx.Status, tx.Reference, tx.Description, tx.CreatedAt, tx.CompletedAt)
	if err != nil {
		return nil, err
	}

	if err := dbTx.Commit(); err != nil {
		return nil, err
	}

	return tx, nil
}

// CreditWin adds winnings to a player's balance (GLI-19 §4.3.3)
func (s *Service) CreditWin(ctx context.Context, playerID string, amount domain.Money, gameID, cycleID string) (*domain.Transaction, error) {
	if amount.Amount < 0 {
		return nil, ErrInvalidAmount
	}
	if amount.Amount == 0 {
		return nil, nil // No win to credit
	}

	// Get current balance
	balance, err := s.GetBalance(ctx, playerID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	newBalance := balance.RealMoney.Add(amount)

	// Create transaction record
	tx := &domain.Transaction{
		ID:            uuid.New().String(),
		PlayerID:      playerID,
		Type:          domain.TxTypeWin,
		Amount:        amount,
		BalanceBefore: balance.RealMoney,
		BalanceAfter:  newBalance,
		Status:        domain.TxStatusCompleted,
		Reference:     cycleID,
		Description:   fmt.Sprintf("Win on %s", gameID),
		CreatedAt:     now,
		CompletedAt:   &now,
	}

	// Update balance and record transaction
	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer dbTx.Rollback()

	_, err = dbTx.ExecContext(ctx, `
		UPDATE balances SET real_money_amount = $1, updated_at = $2 WHERE player_id = $3
	`, newBalance.Amount, now, playerID)
	if err != nil {
		return nil, err
	}

	_, err = dbTx.ExecContext(ctx, `
		INSERT INTO transactions (id, player_id, type, amount, currency, balance_before, balance_after, status, reference, description, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, tx.ID, tx.PlayerID, tx.Type, tx.Amount.Amount, tx.Amount.Currency,
		tx.BalanceBefore.Amount, tx.BalanceAfter.Amount, tx.Status, tx.Reference, tx.Description, tx.CreatedAt, tx.CompletedAt)
	if err != nil {
		return nil, err
	}

	if err := dbTx.Commit(); err != nil {
		return nil, err
	}

	return tx, nil
}

// GetTransactions retrieves transaction history for a player (GLI-19 §2.5.7)
func (s *Service) GetTransactions(ctx context.Context, playerID string, limit int) ([]*domain.Transaction, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, player_id, type, amount, currency, balance_before, balance_after, status, reference, description, created_at, completed_at
		FROM transactions WHERE player_id = $1 ORDER BY created_at DESC LIMIT $2
	`, playerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var transactions []*domain.Transaction
	for rows.Next() {
		var tx domain.Transaction
		var amount, balBefore, balAfter int64
		var currency, reference, description string
		var completedAt sql.NullTime

		err := rows.Scan(&tx.ID, &tx.PlayerID, &tx.Type, &amount, &currency,
			&balBefore, &balAfter, &tx.Status, &reference, &description,
			&tx.CreatedAt, &completedAt)
		if err != nil {
			return nil, err
		}

		tx.Amount = domain.Money{Amount: amount, Currency: currency}
		tx.BalanceBefore = domain.Money{Amount: balBefore, Currency: currency}
		tx.BalanceAfter = domain.Money{Amount: balAfter, Currency: currency}
		tx.Reference = reference
		tx.Description = description
		if completedAt.Valid {
			tx.CompletedAt = &completedAt.Time
		}

		transactions = append(transactions, &tx)
	}

	return transactions, nil
}
