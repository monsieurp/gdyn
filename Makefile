GO ?= go
CGO_ENABLED ?= 0
VERSION ?=

.PHONY: build test clean

build:
	@version='$(VERSION)'; \
	if [ -z "$$version" ]; then \
		version=$$(git describe --tags --dirty 2>/dev/null || printf '%s' dev); \
	fi; \
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -ldflags="-X main.version=$$version" -o gdyn ./cmd/gdyn

test:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) test ./...

clean:
	rm -f gdyn
