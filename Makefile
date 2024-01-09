ORG = projectcontour
PROJECT = contour
MODULE = github.com/$(ORG)/$(PROJECT)
REGISTRY ?= ghcr.io/projectcontour
IMAGE := $(REGISTRY)/$(PROJECT)
SRCDIRS := ./cmd ./internal ./apis
LOCAL_BOOTSTRAP_CONFIG = localenvoyconfig.yaml
SECURE_LOCAL_BOOTSTRAP_CONFIG = securelocalenvoyconfig.yaml
ENVOY_IMAGE = docker.io/envoyproxy/envoy:v1.28.0
GATEWAY_API_VERSION ?= $(shell grep "sigs.k8s.io/gateway-api" go.mod | awk '{print $$2}')

# Used to supply a local Envoy docker container an IP to connect to that is running
# 'contour serve'. On MacOS this will work, but may not on other OSes. Defining
# LOCALIP as an env var before running 'make local' will solve that.
LOCALIP ?= $(shell ifconfig | grep inet | grep -v '::' | grep -v 127.0.0.1 | head -n1 | awk '{print $$2}')

# Variables needed for running e2e tests.
CONTOUR_E2E_LOCAL_HOST ?= $(LOCALIP)
# Variables needed for running e2e and upgrade tests.
CONTOUR_UPGRADE_FROM_VERSION ?= $(shell ./test/scripts/get-contour-upgrade-from-version.sh)
CONTOUR_E2E_IMAGE ?= ghcr.io/projectcontour/contour:main
CONTOUR_E2E_PACKAGE_FOCUS ?= ./test/e2e
# Optional variables
# Run specific test specs (matched by regex)
CONTOUR_E2E_TEST_FOCUS ?=

TAG_LATEST ?= false

ifeq ($(TAG_LATEST), true)
	IMAGE_TAGS = \
		--tag $(IMAGE):$(VERSION) \
		--tag $(IMAGE):latest
else
	IMAGE_TAGS = \
		--tag $(IMAGE):$(VERSION)
endif

IMAGE_RESULT_FLAG = --output=type=oci,dest=$(shell pwd)/image/contour-$(VERSION).tar
ifeq ($(PUSH_IMAGE), true)
	IMAGE_RESULT_FLAG = --push
endif

# Platforms to build the multi-arch image for.
IMAGE_PLATFORMS ?= linux/amd64,linux/arm64

# Base build image to use.
BUILD_BASE_IMAGE ?= golang:1.21.5

# Enable build with CGO.
BUILD_CGO_ENABLED ?= 0

# Specify private modules.
BUILD_GOPRIVATE ?= ""

# Go module mirror to use.
BUILD_GOPROXY ?= https://proxy.golang.org

# Checksum db to use.
BUILD_GOSUMDB ?= sum.golang.org

BUILD_GOEXPERIMENT ?= none

# Sets GIT_REF to a tag if it's present, otherwise the short git sha will be used.
GIT_REF = $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short=8 --verify HEAD)
# Used for Contour container image tag.
VERSION ?= $(GIT_REF)

# Stash the ISO 8601 date. Note that the GMT offset is missing the :
# separator, but there doesn't seem to be a way to do that without
# depending on GNU date.
ISO_8601_DATE = $(shell TZ=GMT date '+%Y-%m-%dT%R:%S%z')

# Sets the current Git sha.
BUILD_SHA = $(shell git rev-parse --verify HEAD)
# Sets the current branch. If we are on a detached header, filter it out so the
# branch will be empty. This is similar to --show-current.
BUILD_BRANCH = $(shell git branch | grep -v detached | awk '$$1=="*"{print $$2}')
# Sets the string output by "contour version" and labels on container image.
# Defaults to current tagged git version but can be overridden.
BUILD_VERSION ?= $(VERSION)

GO_BUILD_VARS = \
	github.com/projectcontour/contour/internal/build.Version=${BUILD_VERSION} \
	github.com/projectcontour/contour/internal/build.Sha=${BUILD_SHA} \
	github.com/projectcontour/contour/internal/build.Branch=${BUILD_BRANCH}

GO_TAGS := -tags "oidc gcp osusergo netgo"
GO_LDFLAGS := -s -w $(patsubst %,-X %, $(GO_BUILD_VARS)) $(EXTRA_GO_LDFLAGS)

