# Contour Project Architecture Analysis

## Project Overview

**Contour** is a Kubernetes ingress controller that deploys Envoy proxy as a reverse proxy and load balancer. It's a cloud-native, production-grade solution written in Go that provides dynamic configuration updates while maintaining a lightweight profile.

**Primary Purpose**: Watch Kubernetes configuration resources and translate them to Envoy xDS (Discovery Service) configuration dynamically.

## Tech Stack

### Core Technologies

- **Language**: Go 1.24.0
- **Build System**: Make, Docker, Docker Buildx (multi-arch)
- **Proxy Engine**: Envoy 1.35.2
- **Kubernetes**: Client-go, Controller-runtime

### Major Frameworks & Libraries

#### Kubernetes & API Management
- **k8s.io/client-go** (v0.34.1) - Kubernetes client library
- **k8s.io/apimachinery** (v0.34.1) - Kubernetes API machinery
- **sigs.k8s.io/controller-runtime** (v0.22.3) - Controller framework
- **sigs.k8s.io/controller-tools** (v0.19.0) - CRD code generation
- **sigs.k8s.io/gateway-api** (v1.3.0) - Gateway API implementation
- **k8s.io/api** (v0.34.1) - Kubernetes API types

#### Envoy Integration
- **github.com/envoyproxy/go-control-plane** (v0.13.4) - Envoy xDS control plane
- **github.com/envoyproxy/go-control-plane/envoy** (v1.35.0) - Envoy API definitions
- **google.golang.org/grpc** (v1.76.0) - gRPC for xDS communication
- **google.golang.org/protobuf** (v1.36.10) - Protocol buffers

#### Observability & Monitoring
- **github.com/prometheus/client_golang** (v1.23.2) - Prometheus metrics
- **github.com/sirupsen/logrus** (v1.9.3) - Structured logging
- **github.com/bombsimon/logrusr/v4** (v4.1.0) - Logr adapter for logrus
- **github.com/grpc-ecosystem/go-grpc-prometheus** (v1.2.0) - gRPC Prometheus metrics

#### Testing
- **github.com/onsi/ginkgo/v2** (v2.27.1) - BDD testing framework for E2E tests
- **github.com/onsi/gomega** (v1.38.2) - Matcher library for Ginkgo
- **github.com/stretchr/testify** (v1.11.1) - Testing toolkit
- **Go native testing** - Standard library testing

#### Certificate Management
- **github.com/cert-manager/cert-manager** (v1.18.2) - Certificate management integration
- **github.com/tsaarni/certyaml** (v0.10.0) - Certificate generation from YAML

#### CLI & Configuration
- **github.com/alecthomas/kingpin/v2** (v2.4.0) - Command-line flag parser
- **github.com/spf13/cobra** (v1.9.1) - CLI framework
- **github.com/spf13/viper** (v1.20.0) - Configuration management

#### Utilities
- **dario.cat/mergo** (v1.0.2) - Structure merging
- **github.com/google/uuid** (v1.6.0) - UUID generation
- **github.com/pkg/errors** (v0.9.1) - Error handling
- **gopkg.in/yaml.v3** (v3.0.1) - YAML parsing
- **go.uber.org/automaxprocs** (v1.6.0) - Automatic GOMAXPROCS configuration

## Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes API Server                     │
│        (Ingress, HTTPProxy, Gateway API Resources)          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           │ Watch/Inform
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                    Contour (Control Plane)                   │
│  ┌────────────┐    ┌──────────┐    ┌─────────────────┐    │
│  │ Kubernetes │ -> │   DAG    │ -> │  xDS Cache      │    │
│  │ Watchers   │    │ Builder  │    │  & Snapshots    │    │
│  └────────────┘    └──────────┘    └─────────────────┘    │
│                                             │                │
│                                             ▼                │
│                                     ┌──────────────┐        │
│                                     │  xDS Server  │        │
│                                     │   (gRPC)     │        │
│                                     └──────┬───────┘        │
└────────────────────────────────────────────┼────────────────┘
                                              │
                                              │ xDS Protocol (CDS, RDS, EDS, LDS)
                                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Envoy Proxy (Data Plane)                  │
