# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Contour is a Kubernetes ingress controller that deploys Envoy proxy as a reverse proxy and load balancer. Written in Go, it watches Kubernetes configuration resources and translates them to Envoy xDS configuration dynamically. Contour supports three configuration APIs: Kubernetes Ingress, HTTPProxy (custom CRD), and Gateway API.

## Tech Stack

- **Language**: Go 1.25.2
- **Build**: Make, Docker
- **Testing**: Ginkgo v2 (E2E), Testify, Go native testing
- **APIs**: Kubernetes Ingress, HTTPProxy CRD, Gateway API v1.3.0
- **Proxy**: Envoy 1.35.2

## Development Commands

### Building
```bash
make build                      # Build contour binary locally
go build ./cmd/contour          # Alternative: build without make
make container                  # Build Docker image
make multiarch-build           # Build multi-arch container image
```

### Testing
```bash
# Unit tests
go test ./...                   # Run all unit tests
go test .                       # Run tests for current package only
make check-test                 # Run unit tests (via make)
make check-test-race           # Run with race detector

# Pre-submit validation
make checkall                   # Run all pre-submit checks (build, test, lint, generate)

# Feature tests (integration)
go test ./internal/featuretests/...

# E2E tests
make e2e                        # Run E2E tests
make gateway-conformance        # Run Gateway API conformance tests
make upgrade                    # Run upgrade tests
```

### Code Quality
```bash
make lint                       # Run golangci-lint
make format                     # Format code with gofumpt
make generate                   # Generate code (CRDs, deepcopy, mocks)
```

### Local Development
```bash
# Deploy to local Kind cluster
make install-contour-working       # Deploy Contour to Kind
make install-provisioner-working   # Deploy Gateway provisioner to Kind
make cleanup-kind                  # Clean up Kind cluster

# Alternative: Run Contour locally with Envoy in Kind
kind create cluster --config=./examples/kind/kind-expose-port.yaml --name=contour
kubectl apply -f examples/contour
# Edit Envoy DaemonSet to point to your local IP, then:
make install && contour serve --kubeconfig=$HOME/.kube/config --xds-address=0.0.0.0 --insecure
```

## Architecture

### Core Components

**Contour (Management Server)**
- Watches Kubernetes API for Ingress, HTTPProxy, and Gateway API resources
- Translates Kubernetes config to internal DAG representation
- Converts DAG to Envoy xDS configuration
- Serves xDS configuration to Envoy via gRPC
- Deployed as Deployment with leader election

**Envoy (Data Plane)**
- High-performance L7 proxy handling actual traffic
- Configured by Contour via xDS protocol (CDS, RDS, EDS, LDS)
- Deployed as DaemonSet or Deployment

### Processing Pipeline

```
Kubernetes Resources (HTTPProxy/Ingress/Gateway API)
    ↓
Watchers/Informers (internal/k8s/)
    ↓
EventHandler (internal/contour/)
    ↓
DAG Builder (internal/dag/)
    ↓
xDS Cache (internal/xdscache/)
    ↓
Envoy Config (internal/envoy/)
    ↓
xDS Server (internal/xds/)
    ↓
Envoy Proxy
```

### Key Packages

- **`internal/dag/`** - Core DAG (Directed Acyclic Graph) representation and builder. Converts Kubernetes resources to internal graph structure.
- **`internal/envoy/`** - Envoy configuration generation. Translates DAG to Envoy xDS resources.
- **`internal/xdscache/`** - xDS cache management. Maintains snapshots of Envoy configuration.
- **`internal/k8s/`** - Kubernetes client wrappers, informers, and resource watching logic.
- **`internal/contour/`** - Main EventHandler that orchestrates DAG building and cache updates.
- **`internal/xds/`** - xDS server implementation (gRPC).
- **`internal/provisioner/`** - Gateway API provisioner for dynamic Envoy/Contour deployment.
- **`internal/status/`** - Status updater for Kubernetes resources (requires leader election).
- **`internal/featuretests/`** - Integration tests verifying Kubernetes → Envoy config translation.
- **`apis/projectcontour/`** - HTTPProxy and other CRD definitions.
- **`cmd/contour/`** - CLI entry points (serve, bootstrap, etc.).

### Test Architecture

The test suite covers the full pipeline:

1. **Unit tests**: Test individual functions/packages
2. **DAG tests** (`internal/dag/*_test.go`): Test Kubernetes → DAG conversion
3. **Envoy tests** (`internal/envoy/*_test.go`): Test DAG → Envoy config conversion
4. **xDS cache tests** (`internal/xdscache/*_test.go`): Test cache snapshots
5. **Feature tests** (`internal/featuretests/`): Integration tests using full event handler and xDS server
6. **E2E tests** (`test/e2e/`): Full cluster tests with real HTTP traffic

Changes to the core pipeline should include tests at multiple levels.

## Code Conventions

### Import Aliases
Follow the pattern `thing_version` for import aliases:

```go
contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
```

See `.golangci.yml` for enforced import aliases.

### Code Formatting
- Use `gofumpt` (stricter than `gofmt`)
- Run `make format` before committing
- CI enforces formatting via `make lint`

### Commit Messages
```
<package>: <imperative mood description>

<detailed explanation>

Fixes #NNNN

Signed-off-by: Your Name <email@example.com>
```

Example:
```
internal/dag: Add support for request header policies

Implements request header modification in HTTPProxy routes
by building appropriate HeaderValueOptions in the DAG.

Fixes #1234

Signed-off-by: Your Name <email@example.com>
```

### Pull Request Requirements
- **DCO sign-off required**: Use `git commit --signoff`
- **Changelog**: Create `changelogs/unreleased/PR#-githubID-category.md`
- **Label**: Add `release-note/category` label (major/minor/small/docs/infra/not-required)
- **Issue reference**: Include `Fixes #NNNN` or `Updates #NNNN`
- **Pre-submit**: Run `make checkall` before submitting

## Leader Election

Only the leader instance writes status updates to Kubernetes resources. When debugging status issues, check which pod is the leader.

## Gateway API Provisioner

The provisioner dynamically creates Contour+Envoy instances per Gateway resource. It runs as a separate deployment and watches GatewayClass/Gateway resources.

## Debugging

Contour exposes:
- Debug endpoint: `/debug/pprof`
- Health checks: `/healthz`
- Metrics: Prometheus format on `:8000/metrics`

Use `kubectl port-forward` to access debug endpoints in cluster deployments.

## Configuration

Contour configuration is primarily via:
1. **ContourConfiguration CRD**: Cluster-wide settings
2. **Command-line flags**: Override defaults
3. **ConfigMap**: Legacy configuration method

Key config areas: xDS server address, metrics binding, health check paths, feature flags, and Gateway API controller settings.
