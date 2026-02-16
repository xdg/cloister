# Package Cohesion Refactoring

Find and fix misplaced code: functions, types, and constants that live in the wrong package.

## Testing Philosophy

- **Build verification after each move**: `go build ./...` catches import errors immediately
- **Unit tests**: `make test` covers ~90% of the codebase without Docker
- **Lint**: `make lint` catches stuttering names, unused imports, and style issues
- **No integration/e2e** unless touching Docker/container code

## Verification Checklist

Before marking a phase complete:

1. `go build ./...` passes
2. `make test` passes
3. `make lint` passes (0 issues)
4. No remaining references to moved/deleted symbols (grep confirmation)

When verification of a phase or subphase is complete, commit all
relevant newly-created and modified files.

## Dependencies Between Phases

```
Phase 1 (Automated Detection — parallel passes)
       │
       ▼
Phase 2 (Manual Review & Triage)
       │
       ▼
Phase 3 (Refactoring — sequential per move)
```

---

## Phase 1: Automated Detection

Run passes B, C, D in parallel to identify misplaced code.

### 1.1 Pass B: Orphan file detection

For each `.go` file in `internal/`, check if it references any unexported
symbol from its own package. Files referencing zero unexported symbols are
candidates for relocation.

- [x] Scan all non-test `.go` files in `internal/`
- [x] For each file, check for references to unexported identifiers defined in sibling files
- [x] Record findings as `[file] → [suggested home]` with rationale

> **Findings:**
> - `internal/testutil/http.go` → `internal/guardian/` or delete (duplicates noProxyClient)
> - `internal/token/credentials.go` → `internal/cloister/` or delete (deprecated, only used by cloister.go)
> - `internal/token/token.go` → stays in `token` (widely used externally, no intra-package coupling)
> - 9 provider-only files identified (define shared types/helpers; not true orphans)
> - 9 single-file packages noted (audit, claude, cloister, codex, guardian/executor, pathutil, prompt, term, version)
> - `internal/guardian/` (14 files) has weak internal cohesion — decomposition candidate
> - `internal/token/` (4 files) has near-zero internal coupling

### 1.2 Pass C: Duplicated constants

Compare exported `const` declarations across `internal/` packages for
identical string or integer values.

- [x] Extract all exported `const` declarations across `internal/`
- [x] Flag duplicates with identical values across packages
- [x] For each duplicate, verify whether it's justified by a real import cycle (use `go list`)

> **Findings:**
> - `AuthMethodAPIKey` = `"api_key"` duplicated in `claude` and `codex` — **no import cycle**, consolidate to `config` package
> - `InstanceIDEnvVar` = `"CLOISTER_INSTANCE_ID"` duplicated in `guardian` and `executor` — **justified** (import cycle: `guardian` → `executor`), consistency test exists
> - `DefaultApprovalPort` = `9999` duplicated in `guardian` and `guardian/approval` — **justified** (import cycle: `guardian` → `approval`)
> - `DefaultRequestPort` = `9998` duplicated in `guardian` and `guardian/request` — **no cycle**, could consolidate
> - `DefaultTokenAPIPort` / `DefaultAPIPort` = `9997` both in `guardian` — same-package redundancy, consolidate to one name

### 1.3 Pass D: Import outliers

For each package in `internal/`, find files whose imports are disjoint from
the rest of the package, suggesting the file belongs elsewhere.

- [ ] For each package, collect per-file import sets (external imports only)
- [ ] Identify files whose imports share nothing with sibling files
- [ ] Flag files where unique imports suggest a different domain than the package name

---

## Phase 2: Manual Review & Triage

### 2.1 Pass E: Domain mismatch via go doc

- [ ] Run `go doc` for each `internal/` package
- [ ] Flag exported symbols whose names or purposes don't fit the package's domain
- [ ] Cross-reference with Phase 1 findings

### 2.2 Consolidate and prioritize

- [ ] Merge results from passes B, C, D, E into a single candidate list
- [ ] For each candidate: move to existing package, extract to new package, or leave as-is
- [ ] Document each proposed move with source, destination, and justification

---

## Phase 3: Refactoring

Execute moves sequentially. Each sub-phase is one atomic commit.

### 3.1 Execute moves

- [ ] For each approved move: relocate code, update all import sites, fix tests
- [ ] **Test**: `go build ./...` after each move
- [ ] **Test**: `make test` after each move
- [ ] **Test**: `make lint` after each move

### 3.2 Final verification

- [ ] Full `make test` pass
- [ ] Full `make lint` pass (0 issues)
- [ ] Grep for any remaining "import cycle" or "mirrors" comments that are now stale

---

## Future Phases (Deferred)

### Deeper structural analysis
- Analyze whether any packages should be split (e.g., `guardian` is large)
- Evaluate whether `guardian/request`, `guardian/approval`, `guardian/executor` sub-packages are well-factored
- Consider whether `internal/cloister` and `internal/container` have the right boundary
