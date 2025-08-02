# Nostr Sample Relay

A sample Nostr relay server implemented in Go.

## Features

- WebSocket-based Nostr protocol implementation
- Support for basic NIPs (Nostr Implementation Possibilities)
- Signature verification
- Event storage and distribution
- Subscription management
- Relay information provision

## Supported NIPs

- NIP-01: Basic protocol flow description
- NIP-02: Contact List and Petnames
- NIP-09: Event Deletion
- NIP-11: Relay Information Document
- NIP-12: Generic Tag Queries
- NIP-15: End of Stored Events Notice
- NIP-16: Event Treatment
- NIP-20: Command Results
- NIP-22: Event `created_at` Limits

## Requirements

- Go 1.21 or higher

## Installation and Usage

### 1. Install dependencies

```bash
go mod download
```

### 2. Start the server

```bash
go run main.go
```

The server will start on the following endpoints:
- WebSocket: `ws://localhost:8080/ws`
- Relay info: `http://localhost:8080/`

### 3. Run the test client

You can run the test client in a separate terminal:

```bash
go run client/main.go
```

## API

### WebSocket Endpoint

`ws://localhost:8080/ws`

Supported message types:

- `EVENT`: Event posting
- `REQ`: Event subscription
- `CLOSE`: Subscription termination

### Relay Information

`GET http://localhost:8080/`

- Browser: Display relay information in HTML format
- `Accept: application/nostr+json`: Return detailed relay information in JSON format

## Running with Docker

### Build

```bash
docker build -t nostr-relay .
```

### Run

```bash
docker run -p 8080:8080 nostr-relay
```

## Development

### Project Structure

```
.
├── main.go          # Main server implementation
├── client/
│   └── main.go      # Test client
├── go.mod           # Go modules configuration
├── go.sum           # Dependency hashes
├── Dockerfile       # Docker configuration
└── README.md        # This file
```

### Key Components

- `NostrServer`: Main server struct
- `Client`: Manages WebSocket connections
- `Subscription`: Manages client subscriptions
- Event filtering and broadcast functionality

## License

This project is a sample implementation for educational purposes.