│          High-performance L7 Reverse Proxy                   │
│              (Handles actual HTTP traffic)                   │
└─────────────────────────────────────────────────────────────┘
```

### Configuration APIs Supported

Contour supports three different configuration APIs:

1. **Kubernetes Ingress** - Standard Kubernetes ingress resources
2. **HTTPProxy (CRD)** - Contour's custom resource for advanced features
3. **Gateway API** - Kubernetes SIG-Network's next-generation API

### Core Components

#### 1. Contour (Management/Control Server)
- **Deployment**: Kubernetes Deployment with leader election
- **Responsibilities**:
  - Watch Kubernetes API for configuration resources
  - Build internal DAG (Directed Acyclic Graph) representation
  - Translate DAG to Envoy xDS configuration
  - Serve xDS configuration to Envoy via gRPC
  - Update status on Kubernetes resources (leader only)

#### 2. Envoy (Data Plane)
- **Deployment**: Kubernetes DaemonSet or Deployment
- **Responsibilities**:
  - Handle actual HTTP/HTTPS traffic
  - Apply routing, load balancing, and proxy rules
  - Receive configuration from Contour via xDS

### Processing Pipeline

```
Kubernetes Resources
    ↓
Watchers/Informers (internal/k8s/)
    ↓
EventHandler (internal/contour/)
    ↓
DAG Builder (internal/dag/)
    ↓
xDS Cache (internal/xdscache/)
    ↓
Envoy Config Generation (internal/envoy/)
    ↓
xDS Server (internal/xds/)
    ↓
Envoy Proxy
```

## Code Organization

### Directory Structure

```
contour/
├── cmd/contour/              # CLI entry points
├── apis/projectcontour/      # Custom Resource Definitions (CRDs)
│   ├── v1/                   # HTTPProxy, TLSCertificateDelegation, etc.
│   └── v1alpha1/             # ContourConfiguration, etc.
├── internal/                 # Internal implementation packages
│   ├── k8s/                  # Kubernetes client wrappers & watchers
│   ├── contour/              # Main EventHandler orchestration
│   ├── dag/                  # DAG representation & builder
│   ├── envoy/                # Envoy xDS configuration generation
│   ├── xdscache/             # xDS cache management & snapshots
│   ├── xds/                  # xDS server (gRPC) implementation
│   ├── provisioner/          # Gateway API provisioner
│   ├── status/               # Kubernetes resource status updates
│   ├── gatewayapi/           # Gateway API specific logic
│   ├── featuretests/         # Integration tests
│   ├── metrics/              # Prometheus metrics
│   ├── health/               # Health check handlers
│   ├── debug/                # Debug/pprof endpoints
│   ├── leadership/           # Leader election
│   └── ...
├── pkg/config/               # Configuration structures
├── test/                     # Test suites
│   ├── e2e/                  # End-to-end tests
│   ├── conformance/          # Gateway API conformance tests
│   └── scripts/              # Test helper scripts
├── examples/                 # Deployment examples
├── docs/                     # Documentation
└── design/                   # Design documents
```

### Key Internal Packages

#### **internal/dag/**
- Core DAG (Directed Acyclic Graph) representation
- Converts Kubernetes resources to internal graph structure
- Handles routing logic, TLS configuration, policy application
- Main entry: `Builder` type that orchestrates DAG construction

#### **internal/envoy/**
- Envoy configuration generation
- Translates DAG to Envoy xDS resources (Clusters, Routes, Endpoints, Listeners)
- Version-specific implementations (v3/)

#### **internal/xdscache/**
- xDS cache management
- Maintains snapshots of Envoy configuration
- Handles incremental updates

#### **internal/k8s/**
- Kubernetes client wrappers
- Informer setup and management
- Resource watching logic

#### **internal/contour/**
- Main `EventHandler` that orchestrates:
  - DAG building
  - Cache updates
  - Configuration changes

#### **internal/xds/**
- xDS server implementation
- gRPC server for Envoy communication
- Implements Envoy's xDS protocol

#### **internal/provisioner/**
- Gateway API provisioner
- Dynamically creates Contour+Envoy instances per Gateway resource
- Separate deployment watching GatewayClass/Gateway resources

#### **internal/status/**
- Status updater for Kubernetes resources
- Requires leader election (only leader updates status)

#### **internal/featuretests/**
- Integration tests
- Verify Kubernetes → Envoy config translation
- Use full event handler and xDS server

### API Definitions

#### **apis/projectcontour/v1/**
- HTTPProxy CRD
- TLSCertificateDelegation
- ExtensionService
- ContourDeployment

#### **apis/projectcontour/v1alpha1/**
- ContourConfiguration
- ExtensionService (alpha)

## UI/Frontend Code

### Overview

**Contour is a headless controller with NO admin dashboard or web UI.** All configuration and management happens through Kubernetes APIs and CLI tools (`kubectl`, `contour` CLI).

The only UI-related code in the repository is for the **static documentation website** (https://projectcontour.io), located in the `/site` directory.

### Documentation Website (`/site`)

#### Technology Stack

- **Static Site Generator**: Hugo (Go-based)
- **Template Engine**: Go HTML templates
- **Styling**: SCSS (~1,353 lines total)
- **JavaScript**: Vanilla JavaScript (22 lines)
- **No JavaScript Frameworks**: No React, Vue, Angular, or similar

#### Directory Structure

```
site/
├── config.yaml              # Hugo configuration
├── content/                 # Markdown documentation files
│   ├── docs/                # Versioned documentation (1.20 - 1.33 + main)
│   ├── community/           # Community pages
│   └── resources/           # Resources and guides
├── themes/contour/          # Custom Hugo theme
│   ├── assets/
│   │   └── scss/            # Stylesheets
│   │       ├── _components.scss (846 lines)
│   │       ├── _base.scss (253 lines)
│   │       ├── _header.scss (107 lines)
│   │       ├── _footer.scss (97 lines)
│   │       ├── _mixins.scss (35 lines)
│   │       ├── _variables.scss (10 lines)
│   │       └── site.scss (5 lines)
│   ├── layouts/             # HTML templates
│   │   ├── index.html       # Homepage
│   │   ├── _default/        # Default layouts
│   │   ├── partials/        # Reusable components
│   │   └── shortcodes/      # Hugo shortcodes
│   └── static/
│       ├── js/
│       │   └── main.js      # Minimal JavaScript (22 lines)
│       ├── fonts/           # Metropolis font family
│       └── img/             # SVG icons, logos, diagrams
└── data/                    # Data files for templates
```

#### JavaScript Functionality (main.js)

The JavaScript code is minimal and handles only basic interactions:

```javascript
// Mobile navigation toggle
function mobileNavToggle()

