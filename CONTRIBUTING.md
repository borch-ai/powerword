# Contributing to Powerword

Thank you for your interest in contributing to **Powerword**! We welcome pull requests, bug reports, and suggestions. To maintain a secure, high-quality, and robust codebase, please follow the guidelines and rules described below.

---

## Code Quality Standards

Before submitting a pull request, your changes must pass our automated quality checks. You can run these locally using the provided [Makefile](file:///Users/human/code/powerword/Makefile):

1.  **Formatting and Linting**:
    *   Format all Go code: `make fmt`
    *   Verify lint rules: `make lint` (uses `golangci-lint` configured via [`.golangci.yml`](file:///Users/human/code/powerword/.golangci.yml) to check for AST security vulnerabilities, TCP resource body leaks, and proper context usage).
2.  **Vulnerability Scanner**:
    *   Audit third-party modules: `make vuln` (runs `govulncheck` to detect CVEs in packages reached by our code path).
3.  **Strict Unit Test Coverage**:
    *   All new code must be fully unit-tested (suffix `_test.go`).
    *   **We enforce a minimum 91% unit test coverage gate.**
    *   Run tests and verify coverage: `make check-coverage` (runs unit tests and evaluates output via [scripts/check_coverage.go](file:///Users/human/code/powerword/scripts/check_coverage.go)).
4.  **AI Local Critic (Pre-Push)**:
    *   Powerword enforces a `pre-push` hook that evaluates both your unpushed committed changes (relative to `main`) and your local uncommitted working-tree changes against the implementation plan in the GitHub issue.
    *   It first runs `make all` locally. If it passes, it extracts the combined git diff and sends it to the configured Critic LLM for strict verification.
    *   If validation fails or the critic rejects the changes, the push aborts and detailed feedback is written locally to `.powerword-critic.md`.

Run the entire verification pipeline before pushing:
```bash
make
```

---

## Branching & Merging Policy

*   **No Direct Pushes / PR Required**: Direct pushes (including web UI commits) to the remote `main` branch are blocked. All changes must be proposed via a Pull Request.
*   **Force Pushes Blocked**: Force pushing (`git push -f`) is blocked on the `main` branch.
*   **Feature Branches**: Create a dedicated branch for your work matching standard patterns (e.g., `feature/*`, `docs/*`, `bugfix/*`, `chore/*`, `refactor/*`, `test/*`).
*   **Squash Merging**: All pull requests must be **squash merged** into `main`. This keeps the git history linear, clean, and easy to parse. Individual intermediate commits will be consolidated into a single structured commit on merge.

---

## Pull Request Guidelines

1.  **Create an Issue / Plan**: For major changes, write or select an implementation plan under the `plans/` directory to get alignment before starting.
2.  **Make Small, Focused Commits**: Write descriptive commit messages using semantic prefixes where possible:
    *   `feat`: A new user-facing feature.
    *   `fix`: A bug fix.
    *   `docs`: Documentation changes.
    *   `test`: Adding or correcting tests.
    *   `refactor`: Code changes that neither fix a bug nor add a feature.
3.  **Open the PR**: Link the PR to any related GitHub issues.
4.  **CI Gates**: The PR checks will run formatting, lints, vulnerabilities, and unit test coverage. If the unit test coverage falls below **91.00%**, the PR cannot be merged.
5.  **Review**: A minimum of one approving code review is required.

---

## Local Development Setup

To get your system ready for development:

1.  Ensure you have **Go 1.22+** installed.
2.  Ensure you have **`golangci-lint`** installed:
    ```bash
    brew install golangci-lint
    ```
3.  Check and tidy dependencies:
    ```bash
    make tidy
    ```
