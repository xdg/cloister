# Go build settings
BINARY := cloister
CMD_PATH := ./cmd/cloister

# Test settings
#   COUNT=1    - bust cache, COUNT=N for flakiness testing
#   RUN=regex  - run only tests matching regex (-run flag)
#   PKG=path   - test specific package(s), default ./...
COUNT ?=
RUN ?=
PKG ?= ./...
COUNT_FLAG = $(if $(COUNT),-count=$(COUNT))
RUN_FLAG = $(if $(RUN),-run=$(RUN))

# D2 diagram settings
D2_SOURCES := $(wildcard docs/diagrams/*.d2)
D2_SVGS := $(D2_SOURCES:.d2=.svg)

.PHONY: build docker install test test-race test-integration test-all fmt lint clean diagrams clean-diagrams

# Go targets
build:
	go build -o $(BINARY) $(CMD_PATH)

docker:
	docker build -t cloister:latest .

install:
	go install $(CMD_PATH)

test:
	go test $(COUNT_FLAG) $(RUN_FLAG) $(PKG)

test-race:
	go test -race $(COUNT_FLAG) $(RUN_FLAG) $(PKG)

test-integration:
	go test -tags=integration $(COUNT_FLAG) $(RUN_FLAG) -p 1 $(PKG)

test-all: test-integration

fmt:
	goimports -w .

lint:
	golangci-lint run --build-tags=integration

clean:
	rm -f $(BINARY)

# Diagram targets
diagrams: $(D2_SVGS)

docs/diagrams/%.svg: docs/diagrams/%.d2
	d2 --pad=20 $< $@

clean-diagrams:
	rm -f $(D2_SVGS)
