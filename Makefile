# Go build settings
BINARY := cloister
CMD_PATH := ./cmd/cloister
GO_FILES := $(shell find . -name '*.go' -type f -not -name '*_test.go' -not -path './test/*')
GO_MOD_FILES := go.mod go.sum
GO_VERSION := $(shell grep '^go ' go.mod | cut -d' ' -f2 | cut -d. -f1,2)

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

.PHONY: docker install test test-race test-integration test-e2e test-all fmt lint clean diagrams clean-diagrams

# Go targets
$(BINARY): $(GO_FILES) $(GO_MOD_FILES)
	go build -o $(BINARY) $(CMD_PATH)

build: $(BINARY)

docker:
	docker build --build-arg GO_VERSION=$(GO_VERSION) -t cloister:latest .

install:
	go install $(CMD_PATH)

test:
	go test $(VERBOSE_FLAG) $(COUNT_FLAG) $(RUN_FLAG) $(PKG)

test-race:
	go test -race $(VERBOSE_FLAG) $(COUNT_FLAG) $(RUN_FLAG) $(PKG)

test-integration: $(BINARY)
	go test -tags=integration $(VERBOSE_FLAG) $(COUNT_FLAG) $(RUN_FLAG) -p 1 $(PKG)

test-e2e: $(BINARY) docker
	go test -tags=e2e $(VERBOSE_FLAG) $(COUNT_FLAG) $(RUN_FLAG) -p 1 ./test/e2e/...

test-all: test-integration test-e2e

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
