.PHONY: build dev test docker release clean

BIN := sumolite
PKG := ./cmd/sumolite
DIST := dist

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

build:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(DIST)/$(BIN) $(PKG)

dev:
	go run $(PKG) serve --addr :7878

test:
	go test ./...

docker:
	docker buildx build -f docker/Dockerfile \
	  --platform linux/amd64,linux/arm64 \
	  -t ghcr.io/asklokesh/sumolite:latest .

# Cross-compile release artifacts. macOS uses CGO so we don't cross-compile
# darwin from linux here; the GitHub workflow runs matrix builds instead.
release:
	mkdir -p $(DIST)
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	  go build -ldflags="-s -w" -o $(DIST)/$(BIN)-$(GOOS)-$(GOARCH) $(PKG)

clean:
	rm -rf $(DIST)
