// Package pateplay provides a client for the Pateplay RGS Wallet API.
//
// The Pateplay API is used for communication between casino operators
// and the Pateplay remote game server (RGS). This client implements
// all the required wallet operations for game integration.
//
// # Authentication
//
// All API requests are authenticated using:
//   - API Key: Sent in the x-api-key header
//   - HMAC Signature: SHA256 hash of the request body, sent in x-api-hmac header
//
// # Basic Usage
//
//	client := pateplay.NewClient(&pateplay.ClientConfig{
//	    BaseURL:   "https://operator.pateplay.net",
//	    APIKey:    "your-api-key",
//	    APISecret: "your-api-secret",
//	    SiteCode:  "your-site",
//	})
//
//	// Authenticate player
//	result, err := client.Authenticate(ctx, authToken, pateplay.DeviceTypeDesktop)
//
//	// Place bet (withdraw)
//	result, err := client.Withdraw(ctx, &pateplay.WithdrawRequest{
//	    SessionToken: session,
//	    PlayerID:     playerID,
//	    Amount:       "10.00",
//	    // ...
//	})
//
// # Error Handling
//
// API errors are returned as *APIError with a Code field indicating the error type:
//
//	result, err := client.Withdraw(ctx, req)
//	if apiErr, ok := err.(*pateplay.APIError); ok {
//	    switch apiErr.Code {
//	    case pateplay.ErrInsufficientBalance:
//	        // Handle insufficient funds
//	    case pateplay.ErrTransactionAlreadyExists:
//	        // Handle duplicate transaction
//	    }
//	}
package pateplay

