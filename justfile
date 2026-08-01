# List available recipes
default:
    @just --list

# Build the punchpage binary
build:
    go build -o punchpage ./cmd/punchpage

# Run all tests (unit + end-to-end)
test: test-go test-e2e

# Run Go unit tests
test-go:
    go test ./...

# Run the full end-to-end tunnel test (fixture -> host -> relays -> headless Chromium)
test-e2e:
    pnpm -C e2e install && pnpm -C e2e e2e

# Vet and gofmt check
lint:
    gofmt -l . && go vet ./...

# Install web dependencies and build the browser client
web:
    pnpm -C web install && pnpm -C web build

# Typecheck the browser client
typecheck:
    pnpm -C web typecheck

# Share a local origin (default http://127.0.0.1:3000)
run target="http://127.0.0.1:3000":
    go run ./cmd/punchpage --target {{target}}

# Share the built-in demo site — open the printed link in any browser
demo:
    go run ./cmd/punchpage demo
