// Package pateplay provides a client for the Pateplay RGS Wallet API
// Based on the Pateplay games API specification
package pateplay

import "time"

// Error codes returned by the Pateplay API
const (
	ErrUnexpectedError          = "UNEXPECTED_ERROR"
	ErrNotAuthorized            = "NOT_AUTHORIZED"
	ErrInvalidAuthToken         = "INVALID_AUTH_TOKEN"
	ErrInvalidSessionToken      = "INVALID_SESSION_TOKEN"
	ErrInsufficientBalance      = "INSUFFICIENT_BALANCE"
	ErrTransactionNotFound      = "TRANSACTION_NOT_FOUND"
	ErrTransactionAlreadyExists = "TRANSACTION_ALREADY_EXISTS"
	ErrRealityCheck             = "REALITY_CHECK"
	ErrBetLimitReached          = "BET_LIMIT_REACHED"
	ErrBetLimit90Percent        = "BET_LIMIT_90_PERCENT"
	ErrLossLimitReached         = "LOSS_LIMIT_REACHED"
	ErrLossLimit90Percent       = "LOSS_LIMIT_90_PERCENT"
	ErrTimeLimitReached         = "TIME_LIMIT_REACHED"
	ErrTimeLimit90Percent       = "TIME_LIMIT_90_PERCENT"
)

// WithdrawReason represents the reason for a withdraw transaction
type WithdrawReason string

const (
	WithdrawReasonRoundStart    WithdrawReason = "round_start"
	WithdrawReasonRoundContinue WithdrawReason = "round_continue"
)

// DepositReason represents the reason for a deposit transaction
type DepositReason string

const (
	DepositReasonRoundEnd      DepositReason = "round_end"
	DepositReasonRoundContinue DepositReason = "round_continue"
)

// DeviceType represents the device type for the session
type DeviceType string

const (
	DeviceTypeDesktop DeviceType = "desktop"
	DeviceTypeMobile  DeviceType = "mobile"
)

// APIError represents an error response from the API
type APIError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}

// Response wraps the API response with either result or error
type Response[T any] struct {
	Result *T        `json:"result,omitempty"`
	Error  *APIError `json:"error,omitempty"`
}

// AuthenticateRequest is the request body for /authenticate
type AuthenticateRequest struct {
	AuthToken  string     `json:"authToken"`
	SiteCode   string     `json:"siteCode"`
	DeviceType DeviceType `json:"deviceType"`
}

// AuthenticateResult is the result of a successful authentication
type AuthenticateResult struct {
	SessionToken string `json:"sessionToken"`
	PlayerID     string `json:"playerId"`
	PlayerName   string `json:"playerName"`
	Currency     string `json:"currency"`
	Country      string `json:"country"`
	Balance      string `json:"balance"`
}

// BalanceRequest is the request body for /balance
type BalanceRequest struct {
	SessionToken string `json:"sessionToken"`
	SiteCode     string `json:"siteCode"`
	PlayerID     string `json:"playerId"`
}

// BalanceResult is the result of a balance query
type BalanceResult struct {
	Balance string `json:"balance"`
}

// InitGameRequest is the request body for /init-game
type InitGameRequest struct {
	SessionToken string `json:"sessionToken"`
	SiteCode     string `json:"siteCode"`
	PlayerID     string `json:"playerId"`
	GameName     string `json:"gameName"`
}

// InitGameResult is the result of initializing a game
type InitGameResult struct {
	SessionToken string `json:"sessionToken"`
	Balance      string `json:"balance"`
}

// WithdrawRequest is the request body for /withdraw
type WithdrawRequest struct {
	SessionToken        string         `json:"sessionToken"`
	SiteCode            string         `json:"siteCode"`
	PlayerID            string         `json:"playerId"`
	Currency            string         `json:"currency"`
	RGSRoundID          string         `json:"rgsRoundId"`
	RGSTransactionID    string         `json:"rgsTransactionId"`
	GameName            string         `json:"gameName"`
	Amount              string         `json:"amount"`
	JackpotContribution string         `json:"jackpotContribution"`
	Reason              WithdrawReason `json:"reason"`
}

