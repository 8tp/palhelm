VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all frontend backend build docker test clean dev-backend dev-frontend

all: build

frontend:
	cd frontend && npm ci --no-audit --no-fund && npm run build

# embeds frontend/dist into the binary via backend/internal/webdist
backend:
	rm -rf backend/internal/webdist/dist
	cp -r frontend/dist backend/internal/webdist/dist
	cd backend && CGO_ENABLED=0 go build -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" -o ../palhelm ./cmd/palhelm

build: frontend backend

docker:
	docker build --build-arg VERSION=$(VERSION) -t palhelm:$(VERSION) -t palhelm:latest .

test:
	cd backend && go vet ./... && go test ./...
	cd frontend && npx tsc --noEmit

dev-backend:
	cd backend && go run ./cmd/palhelm serve

dev-frontend:
	cd frontend && npm run dev

clean:
	rm -f palhelm
	rm -rf frontend/dist backend/internal/webdist/dist
	git checkout -- backend/internal/webdist/dist 2>/dev/null || true
