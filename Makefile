.PHONY: build build-all web server test clean run simulate release

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

# release: builds all platforms, creates wrapper scripts, zips for distribution
release: build-all
	chmod +x dist/webhook-server-linux-amd64 dist/webhook-server-linux-arm64 \
	         dist/webhook-server-darwin-amd64 dist/webhook-server-darwin-arm64
	@printf '#!/bin/bash\ncd "$$(dirname "$$0")"\nxattr -d com.apple.quarantine ./webhook-server-darwin-arm64 2>/dev/null || true\n./webhook-server-darwin-arm64 --simulate\n' \
		> dist/open-macos-arm64.command && chmod +x dist/open-macos-arm64.command
	@printf '#!/bin/bash\ncd "$$(dirname "$$0")"\nxattr -d com.apple.quarantine ./webhook-server-darwin-amd64 2>/dev/null || true\n./webhook-server-darwin-amd64 --simulate\n' \
		> dist/open-macos-amd64.command && chmod +x dist/open-macos-amd64.command
	@printf '#!/bin/bash\ncd "$$(dirname "$$0")"\n./webhook-server-linux-amd64 --simulate\n' \
		> dist/run-linux.sh && chmod +x dist/run-linux.sh
	cd dist && zip -r ../webhook-delivery-release.zip \
		webhook-server-darwin-arm64 open-macos-arm64.command \
		webhook-server-darwin-amd64 open-macos-amd64.command \
		webhook-server-linux-amd64 run-linux.sh \
		webhook-server-windows-amd64.exe
	@echo "Release zip: webhook-delivery-release.zip"

clean:
	rm -rf bin/ dist/ web/dist/ webhook-delivery-release.zip

run:
	./bin/webhook-server
