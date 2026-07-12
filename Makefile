APP := cxre
MODULE := github.com/rcmcsweeney/cxre
GO ?= go
GOFMT ?= gofmt
BINDIR ?= $(CURDIR)/bin
VERSION ?= dev
COMMIT ?= unknown
BUILD_DATE ?= unknown
GOFILES := $(shell find . -type f -name '*.go' -not -path './vendor/*')
LDFLAGS := -s -w \
	-X $(MODULE)/internal/buildinfo.Version=$(VERSION) \
	-X $(MODULE)/internal/buildinfo.Commit=$(COMMIT) \
	-X $(MODULE)/internal/buildinfo.BuildDate=$(BUILD_DATE)

.DEFAULT_GOAL := build

.PHONY: build install test test-race integration coverage fmt fmt-check tidy \
	tidy-check vet vulncheck check ci snapshot release clean

build:
	mkdir -p $(BINDIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(APP) ./cmd/cxre

install:
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/cxre

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

integration:
	CXRE_INTEGRATION=1 $(GO) test ./...

coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

fmt:
	$(GOFMT) -w $(GOFILES)

fmt-check:
	@command -v $(GOFMT) >/dev/null
	@test -z "$$($(GOFMT) -l $(GOFILES))" || (echo "Run 'make fmt' on:"; $(GOFMT) -l $(GOFILES); exit 1)

tidy:
	$(GO) mod tidy

tidy-check:
	$(GO) mod tidy -diff

vet:
	$(GO) vet ./...

vulncheck:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.5.0 ./...

check: fmt-check tidy-check vet test

ci: check test-race vulncheck

snapshot:
	goreleaser release --snapshot --clean

release:
	goreleaser release --skip=publish --clean

clean:
	$(GO) clean
	rm -rf $(BINDIR) dist coverage.out
