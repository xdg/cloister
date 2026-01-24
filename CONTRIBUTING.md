# Contributing to Cloister

Contributions are welcome! This document covers the basics.

## Before You Start

Please read the documentation in [docs/](docs/):

- **[cloister-spec.md](docs/cloister-spec.md)** — Architecture, threat model, and security analysis
- **[implementation-phases.md](docs/implementation-phases.md)** — Development roadmap and current priorities

Contributions should align with the spec and roadmap. If you're considering a significant change, open an issue first to discuss.

## Getting Started

1. Fork and clone the repository
2. Install Go 1.25+
3. Install development tools: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
4. Run tests: `go test ./...`
5. Run linter: `golangci-lint run`

## Development Workflow

1. Check existing issues or open a new one to discuss your idea
2. Fork the repo and create a branch from `main`
3. Make your changes with tests
4. Ensure `go test ./...` and `golangci-lint run` pass
5. Submit a pull request

## Pull Request Guidelines

- Keep PRs focused on a single change
- Include tests for new functionality
- Update documentation if needed
- Reference related issues in the PR description

## Security

Cloister is security infrastructure. Please:

- **Never weaken security controls** without discussion
- **Review the [spec](docs/cloister-spec.md)** to understand the threat model
- **Report vulnerabilities privately** via [GitHub Security Advisories](https://github.com/xdg/cloister/security/advisories/new)

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and testable
- Prefer clarity over cleverness

## Questions?

Open a [discussion](https://github.com/xdg/cloister/discussions) or issue.
