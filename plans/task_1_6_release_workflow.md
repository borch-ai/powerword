# Task 1.6: Automated Release Workflow

Add a post-merge "release" GitHub Action workflow to automate semantic versioning, create Git tags and GitHub releases, generate detailed release notes based on Conventional Commit messages, compile Powerword binaries for multiple platforms, and upload them as release assets.

## User Review Required

> [!IMPORTANT]
> - **Conventional Commits:** The automated release relies on commits pushed to `main` following the Conventional Commits specification (e.g., `feat: ...`, `fix: ...`, `chore: ...`). Commits not conforming to this structure may prevent version increments.
> - **GitHub Token Permissions:** The workflow requires `contents: write` permissions for the automatic `GITHUB_TOKEN` to generate tags and create GitHub Releases.
> - **Workflow Integration:** To avoid duplicating workflows and run releases only on fully tested code, we propose integrating the release job directly into the existing `.github/workflows/ci.yml` file as a post-test job that runs exclusively on pushes to the `main` branch.

## Actual Choices & Configurations

- **Go Version Used**: Go 1.26.4
- **Release Automation Tool**: `go-semantic-release/action@v1`
- **Asset Upload Tool**: GitHub CLI (`gh release upload`) pre-installed on the runner.
- **Compiled Architectures**:
  - macOS amd64 & arm64 (Apple Silicon)
  - Linux amd64 & arm64
  - Windows amd64

## Proposed Changes

### Configuration & CLI Versioning

We need to define the version inside the application and register it with Cobra so that `--version` prints the release version correctly.

#### [NEW] [version.go](file:///Users/human/code/powerword/internal/config/version.go)
- Create `internal/config/version.go` to define the package-level `Version` variable.

#### [MODIFY] [root.go](file:///Users/human/code/powerword/internal/config/root.go)
- Set the `Version` attribute of the Cobra root command using `Version: Version`.

#### [MODIFY] [root_test.go](file:///Users/human/code/powerword/internal/config/root_test.go)
- Add a unit test to verify that `--version` correctly executes and outputs the version of the command, ensuring code coverage remains above the 91% threshold.

---

### Makefile Integration

We need to pass the version variable during compilation dynamically using linker flags (`-ldflags`).

#### [MODIFY] [Makefile](file:///Users/human/code/powerword/Makefile)
- Define a `VERSION` variable defaulting to `dev`.
- Update the `build` target to pass `-ldflags "-X powerword/internal/config.Version=$(VERSION)"`.

---

### GitHub Actions Workflow

We need to add the release logic as a separate workflow.

#### [NEW] [release.yml](file:///Users/human/code/powerword/.github/workflows/release.yml)
- Create a new workflow file `release.yml` that triggers on pushes (merges) to the `main` branch.
- Run all validation gates (linting, testing, and security checks) to ensure release stability.
- Use `go-semantic-release/action` to determine the next version, create the Git tag, and publish the GitHub release with release notes.
- Compile binaries for target platforms (macOS amd64/arm64, Linux amd64/arm64, Windows amd64) using the calculated version.
- Upload the compiled binaries to the newly created GitHub release using the GitHub CLI (`gh`).

---

## Verification Plan

### Automated Tests
- Run `make check-coverage` to verify that all unit tests pass and code coverage is above 91.0%.

### Manual Verification
- Verify that `make build VERSION=v1.2.3` generates a binary that responds with the correct version on `./bin/powerword --version`.
- Verify the local GitHub Action workflow structure via `make lint`.
