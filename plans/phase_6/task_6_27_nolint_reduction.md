# Plan: Task 6.27: Reduce `//nolint` Tags to Absolute Minimum

**Status:** Completed
**Go Version:** 1.26
**Unit Test Coverage Target:** 91%

The codebase currently carries **185 `//nolint` suppressions** across production and test code. The vast majority are either repeated, identical justifications for the same pattern (which can be consolidated into a single documented helper), or the result of functions that are too large to pass complexity-analysis linters (`gocognit`, `funlen`, `nestif`, `gocyclo`). This task systematically eliminates as many suppressions as possible through refactoring — without relaxing linter thresholds.

Current breakdown:

| Linter | Total | Prod | Tests |
|--------|-------|------|-------|
| `gosec` | 151 | 95 | 56 |
| `gocognit` | 24 | ~16 | ~8 |
| `funlen` | 14 | ~10 | ~4 |
| `nestif` | 11 | ~9 | ~2 |
| `gocyclo` | 5 | 5 | 0 |
| `errcheck` | 4 | 1 | 3 |
| `staticcheck` | 2 | 2 | 0 |
| `noctx` | 2 | 2 | 0 |

---

## Proposed Changes

### 1. Consolidate Repeated `gosec` Suppressions via Shared Helpers

The largest category — `gosec` — fires repeatedly for the same justified patterns across many call sites. The fix is to extract a single, well-documented internal helper per pattern and suppress once there.

#### Pattern A: File reads from caller-validated paths

Currently ~12 call sites in `pkg/linter` and `pkg/config` each carry their own `//nolint:gosec` on `os.ReadFile` / `os.Stat` calls where the path has already been validated by the calling function.

**Fix:** Introduce `safeReadFile(path string) ([]byte, error)` and `safeStat(path string) (os.FileInfo, error)` private helpers in the affected packages. Each carries a single, justified suppression with a full explanation. All call sites drop their per-line tags.

Affected files:
- [MODIFY] [validator.go](file://../../pkg/linter/validator.go)
- [MODIFY] [config.go](file://../../pkg/config/config.go)

#### Pattern B: `exec.CommandContext` with a variable ffmpeg path (G204)

`internal/plugins/viral/viral.go` calls `exec.CommandContext(ctx, ffmpegCmd, args...)` in at least 5 separate functions (`runMockVideo`, `generateTTSMock`, `StitchTrailer`, `renderSlide`, `concatSegments`, `finalizeOutput`), each with its own `//nolint:gosec`.

**Fix:** Extract a `runFFmpeg(ctx context.Context, ffmpegCmd string, args []string) ([]byte, error)` private helper in the `viral` package, carrying the single justified suppression. All 5+ call sites drop their tags.

Affected files:
- [MODIFY] [viral.go](file://../../internal/plugins/viral/viral.go)

#### Pattern C: HTTP requests to dynamic-but-trusted URLs (G107)

Multiple plugins (`viral.go`, `imagegen.go`, `trends/source.go`, `seo/seo.go`) carry `//nolint:gosec` on both `http.NewRequestWithContext` *and* `client.Do`. The `client.Do` suppression is redundant — G107 does not fire on `client.Do`. The `NewRequestWithContext` suppressions can be consolidated into a package-level `newTrustedRequest(ctx, method, url, body)` helper per plugin.

Affected files:
- [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)
- [MODIFY] [viral.go](file://../../internal/plugins/viral/viral.go)
- [MODIFY] [source.go](file://../../internal/plugins/trends/source.go)
- [MODIFY] [seo.go](file://../../internal/plugins/seo/seo.go)

#### Pattern D: `cfgPath` construction in MCP server `main.go` entry points

8 MCP server entry points (`pw-mcp-epub`, `pw-mcp-cloud`, `pw-mcp-imagegen`, `pw-mcp-kdp-math`, `pw-mcp-pdfcheck`, `pw-mcp-seo`, `pw-mcp-trends`, `pw-mcp-viral`, `pw-mcp-youtube`) share the same boilerplate: build `cfgPath` from `workspaceRoot` and immediately `//nolint:gosec` the `os.ReadFile` call. This logic should be extracted into a shared `config.LoadFromWorkspace(root string) (*config.Config, error)` function in `pkg/config` that wraps the suppression once.

Affected files:
- [MODIFY] [config.go](file://../../pkg/config/config.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-epub/main.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-cloud/main.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-imagegen/main.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-kdp-math/main.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-pdfcheck/main.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-seo/main.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-trends/main.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-viral/main.go)
- [MODIFY] [main.go](file://../../cmd/pw-mcp-youtube/main.go)

---

### 2. Decompose Large Functions (Complexity Linters)

Every `gocognit`, `funlen`, `nestif`, and `gocyclo` suppression is a direct consequence of a function that is too long or deeply nested to fit within the configured thresholds (`gocognit: 20`, `funlen: 100 lines / 50 statements`, `nestif: 4`). The fix is to decompose these functions into smaller, independently testable helpers.

#### `internal/review/critic.go` — `VerifyWorkspace`

`VerifyWorkspace` (line 96) carries `//nolint:gocognit,funlen,nestif`. It handles plan linting, optional local build validation, MCP critic server startup, tool invocation, and result parsing in a single function.

**Fix:** Extract:
- `runLocalValidation(ctx, validationCmd) error` — handles `make all` execution
- `startCriticServer(ctx, cfg) (*internalmcp.ServerProcess, error)` — handles server discovery and startup
- `invokeCriticTool(ctx, srv, plan, validationCmd) (string, error)` — calls the MCP tool and extracts text
- `parseCriticResult(output string) error` — validates the VERDICT suffix

Affected files:
- [MODIFY] [critic.go](file://../../internal/review/critic.go)

#### `pkg/linter/validator.go` — `validateSinglePlan` and `FixAbsolutePathsInPlans`

`validateSinglePlan` (line 172) carries `//nolint:gocognit,funlen`. It does scanning, heading tracking, status/metadata parsing, and link validation in one pass.

**Fix:** Extract:
- `parsePlanContent(content string) planParseResult` — pure scanning, returns a struct with all parsed fields
- `checkCompletedMetadata(planFile string, result planParseResult) []string` — validates Go version, date, coverage
- `collectLinkErrors(workspaceRoot, planFile string, lines []string, status *string) []string` — link validation loop

`FixAbsolutePathsInPlans` (line 436) carries `//nolint:gocognit,funlen,nestif`. The inner line-editing loop mixes link-fixing and metadata-patching concerns.

**Fix:** Extract:
- `fixPlanFileLinks(workspaceRoot, planFile string, lines []string) ([]string, bool, error)` — link normalization pass
- `patchCompletedMetadata(lines []string, goVersion, today string) ([]string, bool)` — metadata auto-fill pass

Affected files:
- [MODIFY] [validator.go](file://../../pkg/linter/validator.go)

#### `internal/plugins/cloud/cloud.go` — Multiple Functions

Four functions carry complexity suppressions, with one (`//nolint:gocognit,gocyclo,funlen,nestif` at line 456) hitting all four simultaneously — the most severe case in the codebase.

**Fix:** Audit each function and extract sub-operations (input validation, pagination, resource mapping, error formatting) into focused helpers. Exact decomposition to be determined during implementation once function bodies are reviewed in detail.

Affected files:
- [MODIFY] [cloud.go](file://../../internal/plugins/cloud/cloud.go)

#### `internal/mcp/critic/server.go` — Handler Function

Line 64 carries `//nolint:gocognit,nestif`. The MCP tool handler likely mixes argument validation, command dispatch, and output formatting.

**Fix:** Extract argument parsing, validation, and execution into separate helpers following the same pattern used in other MCP server handlers.

Affected files:
- [MODIFY] [server.go](file://../../internal/mcp/critic/server.go)

#### `internal/mcp/git_diff.go` and `cmd/powerword/lint.go`

Both carry `//nolint:gocognit,nestif` and `//nolint:nestif` respectively.

Affected files:
- [MODIFY] [git_diff.go](file://../../internal/mcp/git_diff.go)
- [MODIFY] [lint.go](file://../../cmd/powerword/lint.go)

---

### 3. Audit and Remove Stale/Redundant Suppressions

#### `noctx` on `exec.CommandContext` calls

`internal/mcp/process.go:34` suppresses both `gosec` and `noctx` on the same line. The `noctx` linter fires on `exec.Command` (no context), not `exec.CommandContext` (which already takes a context). If `CommandContext` is actually used, the `noctx` suppression is stale and can be removed.

- [MODIFY] [process.go](file://../../internal/mcp/process.go)

#### Redundant `gosec` on `client.Do`

`gosec` G107 fires on the URL passed to `http.NewRequestWithContext`, not on `client.Do`. Any `//nolint:gosec` directly on `client.Do` lines is redundant.

- [MODIFY] [viral.go](file://../../internal/plugins/viral/viral.go)
- [MODIFY] [imagegen.go](file://../../internal/plugins/imagegen/imagegen.go)

#### Production `errcheck` suppression in `rollback.go`

`internal/loop/rollback.go:88` carries `//nolint:errcheck` in production code. This should be resolved by either propagating the error, logging it at debug level, or using the idiomatic `_ = fn()` pattern if truly best-effort.

- [MODIFY] [rollback.go](file://../../internal/loop/rollback.go)

---

### 4. Suppressions That Are Legitimately Unavoidable (Do Not Touch)

These suppressions are kept with no changes:

- `//nolint:staticcheck` in `cloud.go` for `WithEndpointResolverWithOptions` and `WithCredentialsFile` — these are deprecated AWS SDK options that are still required for mock endpoint compatibility in tests. No non-deprecated alternative exists.
- `//nolint:gosec` tags in test files where `exec.Command` or `os.Open` are used with intentionally dynamic test arguments (clearly noted with test-context comments). These are inherent to the test helpers and cannot be eliminated without fundamentally restructuring the test infrastructure.

---

## Verification Plan

### Automated Tests
- `make lint` — must pass with zero issues and zero suppression warnings after all changes.
- `make check-coverage` — 91% threshold must be maintained. New helper functions must have corresponding unit tests.
- `make all` — full build and test suite must pass clean.

### Manual Verification
- Run `grep -rn "//nolint" --include="*.go" | wc -l` before and after to confirm net reduction.
- Verify that the `gosec` G304, G107, G204 findings are correctly suppressed at the helper level and not at every call site.
- Confirm no regressions in `pw-mcp-viral`, `pw-mcp-imagegen`, `pw-mcp-pdfcheck`, and the `powerword review` command.
