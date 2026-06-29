# Go web

A mini HTTP web framework built from scratch in Go using raw TCP sockets — no `net/http`, no third-party libraries.

Built as a learning project to understand how real web frameworks work internally: routing, middleware, context, graceful shutdown, and more.

---

## Features

- **TCP-level HTTP parsing** — reads and parses raw HTTP/1.1 requests from the socket
- **Router** — method + path matching with static routes and named parameters (`{id}`)
- **Handler functions** — clean, uniform `func(ctx *Context) (Response, error)` signature
- **Route parameters** — extract path segments like `/files/{filename}` or `/users/{id}`
- **Request & Response abstractions** — structured types with automatic serialization to HTTP wire format
- **Context object** — per-request state carrying the parsed request and route params
- **Middleware** — composable chain applied per route
- **Logging middleware** — structured log per request: method, path, status, duration
- **Error handling middleware** — converts `*APIError` values into proper HTTP responses
- **Panic recovery middleware** — catches panics in handlers, returns 500, keeps server alive
- **Graceful shutdown** — listens for `SIGINT`/`SIGTERM`, drains in-flight connections, exits cleanly within a 30-second timeout
- **Dependency injection** — shared state (e.g. file directory) injected into handlers via `App` struct and closures
- **Configuration management** — CLI flags (`--host`, `--port`, `--directory`) parsed via `flag` package

---

## Project Structure

```
.
├── main.go        # Everything: server, router, handlers, middleware, config
```

Currently a single-file project, structured to be split into packages as it grows.

---

## Getting Started

### Prerequisites

- Go 1.21+

### Run

```bash
go run main.go --directory /tmp/files
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `--host` | `0.0.0.0` | Host to listen on |
| `--port` | `4221` | Port to listen on |
| `--directory` | _(required)_ | Directory for file read/write endpoints |

---

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Health check — returns 200 |
| `GET` | `/echo/{str}` | Echoes `str` back as plain text |
| `GET` | `/user-agent` | Returns the request's `User-Agent` header |
| `GET` | `/users/{id}` | Returns the user ID from the path |
| `GET` | `/files/{filename}` | Reads and returns a file from `--directory` |
| `POST` | `/files/{filename}` | Writes request body to a file in `--directory` |
| `GET` | `/slow` | Sleeps 10 seconds — useful for testing graceful shutdown |
| `GET` | `/panic` | Triggers a panic — demonstrates recovery middleware |

---

## Middleware

Middleware is composed using `Chain(handler, ...middleware)`.

```go
router.GET("/echo/{str}", Chain(handleEcho, ErrorHandlingMiddleware, LogMiddleware, RecoverMiddleware))
```

Execution order follows the chain left to right (outermost first). Built-in middleware:

- `LogMiddleware` — logs method, path, status, and duration after the handler returns
- `ErrorHandlingMiddleware` — catches `*APIError` from handlers and writes the correct HTTP status
- `RecoverMiddleware` — catches panics via `defer/recover` and returns 500 without crashing the server
- `NoOpMiddleware` — pass-through, useful as a placeholder

---

## Example Requests

```bash
# Echo
curl http://localhost:4221/echo/hello

# User-Agent
curl http://localhost:4221/user-agent

# Write a file
curl -X POST http://localhost:4221/files/test.txt -d "hello world"

# Read it back
curl http://localhost:4221/files/test.txt

# Trigger panic recovery
curl http://localhost:4221/panic

# Test graceful shutdown (start slow request, then Ctrl+C)
curl http://localhost:4221/slow &
# Then press Ctrl+C — server waits for the slow request to finish
```

---

## Graceful Shutdown

The server listens for `SIGINT` (Ctrl+C) and `SIGTERM` (Kubernetes). On signal:

1. Stops accepting new connections
2. Waits for all in-flight goroutines to finish (up to 30 seconds)
3. Exits cleanly

```
signal=interrupt shutting down...
shutdown clean
```

---

## What I Learned Building This

- How HTTP is parsed from raw bytes over a TCP socket
- How routers work: pattern matching, parameter extraction, handler dispatch
- How middleware chains are composed using function wrapping
- How to design a `Context` object that carries per-request state
- How `sync.WaitGroup` and OS signals enable graceful shutdown
- How dependency injection works without a framework — just structs and closures
- Why separating request parsing, routing, and response serialization makes a codebase maintainable

---

## Roadmap

- [ ] Multi-read for large request bodies (chunked reads respecting `Content-Length`)
- [ ] Gzip compression via `Accept-Encoding` negotiation
- [ ] Keep-alive / persistent connections
- [ ] Split into packages: `router`, `middleware`, `context`, `server`
- [ ] Unit tests for route matching, request parsing, and response serialization
- [ ] Query parameter parsing

---

## License

MIT
