# RGS - Remote Gaming Server

A Go-based Remote Gaming Server designed for GLI-19 compliance.

## Quick Start

```bash
go run main.go
```

The server will start on port 8080.

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Server info |
| `/health` | GET | Health check |

## Development

```bash
# Run tests
go test ./...

# Build
go build -o rgs .

# Run
./rgs
```

## License

MIT

