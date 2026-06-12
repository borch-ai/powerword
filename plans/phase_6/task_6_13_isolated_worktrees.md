# plan: Task 6.13: Isolated Execution via Git Worktrees

**Status:** Open (Issue #[TBD])

This task introduces support for running the agent reasoning loop inside an isolated Git worktree. Instead of performing changes, builds, and tests in the user's primary working copy (which can disrupt the user's focus and risk breaking local states on failure), the CLI spins up a temporary Git worktree, copies any uncommitted changes, and runs all agent tasks within that isolated sandbox.

## User Review Required

> [!NOTE]
> **Performance vs. Isolation**:
> Running in a separate worktree is highly secure and non-destructive. However, to keep build times fast, we must selectively symlink untracked caches and dependencies (such as `node_modules/`, `.venv/`, or `.gradle/`) from the main workspace. Users should be able to configure which paths are symlinked.

> [!WARNING]
> **Git Worktree Cleanup**:
> If the process is forcefully killed (e.g., SIGKILL), the worktree and branch might be left behind. The CLI should auto-prune stale `powerword-worktree-*` directories and branches on the next startup.

---

## Proposed Changes

### Configuration Subsystem

#### [MODIFY] [config.go](file://../../pkg/config/config.go)
- Add `IsolatedWorktree bool` (`isolated_worktree`) to `Config` struct.
- Add `WorktreeSymlinks []string` (`worktree_symlinks`) to `Config` struct to specify untracked folders to symlink (e.g., `["node_modules", ".venv"]`).

#### [MODIFY] [root.go](file://../../pkg/config/root.go)
- Add a new CLI flag `--worktree` to run the session inside an isolated worktree.
- Add `--worktree-symlinks` to configure folders to link.

### Worktree Orchestration Layer

#### [NEW] [worktree.go](file://../../internal/loop/worktree.go)
- Implement `WorktreeSession` struct to manage the life-cycle of the temporary worktree:
  - `Path string` (Path to the temporary worktree directory under `.git/powerword/worktrees/<session_id>`).
  - `BranchName string` (Name of the temporary git branch).
  - `OriginalCwd string` (Backup of the original working directory).
- Implement `NewWorktreeSession(ctx context.Context, cfg *config.Config)`:
  - Generate a unique branch name (e.g., `powerword-worktree-<session_id>`).
  - Create a new branch pointing to the current commit: `git branch <branch> HEAD`.
  - Create the git worktree at a temporary path: `git worktree add --checkout <tempPath> <branch>`.
  - If the main repository had uncommitted changes:
    - Generate a temporary patch of current dirty state: `git diff > patch.diff` and `git diff --cached >> patch.diff`.
    - Apply the patch inside the temp worktree directory: `git apply patch.diff`.
  - Create symbolic links for configured directories (e.g., `node_modules/`, `.venv/`) from the main workspace to the temp worktree path to avoid rebuilding/re-installing dependencies.
- Implement `(*WorktreeSession) Execute(ctx context.Context, runFunc func(dir string) error)`:
  - Change the current working directory of the process to the worktree path: `os.Chdir(worktreePath)`.
  - Run the provided execution function.
  - Revert the current working directory back to the original working directory: `os.Chdir(OriginalCwd)`.
- Implement `(*WorktreeSession) CleanUp(ctx context.Context, success bool)`:
  - If `success` is true, extract the diff generated inside the worktree and apply it back to the main repository, or cherry-pick the commits if any were made.
  - Force remove the worktree: `git worktree remove --force <tempPath>`.
  - Delete the temporary branch: `git branch -D <branch>`.
  - Prune worktree references: `git worktree prune`.

### Core Execution Loop Integration

#### [MODIFY] [loop.go](file://../../internal/loop/loop.go)
- In `RunLoop`:
  - If `cfg.IsolatedWorktree` is enabled, wrap the execution inside `NewWorktreeSession`.
  - Direct all file readers/writers, command execution, and MCP servers to work inside the temp worktree directory.

#### [MODIFY] [repair.go](file://../../internal/review/repair.go)
- In `RunAutonomousLoop`:
  - Support running the repair iterations inside the isolated worktree session if configured, ensuring the repository's main copy is never altered during failures.

---

## Verification Plan

### Automated Tests
- Create `internal/loop/worktree_test.go`:
  - Initialize a temporary Git repository.
  - Mock config files and write files to the repository.
  - Initialize a `WorktreeSession` and verify the temporary directory is created, branch is checked out, and symlinks are set up.
  - Verify that applying uncommitted draft edits copies files correctly to the worktree.
  - Assert that calling `CleanUp` removes the worktree and temp branch cleanly.
- Run command: `go test -v ./internal/loop/... -run TestWorktree`

### Manual Verification
- Run `powerword run "implement test function" --worktree --worktree-symlinks node_modules` on a project.
- Check that a new temporary directory is created in `.git/powerword/worktrees/`, dependencies are accessible via symlinks, and the agent operates successfully without affecting the main working copy.