// Documentation version dropdown toggle
function docsVersionToggle()

// Click-outside handler to close dropdown menus
window.onclick - closes dropdown when clicking outside
```

**Total: 22 lines of vanilla JavaScript** - no build process, no transpilation, no frameworks.

#### SCSS Styling

The site uses SCSS for styling with a modular architecture:

- **_components.scss** (846 lines) - Buttons, cards, navigation, forms, etc.
- **_base.scss** (253 lines) - Base styles, typography, layout
- **_header.scss** (107 lines) - Header and navigation styles
- **_footer.scss** (97 lines) - Footer styles
- **_mixins.scss** (35 lines) - Reusable SCSS mixins
- **_variables.scss** (10 lines) - Color and spacing variables

**Total: ~1,353 lines of SCSS**

#### Features

1. **Versioned Documentation**
   - Supports multiple versions (1.20 through 1.33 + main branch)
   - Version dropdown selector
   - Each version has its own content tree

2. **Search Functionality**
   - Integrated with Algolia for full-text search
   - Configuration in `config.yaml`:
     - App ID: `IW9YQMJ8HH`
     - Index: `projectcontour`

3. **Responsive Design**
   - Mobile-friendly navigation with hamburger menu
   - Responsive grid layouts
   - Mobile-first CSS approach

4. **Code Syntax Highlighting**
   - Built-in Hugo syntax highlighting
   - Pygments style for code blocks
   - Support for multiple languages

5. **Auto-Generated API Reference**
   - HTML files generated from CRD definitions
   - Located in `content/docs/*/config/api-reference.html`
   - Covers HTTPProxy, ContourConfiguration, etc.

#### Static Assets

- **Fonts**: Metropolis font family (Regular, Bold, Light, Medium, SemiBold, with Italic variants)
- **Images**: SVG icons, project logos, architecture diagrams, UML diagrams
- **Icons**: GitHub, Slack, Twitter, search, navigation arrows, etc.

#### Build & Development

```bash
# Prerequisites
brew install hugo  # macOS
choco install hugo-extended -confirm  # Windows

# Build and serve locally
hugo server --disableFastRender

# Serves at http://localhost:1313
```

### What's NOT Included

❌ **No Admin Dashboard** - Configuration is done entirely via Kubernetes YAML
❌ **No Metrics Dashboard** - Uses Prometheus/Grafana for observability
❌ **No Control Panel** - All management through `kubectl` and Kubernetes APIs
❌ **No Application UI** - Contour is a backend controller only
❌ **No Modern JS Framework** - Just vanilla JS for simple interactions
❌ **No Build Pipeline** - No webpack, Vite, npm, or node_modules
❌ **No REST API UI** - Debug endpoints are for curl/browser direct access

### Visualization & Dashboards

For runtime visualization, Contour relies on external tools:

- **Prometheus + Grafana** - Metrics and monitoring dashboards (example configs in `/examples/grafana/`)
- **Kubernetes Dashboard** - View HTTPProxy/Gateway resources
- **kubectl** - CLI-based resource inspection
- **Debug Endpoints** - `/debug/pprof` for profiling (text/binary output)

### Summary

The project is **100% backend-focused** with no application UI. The only "UI code" is a standard static documentation website built with Hugo, featuring minimal JavaScript for basic interactions and SCSS for styling. All Contour configuration, monitoring, and management happens through:

1. **Kubernetes APIs** (kubectl, YAML manifests)
2. **Contour CLI** (bootstrap, version, etc.)
3. **Prometheus/Grafana** (metrics visualization)
4. **External tooling** (debuggers, profilers, log aggregators)

This is typical for Kubernetes controllers, which are headless services that operate entirely through declarative configuration.

## Test Architecture

Contour has a comprehensive multi-layered testing strategy:

### 1. Unit Tests
- Test individual functions and packages
- Standard Go testing with testify assertions
- Run with: `go test ./...` or `make check-test`

### 2. DAG Tests
- Location: `internal/dag/*_test.go`
- Purpose: Test Kubernetes → DAG conversion
- Verify routing logic, policy application

### 3. Envoy Config Tests
- Location: `internal/envoy/*_test.go`
- Purpose: Test DAG → Envoy config conversion
- Verify correct xDS resource generation

### 4. xDS Cache Tests
- Location: `internal/xdscache/*_test.go`
- Purpose: Test cache snapshot management

### 5. Feature Tests (Integration)
- Location: `internal/featuretests/`
- Purpose: Full event handler and xDS server integration
- End-to-end pipeline testing without real cluster

### 6. E2E Tests
- Location: `test/e2e/`
- Framework: Ginkgo v2
- Purpose: Full cluster tests with real HTTP traffic
- Runs in Kind cluster

### 7. Gateway API Conformance Tests
- Location: `test/conformance/`
- Purpose: Validate Gateway API compliance
- Run with: `make gateway-conformance`

### 8. Upgrade Tests
- Purpose: Test version upgrades
- Run with: `make upgrade`

## Build System

### Makefile Targets

**Building:**
- `make build` - Build contour binary locally
- `make install` - Build and install contour binary
- `make container` - Build Docker image
- `make multiarch-build` - Build multi-arch container image

**Testing:**
- `make check-test` - Run unit tests
- `make check-test-race` - Run tests with race detector
- `make e2e` - Run E2E tests
- `make gateway-conformance` - Run Gateway API conformance
- `make upgrade` - Run upgrade tests

**Code Quality:**
- `make lint` - Run golangci-lint
- `make format` - Format code with gofumpt
- `make generate` - Generate code (CRDs, deepcopy, mocks)
- `make checkall` - Run all pre-submit checks

**Local Development:**
- `make install-contour-working` - Deploy to local Kind cluster
- `make install-provisioner-working` - Deploy provisioner to Kind
- `make cleanup-kind` - Clean up Kind cluster

## Key Design Patterns

### 1. Controller Pattern
- Uses Kubernetes controller-runtime for resource watching
- Event-driven architecture with informers and caches

### 2. DAG (Directed Acyclic Graph)
- Internal representation of routing configuration
- Enables validation and conflict detection
- Provides clean separation between input (K8s) and output (Envoy)

### 3. xDS Protocol
- Standard Envoy discovery service protocol
- Implements CDS (Cluster), RDS (Route), EDS (Endpoint), LDS (Listener)
- Supports incremental updates

### 4. Leader Election
- Only leader pod writes status updates
- Prevents conflicts in multi-replica deployments
- Uses Kubernetes lease-based leader election

### 5. Versioned APIs
- Import aliases follow `thing_version` pattern:
  - `contour_v1` for Contour APIs
  - `envoy_v3` for Envoy config
  - `gatewayapi_v1` for Gateway API

## Configuration Management

Contour supports multiple configuration methods:

1. **ContourConfiguration CRD** - Cluster-wide settings (highest precedence)
2. **Command-line flags** - Override defaults
3. **ConfigMap** - Legacy configuration method
4. **Environment variables** - Override file-based config

Key configuration areas:
- xDS server address and port
- Metrics binding
- Health check paths
- Feature flags
- Gateway API controller settings
- TLS configuration
- Timeout values

## Observability

### Metrics
- Prometheus format metrics on `:8000/metrics`
- DAG rebuild duration
- xDS update metrics
- HTTP request metrics

### Health Checks
- `/healthz` endpoint
- Liveness and readiness probes

### Debug Endpoints
- `/debug/pprof` - Go profiling
- Access via `kubectl port-forward`

### Logging
- Structured logging with logrus
- Configurable log levels
- JSON output support

## Security Features

- **TLS Termination** - Support for TLS/HTTPS
- **Certificate Management** - Integration with cert-manager
- **RBAC** - Required for Kubernetes operations
- **External Auth** - Support for external authorization
- **Rate Limiting** - Integration with Envoy rate limit service
- **Security Audit** - Third-party audit by Cure53 (Dec 2020)

## Gateway API Provisioner

Separate component for dynamic Gateway management:
- Watches GatewayClass and Gateway resources
- Dynamically creates Contour+Envoy instances per Gateway
- Runs as separate deployment
- Enables multi-tenancy scenarios

## Code Conventions

### Import Aliases
```go
contour_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
contour_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
```

### Code Formatting
- Uses `gofumpt` (stricter than `gofmt`)
- Enforced via `make lint` in CI
- Run `make format` before committing

### Commit Message Format
```
<package>: <imperative mood description>

<detailed explanation>

Fixes #NNNN

Signed-off-by: Your Name <email@example.com>
```

## Performance Considerations

- **Lightweight Profile** - Minimal resource overhead
- **Dynamic Updates** - No Envoy restarts needed
- **Efficient DAG** - O(1) lookups for most operations
- **Incremental xDS** - Only send changed configuration
- **automaxprocs** - Automatic GOMAXPROCS tuning

## Deployment Models

### Standard Deployment
- Contour as Deployment (2+ replicas with leader election)
- Envoy as DaemonSet (one per node)
- Shared configuration via xDS

### Gateway Provisioner
- Dynamic per-Gateway deployments
- Contour + Envoy per Gateway resource
- Multi-tenant isolation

### Local Development
- Kind cluster with exposed ports
- Contour running outside cluster
- Envoy in cluster connecting to local Contour

## Integration Points

- **Kubernetes API Server** - Resource watching
- **Envoy Proxy** - xDS protocol over gRPC
- **Cert-manager** - Certificate provisioning
- **Prometheus** - Metrics collection
- **External Auth Services** - Authentication/authorization
- **Rate Limit Service** - Rate limiting
- **Jaeger/Zipkin** - Distributed tracing

## Community & Governance

- **License**: Apache 2.0
- **Vendor**: Project Contour (CNCF project)
- **Community Meetings**: Regular schedule
- **Communication**: #contour on Kubernetes Slack
- **DCO**: Developer Certificate of Origin required (sign-off commits)
- **Security**: Dedicated security team for vulnerability reports

## Summary

Contour is a sophisticated, production-grade Kubernetes ingress controller with:
- **Modern Architecture**: Event-driven, DAG-based routing
- **Multiple APIs**: Ingress, HTTPProxy, Gateway API
- **Production Ready**: Comprehensive testing, observability, security
- **Cloud Native**: Built on Kubernetes patterns and CNCF tools
- **High Performance**: Envoy-based with efficient configuration updates
- **Active Community**: Strong governance, regular releases, responsive maintainers

The codebase demonstrates excellent software engineering practices with clear separation of concerns, comprehensive testing at multiple levels, and well-documented APIs and processes.
