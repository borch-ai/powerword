# Powerword Go Module Resolution

Powerword is a private Go module within the `borch-ai` organization. By default, attempting to install or build Powerword using standard Go tools (`go get`, `go build`, etc.) from a downstream project (such as `pithos`) will result in `401 Unauthorized` or `404 Not Found` errors because the Go module proxy and Git lack the necessary credentials.

To successfully use `github.com/borch-ai/powerword` as a dependency, follow these two steps:

## 1. Configure `GOPRIVATE`

Tell Go to bypass the public checksum database and module proxy for all `borch-ai` repositories. Run the following command in your terminal:

```bash
go env -w GOPRIVATE="github.com/borch-ai/*"
```

*Note: This setting is stored in your global Go environment configuration.*

## 2. Authenticate Git via GitHub Token

Since Go uses Git under the hood to fetch private repositories, Git needs to know how to authenticate with GitHub.

Generate a GitHub Personal Access Token (classic or fine-grained) with read access to the repository (`repo` scope). Then, configure Git to automatically inject this token when fetching from `borch-ai`:

```bash
git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/borch-ai/".insteadOf "https://github.com/borch-ai/"
```

*Important:* Replace `${GITHUB_TOKEN}` with your actual token, or ensure the environment variable is set in your CI/CD pipelines when this command is run.

### Summary for CI/CD Pipelines

If you are setting up a GitHub Actions workflow (or any other CI/CD environment), add these steps before running any `go` commands:

```yaml
- name: Authenticate Go for Private Modules
  run: |
    go env -w GOPRIVATE="github.com/borch-ai/*"
    git config --global url."https://${{ secrets.GITHUB_TOKEN }}:x-oauth-basic@github.com/borch-ai/".insteadOf "https://github.com/borch-ai/"
```

*(Note: Depending on the workflow configuration, you may need a Personal Access Token instead of the default `secrets.GITHUB_TOKEN` if cross-repository access is required.)*
