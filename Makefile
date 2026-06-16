.PHONY: build build-all web server test clean run simulate

# Default: build server for the current platform (React app first, then Go)
build: web server

web:
	cd web && npm ci && npm run build

server:
	go build -ldflags="-s -w" -o bin/webhook-server ./cmd/server

simulate:
	go build -ldflags="-s -w" -o bin/simulate ./cmd/simulate

test:
	go test ./...
	cd web && npm test

# Cross-platform release builds (CGO_ENABLED=0 — modernc.org/sqlite is pure Go)
build-all: web
	mkdir -p dist
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/webhook-server-linux-amd64   ./cmd/server
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/webhook-server-linux-arm64   ./cmd/server
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/webhook-server-darwin-amd64  ./cmd/server
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/webhook-server-darwin-arm64  ./cmd/server
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/webhook-server-windows-amd64.exe ./cmd/server
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/simulate-linux-amd64          ./cmd/simulate
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/simulate-darwin-arm64         ./cmd/simulate
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/simulate-windows-amd64.exe    ./cmd/simulate

clean:
	rm -rf bin/ dist/ web/dist/

run:
	./bin/webhook-server
