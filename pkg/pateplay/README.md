# Pateplay Wallet API Client

Go client library for the Pateplay RGS (Remote Game Server) Wallet API.

## Installation

```go
import "github.com/alexbotov/rgs/pkg/pateplay"
```

## Configuration

```go
client := pateplay.NewClient(&pateplay.ClientConfig{
    BaseURL:    "https://your-operator.pateplay.net",
    APIKey:     "your-api-key",
    APISecret:  "your-api-secret",
    SiteCode:   "your-site-code",
    Timeout:    30 * time.Second,
    RetryCount: 3,
})
```

## API Methods

### Authenticate

Create a session from a one-time auth token when a player opens a game.

```go
result, err := client.Authenticate(ctx, "auth-token-123", pateplay.DeviceTypeDesktop)
if err != nil {
    // Handle error
}
fmt.Printf("Session: %s, Player: %s, Balance: %s\n", 
    result.SessionToken, result.PlayerID, result.Balance)
```

### Get Balance

Retrieve the player's current balance.

```go
result, err := client.GetBalance(ctx, sessionToken, playerID)
if err != nil {
    // Handle error
}
fmt.Printf("Balance: %s\n", result.Balance)
```

### Init Game

Start a new game session. May return an updated session token.

```go
result, err := client.InitGame(ctx, sessionToken, playerID, "fortune-slots")
if err != nil {
    // Handle error
}
// Use result.SessionToken for subsequent game calls
```

### Withdraw (Place Bet)

Deduct money from the player's balance when placing a bet.

```go
result, err := client.Withdraw(ctx, &pateplay.WithdrawRequest{
    SessionToken:        sessionToken,
    PlayerID:            playerID,
    Currency:            "USD",
    RGSRoundID:          "round-123",
    RGSTransactionID:    "tx-456",
    GameName:            "fortune-slots",
    Amount:              "10.00",
    JackpotContribution: "0.01",
    Reason:              pateplay.WithdrawReasonRoundStart,
})
if err != nil {
    if apiErr, ok := err.(*pateplay.APIError); ok {
        if apiErr.Code == pateplay.ErrInsufficientBalance {
            // Handle insufficient balance
        }
    }
}
fmt.Printf("Transaction: %s, New Balance: %s\n", result.TransactionID, result.Balance)
```

### Deposit (Credit Win)

Add money to the player's balance when they win.

```go
result, err := client.Deposit(ctx, &pateplay.DepositRequest{
    SessionToken:     sessionToken,
    PlayerID:         playerID,
    GameName:         "fortune-slots",
    Currency:         "USD",
    RGSRoundID:       "round-123",
    RGSTransactionID: "tx-789",
    Amount:           "25.00",
    IsJackpotWin:     false,
    Reason:           pateplay.DepositReasonRoundEnd,
})
```

### Withdraw and Deposit (Combined)

Perform both operations in a single request, typically for no-win rounds.

```go
result, err := client.WithdrawAndDeposit(ctx, &pateplay.WithdrawAndDepositRequest{
    SessionToken:             sessionToken,
    PlayerID:                 playerID,
    GameName:                 "fortune-slots",
    Currency:                 "USD",
    RGSRoundID:               "round-124",
    RGSWithdrawTransactionID: "tx-w-1",
    RGSDepositTransactionID:  "tx-d-1",
    WithdrawAmount:           "1.00",
    DepositAmount:            "0.00",  // No win
    JackpotContribution:      "0.01",
    WithdrawReason:           pateplay.WithdrawReasonRoundStart,
    DepositReason:            pateplay.DepositReasonRoundEnd,
})
```

### Cancel

Cancel a failed transaction.

```go
result, err := client.Cancel(ctx, sessionToken, playerID, rgsRoundID, rgsTransactionID)
if err != nil {
    if apiErr, ok := err.(*pateplay.APIError); ok {
        if apiErr.Code == pateplay.ErrTransactionNotFound {
            // Transaction doesn't exist
        }
    }
}
```

### Create Auth Token (Debug Only)

Create a test player auth token. **Disabled in production.**

```go
result, err := client.CreateAuthToken(ctx, &pateplay.CreateAuthTokenRequest{
    PlayerName: "Test Player",
    Currency:   "USD",
    Balance:    "10000.00",
    Country:    "us",
})
```

## Error Handling

All methods return an `*APIError` when the API returns an error response:

```go
result, err := client.Withdraw(ctx, req)
if err != nil {
    if apiErr, ok := err.(*pateplay.APIError); ok {
        switch apiErr.Code {
        case pateplay.ErrInsufficientBalance:
            // Not enough money
        case pateplay.ErrInvalidSessionToken:
            // Session expired
        case pateplay.ErrTransactionAlreadyExists:
            // Duplicate transaction
            existingTxID := apiErr.Data["transactionId"]
        default:
            // Other error
        }
    }
}
```

## Error Codes

| Code | Description |
|------|-------------|
| `UNEXPECTED_ERROR` | Any unhandled error |
| `NOT_AUTHORIZED` | Invalid API key or HMAC |
| `INVALID_AUTH_TOKEN` | Auth token invalid or already used |
| `INVALID_SESSION_TOKEN` | Session token invalid or expired |
| `INSUFFICIENT_BALANCE` | Not enough money in player's balance |
| `TRANSACTION_NOT_FOUND` | Transaction ID not found |
| `TRANSACTION_ALREADY_EXISTS` | Transaction ID already processed |
| `REALITY_CHECK` | Regulatory reality check (one-time) |
| `BET_LIMIT_REACHED` | Player bet limit reached |
| `LOSS_LIMIT_REACHED` | Player loss limit reached |
| `TIME_LIMIT_REACHED` | Player time limit reached |

## Security

The client automatically:
- Signs all requests with HMAC-SHA256 using the API secret
- Includes the API key in the `x-api-key` header
- Includes the HMAC signature in the `x-api-hmac` header

## Testing

Run the tests:

```bash
go test ./pkg/pateplay/... -v
```

The test suite includes a mock server that validates HMAC signatures and request payloads.

