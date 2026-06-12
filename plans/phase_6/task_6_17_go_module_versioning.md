# plan: Task 6.17: Go Module Versioning & Automated Tagging

**Status:** Completed
**Go Version:** 1.26
**Date Completed:** 2026-06-12
**Unit Test Coverage:** N/A (Documentation & GitHub Actions only)

Configure the Powerword release process to automatically and reliably create semantic version tags (`vX.Y.Z`) on the repository. Additionally, document the workflow for downstream private Go consumers (like Pithos) to resolve and download these versions securely via the Go module ecosystem.

## User Review Required

None required.

## Open Questions
- Are we satisfied with relying on the existing `go-semantic-release` action to generate the `vX.Y.Z` tags, or do we want to explicitly use a GitHub CLI step to force tag creation if it proves unreliable?
  - *Answer:* We explicitly added a `git tag` and `git push` step after `go-semantic-release` to ensure the tag is properly pushed to origin, as Go modules strictly depend on these tags.
- For the downstream `pithos` pipeline, do we want to implement the GitHub App token authentication at the `go env` / `git config` level so we can completely drop the `replace` directive in `pithos/go.mod`?
  - *Answer:* Yes, we documented the configuration needed (`git config --global url...insteadOf` and `GOPRIVATE`) to authenticate with GitHub and drop the `replace` directive.

## Proposed Changes

### GitHub Actions Workflow Updates

#### [MODIFY] [release.yml](../../.github/workflows/release.yml)
- Verify the behavior of `go-semantic-release/action@v1`. By default, this action generates tags, but we should explicitly configure it with a `.semrel` file or additional flags if it isn't pushing git tags to the remote.
- If `go-semantic-release` does not reliably push tags, add an explicit step to push the tag using the GitHub CLI:
  ```yaml
  - name: Push Git Tag
    if: steps.semrel.outputs.version != ''
    run: |
      git tag ${{ steps.semrel.outputs.version }}
      git push origin ${{ steps.semrel.outputs.version }}
  ```

### Documentation & Downstream Configuration

#### [NEW] [go_module_resolution.md](../../docs/go_module_resolution.md)
- Create documentation explaining how downstream repositories in the `borch-ai` organization can depend on Powerword.
- Document the requirement to set the Go environment variable:
  ```bash
  go env -w GOPRIVATE="github.com/borch-ai/*"
  ```
- Document the Git configuration needed to authenticate module fetching (useful for CI/CD like `pithos`):
  ```bash
  git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/borch-ai/".insteadOf "https://github.com/borch-ai/"
  ```

---

## Verification Plan

### Automated Tests
- N/A - This is a deployment/infrastructure change.

### Manual Verification
- Merge a test PR into `powerword` using Conventional Commits (e.g. `fix: test tag` or `feat: new tag`).
- Observe the GitHub Actions release workflow.
- Verify that a tag formatted as `vX.Y.Z` appears under the "Tags" tab in GitHub.
- In `pithos` (locally), run `go get github.com/borch-ai/powerword@vX.Y.Z` (with `GOPRIVATE` configured) and ensure it downloads the module successfully.