# Docker labels to be applied to the Contour image. We don't transform
# this with make because it's not worth pulling the tricks needed to handle
# the embedded whitespace.
#
# See https://github.com/opencontainers/image-spec/blob/master/annotations.md
DOCKER_BUILD_LABELS = \
	--label "org.opencontainers.image.created=${ISO_8601_DATE}" \
	--label "org.opencontainers.image.url=https://projectcontour.io/" \
	--label "org.opencontainers.image.documentation=https://projectcontour.io/" \
	--label "org.opencontainers.image.source=https://github.com/projectcontour/contour/archive/${BUILD_VERSION}.tar.gz" \
	--label "org.opencontainers.image.version=${BUILD_VERSION}" \
	--label "org.opencontainers.image.revision=${BUILD_SHA}" \
	--label "org.opencontainers.image.vendor=Project Contour" \
	--label "org.opencontainers.image.licenses=Apache-2.0" \
	--label "org.opencontainers.image.title=Contour" \
	--label "org.opencontainers.image.description=High performance ingress controller for Kubernetes"

export GO111MODULE=on

.PHONY: check
check: install check-test check-test-race ## Install and run tests

.PHONY: checkall
checkall: check lint check-generate

build: ## Build the contour binary
	go build -mod=readonly -v -ldflags="$(GO_LDFLAGS)" $(GO_TAGS) $(MODULE)/cmd/contour

install: ## Build and install the contour binary
	go install -mod=readonly -v -ldflags="$(GO_LDFLAGS)" $(GO_TAGS) $(MODULE)/cmd/contour

race:
	go install -mod=readonly -v -race $(GO_TAGS) $(MODULE)/cmd/contour

download: ## Download Go modules
	go mod download

multiarch-build: ## Build and optionally push a multi-arch Contour container image to the Docker registry
	@mkdir -p $(shell pwd)/image
	docker buildx build $(IMAGE_RESULT_FLAG) \
		--platform $(IMAGE_PLATFORMS) \
		--build-arg "BUILD_GOPRIVATE=$(BUILD_GOPRIVATE)" \
		--build-arg "BUILD_GOPROXY=$(BUILD_GOPROXY)" \
		--build-arg "BUILD_GOSUMDB=$(BUILD_GOSUMDB)" \
		--build-arg "BUILD_BASE_IMAGE=$(BUILD_BASE_IMAGE)" \
		--build-arg "BUILD_VERSION=$(BUILD_VERSION)" \
		--build-arg "BUILD_BRANCH=$(BUILD_BRANCH)" \
		--build-arg "BUILD_SHA=$(BUILD_SHA)" \
		--build-arg "BUILD_CGO_ENABLED=$(BUILD_CGO_ENABLED)" \
		--build-arg "BUILD_EXTRA_GO_LDFLAGS=$(BUILD_EXTRA_GO_LDFLAGS)" \
		--build-arg "BUILD_GOEXPERIMENT=$(BUILD_GOEXPERIMENT)" \
		$(DOCKER_BUILD_LABELS) \
		$(IMAGE_TAGS) \
		$(shell pwd)

container: ## Build the Contour container image
	docker build \
		--build-arg "BUILD_GOPRIVATE=$(BUILD_GOPRIVATE)" \
		--build-arg "BUILD_GOPROXY=$(BUILD_GOPROXY)" \
		--build-arg "BUILD_GOSUMDB=$(BUILD_GOSUMDB)" \
		--build-arg "BUILD_BASE_IMAGE=$(BUILD_BASE_IMAGE)" \
		--build-arg "BUILD_VERSION=$(BUILD_VERSION)" \
		--build-arg "BUILD_BRANCH=$(BUILD_BRANCH)" \
		--build-arg "BUILD_SHA=$(BUILD_SHA)" \
		--build-arg "BUILD_CGO_ENABLED=$(BUILD_CGO_ENABLED)" \
		--build-arg "BUILD_EXTRA_GO_LDFLAGS=$(BUILD_EXTRA_GO_LDFLAGS)" \
		--build-arg "BUILD_GOEXPERIMENT=$(BUILD_GOEXPERIMENT)" \
		$(DOCKER_BUILD_LABELS) \
		$(shell pwd) \
		--tag $(IMAGE):$(VERSION)

push: ## Push the Contour container image to the Docker registry
push: container
	docker push $(IMAGE):$(VERSION)
ifeq ($(TAG_LATEST), true)
	docker tag $(IMAGE):$(VERSION) $(IMAGE):latest
	docker push $(IMAGE):latest
endif

.PHONY: check-test
check-test:
	go test $(GO_TAGS) -cover -mod=readonly $(MODULE)/...

.PHONY: check-test-race
check-test-race: | check-test
	go test $(GO_TAGS) -race -mod=readonly $(MODULE)/...

.PHONY: check-coverage
check-coverage: ## Run tests to generate code coverage
	@go test \
		$(GO_TAGS) \
		-race \
		-mod=readonly \
		-covermode=atomic \
		-coverprofile=coverage.out \
		-coverpkg=./cmd/...,./internal/...,./pkg/... \
		$(MODULE)/...
	@go tool cover -html=coverage.out -o coverage.html

