// Package database provides database access for the RGS
package database

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// DB wraps the SQL database connection
type DB struct {
	*sql.DB
}

// New creates a new database connection
func New(driver, dsn string) (*DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

// Migrate creates all required tables
// Based on GLI-19 §2.8 Information to be Maintained
func (db *DB) Migrate() error {
	schema := `
	-- Players table (GLI-19 §2.5, §2.8.5)
	CREATE TABLE IF NOT EXISTS players (
		id UUID PRIMARY KEY,
		username VARCHAR(255) UNIQUE NOT NULL,
		email VARCHAR(255) UNIQUE NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		status VARCHAR(50) NOT NULL DEFAULT 'active',
		registration_date TIMESTAMP NOT NULL,
		last_login_at TIMESTAMP,
		tc_accepted_at TIMESTAMP NOT NULL,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	-- Sessions table (GLI-19 §2.5.3)
	CREATE TABLE IF NOT EXISTS sessions (
		id UUID PRIMARY KEY,
		player_id UUID NOT NULL REFERENCES players(id),
		token TEXT NOT NULL,
		ip_address VARCHAR(45) NOT NULL,
		user_agent TEXT,
		created_at TIMESTAMP NOT NULL,
		last_activity_at TIMESTAMP NOT NULL,
		expires_at TIMESTAMP NOT NULL,
		status VARCHAR(50) NOT NULL DEFAULT 'active'
	);

	-- Balances table (GLI-19 §2.5.7)
	CREATE TABLE IF NOT EXISTS balances (
		player_id UUID PRIMARY KEY REFERENCES players(id),
		real_money_amount BIGINT NOT NULL DEFAULT 0,
		real_money_currency VARCHAR(3) NOT NULL DEFAULT 'USD',
		bonus_amount BIGINT NOT NULL DEFAULT 0,
		bonus_currency VARCHAR(3) NOT NULL DEFAULT 'USD',
		updated_at TIMESTAMP NOT NULL
	);

	-- Transactions table (GLI-19 §2.5.6, §2.5.7, §2.8.5)
	CREATE TABLE IF NOT EXISTS transactions (
		id UUID PRIMARY KEY,
		player_id UUID NOT NULL REFERENCES players(id),
		type VARCHAR(50) NOT NULL,
		amount BIGINT NOT NULL,
		currency VARCHAR(3) NOT NULL,
		balance_before BIGINT NOT NULL,
		balance_after BIGINT NOT NULL,
		status VARCHAR(50) NOT NULL,
		reference VARCHAR(255),
		description TEXT,
		created_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP
	);

	-- Game Sessions table (GLI-19 §4.3)
	CREATE TABLE IF NOT EXISTS game_sessions (
		id UUID PRIMARY KEY,
		player_id UUID NOT NULL REFERENCES players(id),
		game_id VARCHAR(255) NOT NULL,
		started_at TIMESTAMP NOT NULL,
		ended_at TIMESTAMP,
		last_activity_at TIMESTAMP NOT NULL,
		status VARCHAR(50) NOT NULL DEFAULT 'active',
		opening_balance BIGINT NOT NULL,
		current_balance BIGINT NOT NULL,
		total_wagered BIGINT NOT NULL DEFAULT 0,
		total_won BIGINT NOT NULL DEFAULT 0,
		games_played INTEGER NOT NULL DEFAULT 0,
		currency VARCHAR(3) NOT NULL
	);

	-- Game Cycles table (GLI-19 §4.3.3, §2.8.2)
	CREATE TABLE IF NOT EXISTS game_cycles (
		id UUID PRIMARY KEY,
		session_id UUID NOT NULL REFERENCES game_sessions(id),
		player_id UUID NOT NULL REFERENCES players(id),
		game_id VARCHAR(255) NOT NULL,
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP,
		wager_amount BIGINT NOT NULL,
		win_amount BIGINT NOT NULL DEFAULT 0,
		balance_before BIGINT NOT NULL,
		balance_after BIGINT NOT NULL,
		outcome JSONB,
		status VARCHAR(50) NOT NULL DEFAULT 'pending',
		currency VARCHAR(3) NOT NULL
	);

	-- Audit Events table (GLI-19 §2.8.8)
	CREATE TABLE IF NOT EXISTS audit_events (
		id UUID PRIMARY KEY,
		type VARCHAR(100) NOT NULL,
		severity VARCHAR(20) NOT NULL,
		timestamp TIMESTAMP NOT NULL,
		player_id UUID,
		session_id UUID,
		description TEXT NOT NULL,
		data JSONB,
		ip_address VARCHAR(45),
		component VARCHAR(100) NOT NULL
	);

	-- Failed Login Attempts table (GLI-19 §2.8.8)
	CREATE TABLE IF NOT EXISTS failed_logins (
		id UUID PRIMARY KEY,
		username VARCHAR(255) NOT NULL,
		ip_address VARCHAR(45) NOT NULL,
		attempted_at TIMESTAMP NOT NULL
	);

	-- Player Limits table (GLI-19 §2.5.5)
	CREATE TABLE IF NOT EXISTS player_limits (
		id UUID PRIMARY KEY,
		player_id UUID NOT NULL REFERENCES players(id),
		daily_deposit BIGINT,
		weekly_deposit BIGINT,
		monthly_deposit BIGINT,
		daily_wager BIGINT,
		weekly_wager BIGINT,
		daily_loss BIGINT,
		weekly_loss BIGINT,
		session_duration INTEGER,
		cooling_off_until TIMESTAMP,
		source VARCHAR(50) NOT NULL DEFAULT 'player',
		effective_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE(player_id)
	);

	-- Self Exclusions table (GLI-19 §2.5.5.c)
	CREATE TABLE IF NOT EXISTS self_exclusions (
		id UUID PRIMARY KEY,
		player_id UUID NOT NULL REFERENCES players(id),
		reason TEXT NOT NULL,
		started_at TIMESTAMP NOT NULL,
		expires_at TIMESTAMP,
		removed_at TIMESTAMP,
		removed_by VARCHAR(255),
		is_active BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMP NOT NULL
	);

	-- System State table (GLI-19 §2.4)
	CREATE TABLE IF NOT EXISTS system_state (
		key VARCHAR(100) PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		updated_by VARCHAR(255)
	);

	-- Disabled Games table (GLI-19 §2.4)
	CREATE TABLE IF NOT EXISTS disabled_games (
		game_id VARCHAR(255) PRIMARY KEY,
		reason TEXT NOT NULL,
		disabled_at TIMESTAMP NOT NULL,
		disabled_by VARCHAR(255) NOT NULL
	);

	-- Indexes for performance
	CREATE INDEX IF NOT EXISTS idx_sessions_player ON sessions(player_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
	CREATE INDEX IF NOT EXISTS idx_transactions_player ON transactions(player_id);
	CREATE INDEX IF NOT EXISTS idx_transactions_created ON transactions(created_at);
	CREATE INDEX IF NOT EXISTS idx_game_sessions_player ON game_sessions(player_id);
	CREATE INDEX IF NOT EXISTS idx_game_cycles_session ON game_cycles(session_id);
	CREATE INDEX IF NOT EXISTS idx_game_cycles_player ON game_cycles(player_id);
	CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp ON audit_events(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_events_player ON audit_events(player_id);
	CREATE INDEX IF NOT EXISTS idx_player_limits_player ON player_limits(player_id);
	CREATE INDEX IF NOT EXISTS idx_self_exclusions_player ON self_exclusions(player_id);
	CREATE INDEX IF NOT EXISTS idx_self_exclusions_active ON self_exclusions(is_active);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// Reset drops all tables (for testing)
func (db *DB) Reset() error {
	_, err := db.Exec(`
		DROP TABLE IF EXISTS disabled_games CASCADE;
		DROP TABLE IF EXISTS system_state CASCADE;
		DROP TABLE IF EXISTS self_exclusions CASCADE;
		DROP TABLE IF EXISTS player_limits CASCADE;
		DROP TABLE IF EXISTS failed_logins CASCADE;
		DROP TABLE IF EXISTS audit_events CASCADE;
		DROP TABLE IF EXISTS game_cycles CASCADE;
		DROP TABLE IF EXISTS game_sessions CASCADE;
		DROP TABLE IF EXISTS transactions CASCADE;
		DROP TABLE IF EXISTS balances CASCADE;
		DROP TABLE IF EXISTS sessions CASCADE;
		DROP TABLE IF EXISTS players CASCADE;
	`)
	return err
}

// CleanData truncates all tables without dropping them (for testing)
func (db *DB) CleanData() error {
	_, err := db.Exec(`
		TRUNCATE TABLE disabled_games, system_state, self_exclusions, player_limits,
		               failed_logins, audit_events, game_cycles, game_sessions, 
		               transactions, balances, sessions, players CASCADE;
	`)
	return err
}