// WithdrawResult is the result of a withdraw operation
type WithdrawResult struct {
	TransactionID string `json:"transactionId"`
	Balance       string `json:"balance"`
}

// DepositRequest is the request body for /deposit
type DepositRequest struct {
	SessionToken     string        `json:"sessionToken"`
	SiteCode         string        `json:"siteCode"`
	PlayerID         string        `json:"playerId"`
	GameName         string        `json:"gameName"`
	Currency         string        `json:"currency"`
	RGSRoundID       string        `json:"rgsRoundId"`
	RGSTransactionID string        `json:"rgsTransactionId"`
	Amount           string        `json:"amount"`
	IsJackpotWin     bool          `json:"isJackpotWin"`
	Reason           DepositReason `json:"reason"`
}

// DepositResult is the result of a deposit operation
type DepositResult struct {
	TransactionID string `json:"transactionId"`
	Balance       string `json:"balance"`
}

// WithdrawAndDepositRequest is the request body for /withdraw-and-deposit
type WithdrawAndDepositRequest struct {
	SessionToken            string         `json:"sessionToken"`
	SiteCode                string         `json:"siteCode"`
	PlayerID                string         `json:"playerId"`
	GameName                string         `json:"gameName"`
	Currency                string         `json:"currency"`
	RGSRoundID              string         `json:"rgsRoundId"`
	RGSWithdrawTransactionID string        `json:"rgsWithdrawTransactionId"`
	RGSDepositTransactionID  string        `json:"rgsDepositTransactionId"`
	WithdrawAmount          string         `json:"withdrawAmount"`
	DepositAmount           string         `json:"depositAmount"`
	JackpotContribution     string         `json:"jackpotContribution"`
	WithdrawReason          WithdrawReason `json:"withdrawReason"`
	DepositReason           DepositReason  `json:"depositReason"`
}

// WithdrawAndDepositResult is the result of a combined withdraw and deposit
type WithdrawAndDepositResult struct {
	Balance              string `json:"balance"`
	WithdrawTransactionID string `json:"withdrawTransactionId"`
	DepositTransactionID  string `json:"depositTransactionId"`
}

// CancelRequest is the request body for /cancel
type CancelRequest struct {
	SessionToken     string `json:"sessionToken"`
	SiteCode         string `json:"siteCode"`
	PlayerID         string `json:"playerId"`
	RGSRoundID       string `json:"rgsRoundId"`
	RGSTransactionID string `json:"rgsTransactionId"`
}

// CancelResult is the result of a cancel operation
type CancelResult struct {
	TransactionID string `json:"transactionId"`
}

// CreateAuthTokenRequest is the request body for /auth-token (debug only)
type CreateAuthTokenRequest struct {
	SiteCode   string `json:"siteCode"`
	PlayerID   string `json:"playerId,omitempty"`
	PlayerName string `json:"playerName,omitempty"`
	Currency   string `json:"currency,omitempty"`
	Balance    string `json:"balance,omitempty"`
	Country    string `json:"country,omitempty"`
}

// CreateAuthTokenResult is the result of creating an auth token
type CreateAuthTokenResult struct {
	PlayerID   string `json:"playerId"`
	PlayerName string `json:"playerName"`
	Currency   string `json:"currency"`
	Country    string `json:"country"`
	Balance    string `json:"balance"`
	AuthToken  string `json:"authToken"`
}

// ClientConfig holds the configuration for the Pateplay client
type ClientConfig struct {
	BaseURL    string
	APIKey     string
	APISecret  string
	SiteCode   string
	Timeout    time.Duration
	RetryCount int
}

// DefaultConfig returns a default client configuration
func DefaultConfig() *ClientConfig {
	return &ClientConfig{
		Timeout:    30 * time.Second,
		RetryCount: 3,
	}
}

