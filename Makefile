# Makefile for lets - drives CLI build and plugin tasks.
#
# Windows: GNU Make defaults to cmd.exe which lacks POSIX shell syntax.
# If make targets fail in cmd.exe, run from Git Bash or WSL.
# When a Windows contributor needs proper detection, see beads/Makefile pattern.

.PHONY: all build test vet lint fmt fmt-check install install-go clean help

CLI_DIR := cli

# VERSION: command line override > git tag (with leading 'v' stripped) > empty
# Tags follow vX.Y.Z convention. Strip 'v' so version string is X.Y.Z (matches cobra default).
# When VERSION is empty, no -ldflags is passed, and the Go default in version.go wins.
# This makes version.go the single source of truth for the dev placeholder.
#
# TODO(lets-pplgq): release script must enforce that plugins/lets/.claude-plugin/plugin.json
# "version" matches the git tag (lockstep). No automated check today - manual audit before tagging.
VERSION := $(shell git describe --tags --exact-match 2>/dev/null | sed 's/^v//')

LDFLAGS :=
ifneq ($(VERSION),)
LDFLAGS := -ldflags "-X github.com/restarter/lets-workflow/cli/internal/version.Version=$(VERSION)"
endif

all: build

help:
	@echo "Targets:"
	@echo "  build           - Build cli/lets binary (-trimpath, ldflags from git tag if present)"
	@echo "  test            - Run cli unit tests with -race"
	@echo "  vet             - Run go vet"
	@echo "  lint            - Run golangci-lint (requires it installed)"
	@echo "  fmt             - Run gofmt -w -s"
	@echo "  fmt-check       - Verify gofmt is clean (CI use)"
	@echo "  install         - Install lets to /usr/local/bin (or ~/.local/bin if not writable)"
	@echo "  install-go      - Install lets to \$$GOBIN via 'go install' (Go-standard layout)"
	@echo "  clean           - Remove built binary and test cache"

build:
	cd $(CLI_DIR) && go build -trimpath $(LDFLAGS) -o lets ./cmd/lets

test:
	cd $(CLI_DIR) && go test -race ./...

vet:
	cd $(CLI_DIR) && go vet ./...

lint:
	cd $(CLI_DIR) && golangci-lint run ./...

fmt:
	cd $(CLI_DIR) && gofmt -w -s .

fmt-check:
	@cd $(CLI_DIR) && test -z "$$(gofmt -l .)" || (echo "Code not formatted - run 'make fmt'" && gofmt -l . && exit 1)

install: build
	@if [ -w /usr/local/bin ]; then \
		install_dir="/usr/local/bin"; \
	else \
		install_dir="$$HOME/.local/bin"; \
		mkdir -p "$$install_dir"; \
	fi; \
	cp $(CLI_DIR)/lets "$$install_dir/lets" && \
	echo "Installed: $$install_dir/lets" && \
	case ":$$PATH:" in \
		*":$$install_dir:"*) ;; \
		*) \
			echo ""; \
			echo "Warning: $$install_dir is not in your PATH"; \
			echo "Add to your shell rc (~/.zshrc, ~/.bashrc):"; \
			echo "  export PATH=\"$$install_dir:\$$PATH\""; \
			;; \
	esac

# install-go: traditional 'go install' to $GOBIN (or $GOPATH/bin).
# Use when you specifically want the Go-standard layout. Requires
# $GOBIN/$(go env GOPATH)/bin to be in PATH (Go does NOT manage this).
install-go: build
	cd $(CLI_DIR) && go install -trimpath $(LDFLAGS) ./cmd/lets

clean:
	rm -f $(CLI_DIR)/lets $(CLI_DIR)/lets.exe
	cd $(CLI_DIR) && go clean -testcache
