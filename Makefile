# Go build settings
BINARY := cloister
CMD_PATH := ./cmd/cloister

# Test settings (set COUNT=1 to bust cache, COUNT=N for flakiness testing)
COUNT ?=
COUNT_FLAG = $(if $(COUNT),-count=$(COUNT))

# D2 diagram settings
D2_SOURCES := $(wildcard docs/diagrams/*.d2)
D2_SVGS := $(D2_SOURCES:.d2=.svg)

.PHONY: build docker install test test-race test-integration test-all lint clean diagrams clean-diagrams

# Go targets
build:
	go build -o $(BINARY) $(CMD_PATH)

docker:
	docker build -t cloister:latest .

install:
	go install $(CMD_PATH)

test:
	go test $(COUNT_FLAG) ./...

test-race:
	go test -race $(COUNT_FLAG) ./...

test-integration:
	go test -tags=integration $(COUNT_FLAG) -p 1 ./...

test-all: test-integration

lint:
	golangci-lint run

clean:
	rm -f $(BINARY)

# Diagram targets
diagrams: $(D2_SVGS)

docs/diagrams/%.svg: docs/diagrams/%.d2
	d2 --pad=20 $< $@

clean-diagrams:
	rm -f $(D2_SVGS)