.PHONY: lint
lint: ## Run lint checks
lint: lint-golint lint-yamllint lint-flags lint-codespell

.PHONY: lint-codespell
lint-codespell: CODESPELL_SKIP := $(shell cat .codespell.skip | tr \\n ',')
lint-codespell:
	@./hack/codespell.sh --skip $(CODESPELL_SKIP) --ignore-words .codespell.ignorewords --check-filenames --check-hidden -q2

# TODO: re-enable linting tools package once https://github.com/projectcontour/contour/issues/5077
# is resolved
.PHONY: lint-golint
lint-golint:
	@echo Running Go linter ...
	@./hack/golangci-lint run --build-tags=e2e,conformance,gcp,oidc,none

.PHONY: lint-yamllint
lint-yamllint:
	@echo Running YAML linter ...
	@./hack/yamllint examples/ site/content/examples/ ./versions.yaml

# Check that CLI flags are formatted consistently. We are checking
# for calls to Kingpin Flags() and Command() APIs where the 2nd
# argument (the help text) either doesn't start with a capital letter
# or doesn't end with a period. "xDS" and "gRPC" are exceptions to
# the first rule.
.PHONY: lint-flags
lint-flags:
	@if git --no-pager grep --extended-regexp '[.]Flag\("[^"]+", "([^A-Zxg][^"]+|[^"]+[^.])"' cmd/contour; then \
		echo "ERROR: CLI flag help strings must start with a capital and end with a period."; \
		exit 2; \
	fi
	@if git --no-pager grep --extended-regexp '[.]Command\("[^"]+", "([^A-Z][^"]+|[^"]+[^.])"' cmd/contour; then \
		echo "ERROR: CLI flag help strings must start with a capital and end with a period."; \
		exit 2; \
	fi

.PHONY: generate
generate: ## Re-generate generated code and documentation
generate: generate-rbac generate-crd-deepcopy generate-crd-yaml generate-gateway-yaml generate-deployment generate-api-docs generate-metrics-docs generate-uml generate-go

.PHONY: generate-rbac
generate-rbac:
	@echo Updating generated RBAC policy...
	@./hack/generate-rbac.sh

.PHONY: generate-crd-deepcopy
generate-crd-deepcopy:
	@echo Updating generated CRD deep-copy API code ...
	@./hack/generate-crd-deepcopy.sh

.PHONY: generate-deployment
generate-deployment:
	@echo Generating example deployment files ...
	@./hack/generate-deployment.sh deployment
	@./hack/generate-deployment.sh daemonset
	@./hack/generate-gateway-deployment.sh
	@./hack/generate-provisioner-deployment.sh

.PHONY: generate-crd-yaml
generate-crd-yaml:
	@echo "Generating Contour CRD YAML documents..."
	@./hack/generate-crd-yaml.sh

.PHONY: generate-gateway-yaml
generate-gateway-yaml:
	@echo "Generating Gateway API CRD YAML documents..."
	@GATEWAY_API_VERSION=$(GATEWAY_API_VERSION) ./hack/generate-gateway-yaml.sh


.PHONY: generate-api-docs
generate-api-docs:
	@echo "Generating API documentation..."
	@./hack/generate-api-docs.sh github.com/projectcontour/contour/apis/projectcontour

.PHONY: generate-metrics-docs
generate-metrics-docs:
	@echo "Generating metrics documentation..."
	@cd site/content/docs/main/guides/metrics && rm -f *.md && go run ../../../../../../hack/generate-metrics-doc.go

.PHONY: generate-go
generate-go:
	@echo "Generating mocks..."
	@go run github.com/vektra/mockery/v2

.PHONY: check-generate
check-generate: generate
	@./hack/actions/check-uncommitted-codegen.sh

# Site development targets

