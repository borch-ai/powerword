# plan: Task 3.3: Kubernetes Diagnostician Plugin (K8s)

**Status:** Open (Issue #TBD)

This task implements a native Go-based MCP server (`pw-mcp-k8s`) that integrates with Kubernetes cluster contexts to let the agent inspect namespaces, describe resources, fetch pod logs, and perform basic troubleshooting commands.

## User Review Required

> [!IMPORTANT]
> The Kubernetes client-go library (`k8s.io/client-go`) is a heavy dependency tree. We will need to include it in our dependencies or build a lightweight HTTP-based wrapper calling the raw Kubernetes API. We propose using standard `k8s.io/client-go` and `k8s.io/apimachinery` for maximum compatibility and robust parsing of local kubeconfig files.

## Proposed Changes

### K8s Plugin Component
Create a new directory `internal/plugins/k8s/` to contain the native Go plugin implementation.

#### [NEW] [k8s.go](file:///Users/human/code/powerword/internal/plugins/k8s/k8s.go)
- [ ] Initialize Kubernetes in-cluster config or load local default kubeconfig from `~/.kube/config`.
- [ ] Register the following MCP tools:
  - `k8s_list_pods`: list all pods in a namespace (or all namespaces).
  - `k8s_describe_pod`: fetch detailed status and events for a specific pod.
  - `k8s_get_logs`: stream or fetch tail logs from a pod container.
  - `k8s_list_events`: check cluster events to diagnose crashloops or provisioning errors.

#### [NEW] [k8s_test.go](file:///Users/human/code/powerword/internal/plugins/k8s/k8s_test.go)
- [ ] Mock the Kubernetes clientset interfaces using `k8s.io/client-go/kubernetes/fake` to verify tool registrations and log parsing functions without a live cluster.

### CLI Manifest Integration
#### [MODIFY] [internal/config/config.go](file:///Users/human/code/powerword/internal/config/config.go)
- [ ] Mount the native K8s plugin directly as a built-in server option when a configuration key `[plugins.k8s]` is set in `powerword.toml`.

---

## Verification Plan

### Automated Tests
- [ ] Run `go test ./internal/plugins/k8s/...` to verify correct behavior against fake clientsets.
- [ ] Coverage threshold: ensure the package meets or exceeds the 91% coverage criteria.

### Manual Verification
- [ ] Define local kubeconfig with access to a local development cluster (e.g. Minikube or Kind).
- [ ] Run `powerword "check the status of pods in the default namespace"` and verify the agent successfully uses the `k8s_list_pods` tool and presents a table of pods.
