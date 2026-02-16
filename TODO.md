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

- [x] For each package, collect per-file import sets (external imports only)
- [x] Identify files whose imports share nothing with sibling files
- [x] Flag files where unique imports suggest a different domain than the package name

> **Findings:**
> - No strong domain mismatches found — outlier files reflect complementary concerns within coherent domains
> - `token/store.go` is an import outlier (imports only `config`), reinforcing Pass B finding of weak `token` package cohesion
> - `container/config.go` and `container/manager.go` are disjoint (config vs lifecycle) but domain-appropriate
> - `project/detect.go` (git detection) and `project/registry.go` (YAML persistence) are functionally independent — note for future growth
> - `cmd/` package (13 files) has no outliers — healthy cohesion
> - No relocations indicated by import analysis alone; cross-reference with Pass B confirms `token/credentials.go` as movable

---

## Phase 2: Manual Review & Triage

### 2.1 Pass E: Domain mismatch via go doc

- [x] Run `go doc` for each `internal/` package
- [x] Flag exported symbols whose names or purposes don't fit the package's domain
- [x] Cross-reference with Phase 1 findings

> **Findings — new mismatches:**
> - `version.DefaultRegistry`, `version.ImageEnvVar`, `version.DefaultImage()` → move to `container` (image selection ≠ version info)
> - `cmd.AuthMethod` type + constants → move to `config` (third location for auth method concept, alongside `claude` and `codex`)
> - `agent.ContainerUID`/`ContainerGID` → consider moving to `container` alongside `DefaultUID`
> - `executor` package: half its surface is daemon state management — cohesion concern for future
>
> **Phase 1 findings confirmed:**
> - `AuthMethodAPIKey` duplication now known to span 3 packages (`cmd`, `claude`, `codex`)
> - `DefaultTokenAPIPort`/`DefaultAPIPort` redundancy in `guardian` confirmed
> - `DefaultRequestPort` consolidatable between `guardian` and `guardian/request`
> - `token.CredentialEnvVars` confirmed as belonging in `cloister`
> - `token` package weak cohesion confirmed
> - `guardian` package (60+ exported symbols) confirmed as decomposition candidate

### 2.2 Consolidate and prioritize

- [x] Merge results from passes B, C, D, E into a single candidate list
- [x] For each candidate: move to existing package, extract to new package, or leave as-is
- [x] Document each proposed move with source, destination, and justification

> **Approved moves (Phase 3 execution order):**
>
> | # | What | From | To | Justification |
> |---|------|------|----|---------------|
> | M1 | `DefaultRegistry`, `ImageEnvVar`, `DefaultImage()` | `version` | `container` | Image selection is container config, not version info (Pass E) |
> | M2 | `AuthMethod` type, `AuthMethodAPIKey`, `AuthMethodToken` | `cmd` + `claude` + `codex` | `config` | Three packages define auth method concepts; unify (Pass C + E) |
> | M3 | `CredentialEnvVars`, `CredentialEnvVarsUsed` | `token` | `cloister` (or inline) | Only consumer is `cloister.go`; deprecated passthrough (Pass B + E) |
> | M4 | `DefaultTokenAPIPort` / `DefaultAPIPort` | `guardian` (both) | `guardian` (one name) | Same-package redundancy, consolidate to `DefaultAPIPort` (Pass C) |
> | ~~M5~~ | ~~`DefaultRequestPort`~~ | ~~`guardian`~~ | ~~remove; use `guardian/request`~~ | **Cancelled**: test-time import cycle discovered (`guardian` → `request` → `testutil` → `guardian`) |
>
> **Leave as-is (justified):**
> - `InstanceIDEnvVar` in `guardian`/`executor` — real import cycle, consistency test exists
> - `DefaultApprovalPort` in `guardian`/`guardian/approval` — real import cycle
> - `DefaultRequestPort` in `guardian`/`guardian/request` — real test-time import cycle (discovered during M5 attempt)
> - `token/token.go` (Generate/TokenBytes) — core token concepts, no mismatch
> - `agent.ContainerUID`/`ContainerGID` — mild mismatch but pragmatic; only used in agent setup
> - `testutil/http.go` (NoProxyClient) — testutil is a reasonable shared location
>
> **Deferred to Future Phases:**
> - `executor` daemon state management split — cohesion concern but large scope
> - `guardian` package decomposition — 60+ symbols, needs its own planning effort

---

## Phase 3: Refactoring

Execute moves sequentially. Each move is one atomic commit.

### 3.1 Move M1: Image constants from `version` to `container`

- [x] Move `DefaultRegistry`, `ImageEnvVar`, `DefaultImage()` from `version/version.go` to `container/`
- [x] Update all import sites to use `container.DefaultRegistry`, `container.ImageEnvVar`, `container.DefaultImage()`
- [x] Remove moved symbols from `version/version.go`
- [x] Verify: `go build ./...`, `make test`, `make lint`

### 3.2 Move M2: Auth method types to `config`

- [x] Define `AuthMethod` type, `AuthMethodAPIKey`, `AuthMethodToken` in `config/`
- [x] Update `claude/inject.go` to use `config.AuthMethodAPIKey`, `config.AuthMethodToken`
- [x] Update `codex/inject.go` to use `config.AuthMethodAPIKey`
- [x] Update `cmd/` to use `config.AuthMethod` type and constants
- [x] Remove old auth method definitions from `claude`, `codex`, `cmd`
- [x] Verify: `go build ./...`, `make test`, `make lint`

### 3.3 Move M3: Credential env vars from `token` to `cloister`

- [x] Move `CredentialEnvVars`, `CredentialEnvVarsUsed`, `credentialEnvVarNames` from `token/credentials.go` to `cloister/`
- [x] Update all import sites (expected: only `cloister/cloister.go`)
- [x] Delete `token/credentials.go`
- [x] Verify: `go build ./...`, `make test`, `make lint`

### 3.4 Move M4: Consolidate API port constants in `guardian`

- [x] Replace `DefaultTokenAPIPort` with `DefaultAPIPort` (or vice versa) in `guardian/`
- [x] Update all references to the removed name
- [x] Verify: `go build ./...`, `make test`, `make lint`

### 3.5 Move M5: Remove duplicate `DefaultRequestPort` from `guardian`

- [x] ~~Remove `DefaultRequestPort` from `guardian/instance.go`~~ — **Cancelled**: test-time import cycle (`guardian` → `guardian/request` → `testutil` → `guardian`) prevents this. Duplication is justified, same pattern as `DefaultApprovalPort` and `InstanceIDEnvVar`.
- [x] ~~Update references to use `request.DefaultRequestPort`~~ — N/A (see above)
- [x] Verify: `go build ./...`, `make test`, `make lint` — confirmed cycle exists

### 3.6 Final verification

- [ ] Full `make test` pass
- [ ] Full `make lint` pass (0 issues)
- [ ] Grep for any remaining "import cycle" or "mirrors" comments that are now stale

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
