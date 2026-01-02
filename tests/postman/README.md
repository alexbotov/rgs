# RGS Postman Test Suite

End-to-end API tests for the Remote Gaming Server, generated from the Go integration tests.

## Files

- `RGS_Collection.postman_collection.json` - Main test collection
- `RGS_Local.postman_environment.json` - Local development environment

## Import into Postman

1. Open Postman
2. Click **Import** (top-left)
3. Drag both JSON files or select them
4. The collection and environment will be imported

## Configuration

1. Select the **RGS Local** environment from the dropdown (top-right)
2. Update `base_url` if your server runs on a different port:
   - Click the environment dropdown → Edit
   - Change `http://localhost:8080` to your server URL

## Running Tests

### Run Entire Collection

1. Click the **RGS - Remote Gaming Server** collection
2. Click **Run** (or right-click → Run collection)
3. Select **Run RGS - Remote Gaming Server**

### Run Specific Folders

Run individual test suites:
- **Health Check** - Basic server health
- **Authentication** - Registration, login, session, logout
- **Wallet** - Balance, deposits, withdrawals, transactions
- **Games** - Game listing, sessions, gameplay, history
- **Complete Player Journey** - Full E2E flow (recommended)

### Run with Newman (CLI)

```bash
# Install Newman
npm install -g newman

# Run collection
newman run RGS_Collection.postman_collection.json \
  -e RGS_Local.postman_environment.json

# Run with HTML report
newman run RGS_Collection.postman_collection.json \
  -e RGS_Local.postman_environment.json \
  -r html
```

## Test Coverage

| Category | Tests |
|----------|-------|
| Health Check | 2 |
| Authentication | 10 |
| Wallet | 7 |
| Games | 9 |
| E2E Journey | 12 |
| **Total** | **40** |

## Test Scenarios

### Authentication
- ✅ Successful registration
- ✅ Duplicate username rejection
- ✅ T&C acceptance requirement
- ✅ Password length validation
- ✅ Successful login
- ✅ Invalid password rejection
- ✅ Non-existent user rejection
- ✅ Session retrieval
- ✅ Unauthorized access blocking
- ✅ Logout

### Wallet
- ✅ Initial zero balance
- ✅ Deposit funds
- ✅ Balance verification after deposit
- ✅ Withdraw funds
- ✅ Insufficient funds rejection
- ✅ Transaction history

### Games
- ✅ List available games
- ✅ Get game details
- ✅ Start game session
- ✅ Play game with valid wager
- ✅ Multiple game rounds
- ✅ Insufficient balance rejection
- ✅ Below minimum wager rejection
- ✅ Game history retrieval

### Complete Player Journey
Full end-to-end flow simulating a real player:
1. Register new account
2. Login
3. Check initial balance
4. Deposit $500
5. Browse available games
6. Start game session
7. Play multiple rounds
8. Check game history
9. Check final balance
10. Withdraw winnings
11. View transaction history
12. Logout

## Notes

- Tests use dynamic timestamps to generate unique usernames/emails
- Authentication tokens are automatically stored and reused
- The collection is designed to run in order within each folder
- Pre-request scripts handle test data generation
- Test scripts validate responses and store variables for subsequent requests

