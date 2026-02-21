# Go build settings
BINARY := cloister
CMD_PATH := ./cmd/cloister
GO_FILES := $(shell find . -name '*.go' -type f -not -name '*_test.go' -not -path './test/*')
GO_MOD_FILES := go.mod go.sum
GO_VERSION := $(shell grep '^go ' go.mod | cut -d' ' -f2 | cut -d. -f1,2)

# Version settings
# Set VERSION to build a release (e.g., VERSION=v1.0.0)
# When VERSION is set, the binary uses cloister:<version> image; otherwise cloister:latest
VERSION ?=
VERSION_PKG := github.com/xdg/cloister/internal/version
LDFLAGS := $(if $(VERSION),-ldflags "-X $(VERSION_PKG).Version=$(VERSION)")

# Docker image settings
# make docker       -> cloister:latest (or cloister:$(VERSION) if set)
# make docker-commit-tag -> cloister:<commit> for worktree-isolated tests
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)
DOCKER_TAG := $(if $(VERSION),$(VERSION),latest)
TEST_IMAGE := cloister:$(GIT_COMMIT)

# Test settings
#   COUNT=1    - bust cache, COUNT=N for flakiness testing
#   RUN=regex  - run only tests matching regex (-run flag)
#   PKG=path   - test specific package(s), default ./...
#   VERBOSE=1  - verbose test output (-v flag)
COUNT ?=
RUN ?=
PKG ?= ./...
VERBOSE ?=
COUNT_FLAG = $(if $(COUNT),-count=$(COUNT))
RUN_FLAG = $(if $(RUN),-run=$(RUN))
VERBOSE_FLAG = $(if $(VERBOSE),-v)

# D2 diagram settings
D2_SOURCES := $(wildcard specs/diagrams/*.d2)
D2_SVGS := $(D2_SOURCES:.d2=.svg)

.PHONY: docker docker-no-cache docker-commit-tag install test test-race test-integration test-e2e test-all fmt lint clean diagrams clean-diagrams

# Go targets
$(BINARY): $(GO_FILES) $(GO_MOD_FILES)
	go build $(LDFLAGS) -o $(BINARY) $(CMD_PATH)

build: $(BINARY)

docker:
	docker build --build-arg GO_VERSION=$(GO_VERSION) $(if $(VERSION),--build-arg VERSION=$(VERSION)) -t cloister:$(DOCKER_TAG) .

docker-no-cache:
	docker build --no-cache --build-arg GO_VERSION=$(GO_VERSION) $(if $(VERSION),--build-arg VERSION=$(VERSION)) -t cloister:$(DOCKER_TAG) .

docker-commit-tag:
	docker build --build-arg GO_VERSION=$(GO_VERSION) -t $(TEST_IMAGE) .

install:
	go install $(LDFLAGS) $(CMD_PATH)

test:
	go test $(VERBOSE_FLAG) $(COUNT_FLAG) $(RUN_FLAG) $(PKG)

test-race:
	go test -race $(VERBOSE_FLAG) $(COUNT_FLAG) $(RUN_FLAG) $(PKG)

test-integration: docker-commit-tag
	go build $(LDFLAGS) -o $(BINARY) $(CMD_PATH)
	CLOISTER_IMAGE=$(TEST_IMAGE) go test -tags=integration $(VERBOSE_FLAG) $(COUNT_FLAG) $(RUN_FLAG) -p 1 $(PKG)

test-e2e: docker-commit-tag
	go build $(LDFLAGS) -o $(BINARY) $(CMD_PATH)
	CLOISTER_IMAGE=$(TEST_IMAGE) go test -tags=e2e $(VERBOSE_FLAG) $(COUNT_FLAG) $(RUN_FLAG) -p 1 ./test/e2e/...

test-all: test-race test-integration test-e2e

fmt:
	goimports -w .

lint:
	golangci-lint run --build-tags=integration,e2e

clean:
	rm -f $(BINARY)

# Diagram targets
diagrams: $(D2_SVGS)

specs/diagrams/%.svg: specs/diagrams/%.d2
	d2 --pad=20 $< $@

clean-diagrams:
	rm -f $(D2_SVGS)