generate-uml: $(patsubst %.uml,%.png,$(wildcard site/img/uml/*.uml))

# Generate a PNG from a PlantUML specification. This rule should only
# trigger when someone updates the UML and that person needs to have
# PlantUML installed.
%.png: %.uml
	cd `dirname $@` && plantuml `basename "$^"`

.PHONY: site-devel
site-devel: ## Launch the website
	cd site && hugo serve

.PHONY: site-check
site-check: ## Test the site's links
	# TODO: Clean up to use htmltest


# Tools for testing and troubleshooting

.PHONY: setup-kind-cluster
setup-kind-cluster: ## Make a kind cluster for testing
	./test/scripts/make-kind-cluster.sh

.PHONY: install-contour-working
install-contour-working: | setup-kind-cluster ## Install the local working directory version of Contour into a kind cluster
	./test/scripts/install-contour-working.sh

.PHONY: install-contour-release
install-contour-release: | setup-kind-cluster ## Install the release version of Contour in CONTOUR_UPGRADE_FROM_VERSION, defaults to latest
	./test/scripts/install-contour-release.sh $(CONTOUR_UPGRADE_FROM_VERSION)

.PHONY: install-provisioner-working
install-provisioner-working: | setup-kind-cluster ## Set up the Contour provisioner for local testing
	./test/scripts/install-provisioner-working.sh

.PHONY: install-provisioner-release
install-provisioner-release: | setup-kind-cluster ## Install the release version of the Contour gateway provisioner in CONTOUR_UPGRADE_FROM_VERSION, defaults to latest
	./test/scripts/install-provisioner-release.sh $(CONTOUR_UPGRADE_FROM_VERSION)


.PHONY: e2e
e2e: | setup-kind-cluster load-contour-image-kind run-e2e cleanup-kind ## Run E2E tests against a real k8s cluster

.PHONY: run-e2e
run-e2e:
	CONTOUR_E2E_LOCAL_HOST=$(CONTOUR_E2E_LOCAL_HOST) \
	CONTOUR_E2E_IMAGE=$(CONTOUR_E2E_IMAGE) \
	go run github.com/onsi/ginkgo/v2/ginkgo -tags=e2e -mod=readonly -skip-package=upgrade,bench -keep-going -randomize-suites -randomize-all -poll-progress-after=120s --focus '$(CONTOUR_E2E_TEST_FOCUS)' -r $(CONTOUR_E2E_PACKAGE_FOCUS)

.PHONY: cleanup-kind
cleanup-kind:
	./test/scripts/cleanup.sh

## Loads contour image into kind cluster specified by CLUSTERNAME (default
## contour-e2e). By default for local development will build the current
## working contour source and load into the cluster. If LOAD_PREBUILT_IMAGE
## is specified and set to true, it will load a pre-build image. This requires
## the multiarch-build target to have been run which puts the Contour docker
## image at <repo>/image/contour-version.tar.gz. This second option is chosen
## in CI to speed up builds.
.PHONY: load-contour-image-kind
load-contour-image-kind: ## Load Contour image from building working source or pre-built image into Kind.
	./test/scripts/kind-load-contour-image.sh

.PHONY: upgrade
upgrade: | setup-kind-cluster load-contour-image-kind run-upgrade cleanup-kind ## Run upgrade tests against a real k8s cluster

.PHONY: run-upgrade
run-upgrade:
	CONTOUR_UPGRADE_FROM_VERSION=$(CONTOUR_UPGRADE_FROM_VERSION) \
		CONTOUR_E2E_IMAGE=$(CONTOUR_E2E_IMAGE) \
		go run github.com/onsi/ginkgo/v2/ginkgo -tags=e2e -mod=readonly -randomize-all -poll-progress-after=300s -v ./test/e2e/upgrade

.PHONY: check-ingress-conformance
check-ingress-conformance: | install-contour-working run-ingress-conformance cleanup-kind ## Run Ingress controller conformance

.PHONY: run-ingress-conformance
run-ingress-conformance:
	./test/scripts/run-ingress-conformance.sh

.PHONY: gateway-conformance
gateway-conformance: | setup-kind-cluster load-contour-image-kind run-gateway-conformance cleanup-kind ## Setup a kind cluster and run Gateway API conformance tests in it.

.PHONY: run-gateway-conformance
run-gateway-conformance: ## Run Gateway API conformance tests against the current cluster.
	GATEWAY_API_VERSION=$(GATEWAY_API_VERSION) ./test/scripts/run-gateway-conformance.sh

.PHONY: deploy-gcp-bench-cluster
deploy-gcp-bench-cluster:
	./test/scripts/gcp-bench-cluster.sh deploy

.PHONY: teardown-gcp-bench-cluster
teardown-gcp-bench-cluster:
	./test/scripts/gcp-bench-cluster.sh teardown

.PHONY: run-bench
run-bench:
	go run github.com/onsi/ginkgo/v2/ginkgo -tags=e2e -mod=readonly -keep-going -randomize-suites -randomize-all -poll-progress-after=4h -timeout=5h -r -v ./test/e2e/bench

.PHONY: bench
bench: deploy-gcp-bench-cluster run-bench teardown-gcp-bench-cluster

help: ## Display this help
	@echo Contour high performance Ingress controller for Kubernetes
	@echo
	@echo Targets:
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9._-]+:.*?## / {printf "  %-25s %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort
