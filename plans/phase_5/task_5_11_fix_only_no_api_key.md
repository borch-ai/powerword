# plan: Task 5.11: Allow `review --fix` Without an API Key

**Status:** Completed
**Go Version:** 1.26.4
**Date Completed:** 2026-06-13
**Unit Test Coverage:** 91.20%

`powerword review --fix` auto-corrects plan file formatting (absolute paths → relative,
label normalization, metadata fixes). It performs **no LLM inference** — it only reads
and rewrites `.md` files under `plans/`. Despite this, it currently fails with
`"no API keys found"` when run from a repo that has no `powerword.toml` or
`POWERWORD_*` key variables set.

The root cause: `persistentPreRunE` in `pkg/config/root.go` calls `cfg.Validate()`
(which asserts at least one API key is present) for **every** subcommand, including
`review --fix`. The `--fix` execution path never touches the LLM, so the key
requirement is spurious.

## User Review Required

> [!NOTE]
> Moving `Validate()` calls from `PersistentPreRunE` into subcommand RunE methods that actually require LLM access.

## Root Cause

```
persistentPreRunE
  └─ cfg.Validate()          ← always called, regardless of flags
       └─ error: no API keys
            ← review.RunE never reached
                 └─ fixPlans → FixAbsolutePathsInPlans  (pure file I/O, no LLM)
```

## Proposed Changes

### `pkg/config/root.go`

#### [MODIFY] [root.go](file://../../pkg/config/root.go)

Add a `fixOnly` flag probe to `persistentPreRunE`. When the invoked command is
`review` and the `--fix` flag is set without `--local` or `--issue`, skip
`cfg.Validate()`.

The cleanest approach is to add a package-level exported function `IsFixOnlyMode`
that the pre-run hook can query — avoiding import cycles between `config` and the
`review` command:

```go
// In persistentPreRunE, after LoadConfig:
if !cfg.ListSessions && !isFixOnlyReview(cmd) {
    if err := cfg.Validate(); err != nil {
        return err
    }
}
```

Where `isFixOnlyReview` inspects `cmd.Name() == "review"` and checks that the
`--fix` flag was passed while `--local` and `--issue` were **not** passed.

**Alternative (preferred — simpler):** Move `Validate()` out of
`persistentPreRunE` entirely and call it only inside the subcommands that require
an API key. The `review` command already has its own `RunE`; it can call
`cfg.Validate()` when `fixPlans == false && (localOnly || issueID != "")`.
This is consistent with how `--list-sessions` already skips validation today.

**Decision: prefer the alternative.** Moving `Validate()` to the call sites that
need it is the correct design — `PersistentPreRunE` should load config but not
assert capabilities that only some commands need.

#### Changes to `persistentPreRunE`:

```go
func persistentPreRunE(cmd *cobra.Command, args []string) error {
    // Skip config loading for the bare root command (shows help).
    if cmd.Name() == "powerword" && len(args) == 0 && !listSessions && resumeID == "" && os.Getenv("POWERWORD_RESUME") == "" {
        return nil
    }

    cfg, err := LoadConfig(cfgFile)
    if err != nil {
        return err
    }

    applyFlagOverrides(cmd, cfg)

    // API key validation is now the responsibility of each subcommand that
    // actually requires LLM access. Commands that perform pure local I/O
    // (e.g. review --fix, list-sessions) must not be blocked by missing keys.
    // Legacy path: keep validation here only for the root run command.
    if cmd.Name() == "powerword" && !cfg.ListSessions {
        if err := cfg.Validate(); err != nil {
            return err
        }
    }

    Active = cfg
    return nil
}
```

#### Changes to `cmd/powerword/review.go`:

Call `cfg.Validate()` inside `RunE`, **only** when the command will perform
LLM-backed operations (i.e., when `localOnly` or `issueID != ""`):

```go
RunE: func(cmd *cobra.Command, args []string) error {
    cfg := config.Active
    if cfg == nil {
        return fmt.Errorf("configuration not loaded")
    }

    if fixPlans {
        cmd.Printf("Checking and auto-fixing plan files...\n")
        fixedCount, err := review.FixAbsolutePathsInPlans(".", cfg)
        if err != nil {
            return err
        }
        cmd.Printf("Auto-fix complete. Modified %d plan file(s).\n", fixedCount)
        // If --fix was the only flag, we're done — no LLM step needed.
        if !localOnly && issueID == "" && !listen {
            return nil
        }
    }

    // Beyond this point, an LLM call will be made — validate API keys.
    if err := cfg.Validate(); err != nil {
        return err
    }
    // ... rest of the review logic unchanged
},
```

---

## Verification Plan

### Automated Tests

- `go test ./pkg/config/...` — existing `Validate()` tests still pass
- `go test ./cmd/powerword/...` — review command tests cover:
  - `--fix` alone succeeds without API keys (new test case)
  - `--fix --local` still validates keys (API key required for LLM step)
  - `--fix` alone with API keys present still runs fix correctly

### Manual Verification

1. From kiln repo (which has `POWERWORD_*` keys in `.env` but no `powerword.toml`):
   ```bash
   make fix-plans
   ```
   Should succeed, print `Auto-fix complete. Modified N plan file(s).`

2. From a repo with no powerword config at all:
   ```bash
   powerword review --fix
   ```
   Should succeed (fix runs) even with no API keys.

3. From a repo with no powerword config:
   ```bash
   powerword review --local
   ```
   Should still fail with `"no API keys found"` (LLM step requires keys).

4. `make fix-plans` from powerword itself — unchanged behavior.
