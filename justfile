# List available recipes
default:
    @just --list

# Build the punchpage binary
build:
    go build -o punchpage ./cmd/punchpage

# Run Go unit tests
test:
    go test ./...

# Vet and gofmt check
lint:
    gofmt -l . && go vet ./...

# Install web dependencies and build the browser client
web:
    pnpm -C web install && pnpm -C web build

# Typecheck the browser client
typecheck:
    pnpm -C web typecheck

# Run the full end-to-end tunnel test (fixture -> host -> relays -> headless Chromium)
e2e:
    pnpm -C e2e install && pnpm -C e2e e2e

# Share a local origin (default http://127.0.0.1:3000)
run target="http://127.0.0.1:3000":
    go run ./cmd/punchpage --target {{target}}

# Share a built-in demo site — open the printed link in any browser
demo:
    #!/usr/bin/env bash
    set -euo pipefail
    go run ./e2e/fixture --port 8213 &
    trap 'kill $! 2>/dev/null' EXIT
    sleep 1
    go run ./cmd/punchpage --target http://127.0.0.1:8213
