# Go build settings
BINARY := cloister
CMD_PATH := ./cmd/cloister

# D2 diagram settings
D2_SOURCES := $(wildcard docs/diagrams/*.d2)
D2_SVGS := $(D2_SOURCES:.d2=.svg)

.PHONY: build install test lint clean diagrams clean-diagrams

# Go targets
build:
	go build -o $(BINARY) $(CMD_PATH)

install:
	go install $(CMD_PATH)

test:
	go test ./...

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
