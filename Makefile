.PHONY: web build dev test clean

web:
	cd web && npm ci && npm run build

# Single static binary with the frontend embedded.
build: web
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o bin/ydiskspanel ./cmd/ydiskspanel

# Run backend on :8080; run `cd web && npm run dev` for the hot-reload UI (proxies /api).
dev:
	go run ./cmd/ydiskspanel

test:
	go test ./...
	cd web && npm run typecheck

clean:
	rm -rf bin web/dist/assets web/dist/index.html
