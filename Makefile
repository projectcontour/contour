ORG = projectcontour
PROJECT = contour
MODULE = github.com/$(ORG)/$(PROJECT)
REGISTRY ?= projectcontour
IMAGE := $(REGISTRY)/$(PROJECT)
SRCDIRS := ./cmd ./internal ./apis
LOCAL_BOOTSTRAP_CONFIG = localenvoyconfig.yaml
SECURE_LOCAL_BOOTSTRAP_CONFIG = securelocalenvoyconfig.yaml
PHONY = gencerts
ENVOY_IMAGE = docker.io/envoyproxy/envoy:v1.18.3

# The version of Jekyll is pinned in site/Gemfile.lock.
# https://docs.netlify.com/configure-builds/common-configurations/#jekyll
JEKYLL_IMAGE := jekyll/jekyll:4
JEKYLL_PORT := 4000
JEKYLL_LIVERELOAD_PORT := 35729

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
BUILD_BASE_IMAGE ?= golang:1.16.4

# Enable build with CGO.
BUILD_CGO_ENABLED ?= 0

# Used to supply a local Envoy docker container an IP to connect to that is running
# 'contour serve'. On MacOS this will work, but may not on other OSes. Defining
# LOCALIP as an env var before running 'make local' will solve that.
LOCALIP ?= $(shell ifconfig | grep inet | grep -v '::' | grep -v 127.0.0.1 | head -n1 | awk '{print $$2}')

# Sets GIT_REF to a tag if it's present, otherwise the short git sha will be used.
GIT_REF = $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short=8 --verify HEAD)
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
# Sets the current tagged git version.
BUILD_VERSION = $(VERSION)

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
		--build-arg "BUILD_BASE_IMAGE=$(BUILD_BASE_IMAGE)" \
		--build-arg "BUILD_VERSION=$(BUILD_VERSION)" \
		--build-arg "BUILD_BRANCH=$(BUILD_BRANCH)" \
		--build-arg "BUILD_SHA=$(BUILD_SHA)" \
		--build-arg "BUILD_CGO_ENABLED=$(BUILD_CGO_ENABLED)" \
		--build-arg "BUILD_EXTRA_GO_LDFLAGS=$(BUILD_EXTRA_GO_LDFLAGS)" \
		$(DOCKER_BUILD_LABELS) \
		$(IMAGE_TAGS) \
		$(shell pwd)

container: ## Build the Contour container image
	docker build \
		--build-arg "BUILD_BASE_IMAGE=$(BUILD_BASE_IMAGE)" \
		--build-arg "BUILD_VERSION=$(BUILD_VERSION)" \
		--build-arg "BUILD_BRANCH=$(BUILD_BRANCH)" \
		--build-arg "BUILD_SHA=$(BUILD_SHA)" \
		--build-arg "BUILD_CGO_ENABLED=$(BUILD_CGO_ENABLED)" \
		--build-arg "BUILD_EXTRA_GO_LDFLAGS=$(BUILD_EXTRA_GO_LDFLAGS)" \
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
		-coverpkg=./cmd/...,./internal/... \
		$(MODULE)/...
	@go tool cover -html=coverage.out -o coverage.html

.PHONY: lint
lint: ## Run lint checks
lint: lint-golint lint-yamllint lint-flags lint-codespell

.PHONY: lint-codespell
lint-codespell: CODESPELL_SKIP := $(shell cat .codespell.skip | tr \\n ',')
lint-codespell:
	@./hack/codespell.sh --skip $(CODESPELL_SKIP) --ignore-words .codespell.ignorewords --check-filenames --check-hidden -q2

.PHONY: check-golint
lint-golint:
	@echo Running Go linter ...
	@./hack/golangci-lint run --build-tags=e2e

.PHONY: check-yamllint
lint-yamllint:
	@echo Running YAML linter ...
	@./hack/yamllint examples/ site/examples/

# Check that CLI flags are formatted consistently. We are checking
# for calls to Kingping Flags() and Command() APIs where the 2nd
# argument (the help text) either doesn't start with a capital letter
# or doesn't end with a period. "xDS" and "gRPC" are exceptions to
# the first rule.
.PHONY: check-flags
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
generate: generate-rbac generate-crd-deepcopy generate-crd-yaml generate-deployment generate-api-docs generate-metrics-docs generate-uml

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
	@./hack/generate-deployment.sh

.PHONY: generate-crd-yaml
generate-crd-yaml:
	@echo "Generating CRD YAML documents..."
	@./hack/generate-crd-yaml.sh

.PHONY: generate-api-docs
generate-api-docs:
	@echo "Generating API documentation..."
	@./hack/generate-api-docs.sh github.com/projectcontour/contour/apis/projectcontour

.PHONY: generate-metrics-docs
generate-metrics-docs:
	@echo Generating metrics documentation ...
	@cd site/_metrics && rm -f *.md && go run ../../hack/generate-metrics-doc.go

.PHONY: check-generate
check-generate: generate
	@./hack/actions/check-uncommitted-codegen.sh

# TODO(youngnick): Move these local bootstrap config files out of the repo root dir.
$(LOCAL_BOOTSTRAP_CONFIG): install
	contour bootstrap --xds-address $(LOCALIP) --xds-port=8001 $@

$(SECURE_LOCAL_BOOTSTRAP_CONFIG): install
	contour bootstrap --xds-address $(LOCALIP) --xds-port=8001 --envoy-cafile /config/certs/CAcert.pem --envoy-cert-file /config/certs/envoycert.pem --envoy-key-file /config/certs/envoykey.pem $@

secure-local: $(SECURE_LOCAL_BOOTSTRAP_CONFIG)
	docker run \
		-it \
		--mount type=bind,source=$(CURDIR),target=/config \
		--net bridge \
		$(ENVOY_IMAGE) \
		envoy \
		--config-path /config/$< \
		--service-node node0 \
		--service-cluster cluster0

local: $(LOCAL_BOOTSTRAP_CONFIG)
	docker run \
		-it \
		--mount type=bind,source=$(CURDIR),target=/config \
		--net bridge \
		$(ENVOY_IMAGE) \
		envoy \
		--config-path /config/$< \
		--service-node node0 \
		--service-cluster cluster0

gencerts: certs/contourcert.pem certs/envoycert.pem
	@echo "certs are generated."

applycerts: gencerts
	@kubectl create secret -n projectcontour generic cacert --from-file=./certs/CAcert.pem
	@kubectl create secret -n projectcontour tls contourcert --key=./certs/contourkey.pem --cert=./certs/contourcert.pem
	@kubectl create secret -n projectcontour tls envoycert --key=./certs/envoykey.pem --cert=./certs/envoycert.pem

cleancerts:
	@kubectl delete secret -n projectcontour cacert contourcert envoycert

certs:
	@mkdir -p certs

certs/CAkey.pem: | certs
	@echo No CA keypair present, generating
	openssl req -x509 -new -nodes -keyout certs/CAkey.pem \
		-sha256 -days 1825 -out certs/CAcert.pem \
		-subj "/O=Project Contour/CN=Contour CA"

certs/contourkey.pem:
	@echo Generating new contour key
	openssl genrsa -out certs/contourkey.pem 2048

certs/contourcert.pem: certs/CAkey.pem certs/contourkey.pem
	@echo Generating new contour cert
	openssl req -new -key certs/contourkey.pem \
		-out certs/contour.csr \
		-subj "/O=Project Contour/CN=contour"
	openssl x509 -req -in certs/contour.csr \
		-CA certs/CAcert.pem \
		-CAkey certs/CAkey.pem \
		-CAcreateserial \
		-out certs/contourcert.pem \
		-days 1825 -sha256 \
		-extfile _integration/cert-contour.ext

certs/envoykey.pem:
	@echo Generating new Envoy key
	openssl genrsa -out certs/envoykey.pem 2048

certs/envoycert.pem: certs/CAkey.pem certs/envoykey.pem
	@echo generating new Envoy Cert
	openssl req -new -key certs/envoykey.pem \
		-out certs/envoy.csr \
		-subj "/O=Project Contour/CN=envoy"
	openssl x509 -req -in certs/envoy.csr \
		-CA certs/CAcert.pem \
		-CAkey certs/CAkey.pem \
		-CAcreateserial \
		-out certs/envoycert.pem \
		-days 1825 -sha256 \
		-extfile _integration/cert-envoy.ext

generate-uml: $(patsubst %.uml,%.png,$(wildcard site/img/uml/*.uml))

# Generate a PNG from a PlantUML specification. This rule should only
# trigger when someone updates the UML and that person needs to have
# PlantUML installed.
%.png: %.uml
	cd `dirname $@` && plantuml `basename "$^"`

.PHONY: site-devel
site-devel: ## Launch the website in a Docker container
	docker run --rm -p $(JEKYLL_PORT):$(JEKYLL_PORT) -p $(JEKYLL_LIVERELOAD_PORT):$(JEKYLL_LIVERELOAD_PORT) -v $$(pwd)/site:/site -it $(JEKYLL_IMAGE) \
		bash -c "cd /site && bundle install --path bundler/cache && bundle exec jekyll serve --host 0.0.0.0 --port $(JEKYLL_PORT) --livereload_port $(JEKYLL_LIVERELOAD_PORT) --livereload --force_polling --incremental"

.PHONY: site-check
site-check: ## Test the site's links
	docker run --rm -v $$(pwd):/src -it $(JEKYLL_IMAGE) bash -c "cd /src && ./hack/site-proofing/cibuild"

check-integration:
	@integration-tester --version > /dev/null 2>&1 || (echo "ERROR: To run the integration tests, you will need to install integration-tester (https://github.com/projectcontour/integration-tester)"; exit 1)

.PHONY: integration ## Run integration tests against a real k8s cluster
integration: check-integration
	./_integration/testsuite/make-kind-cluster.sh
	./_integration/testsuite/install-contour-working.sh
	./_integration/testsuite/run-test-case.sh ./_integration/testsuite/ingress/*.yaml ./_integration/testsuite/httpproxy/*.yaml ./_integration/testsuite/gatewayapi/*.yaml
	./_integration/testsuite/cleanup.sh

.PHONY: e2e
e2e:
	./_integration/testsuite/make-kind-cluster.sh
	./_integration/testsuite/install-contour-working.sh
	ginkgo -tags=e2e -mod=readonly -keepGoing -randomizeSuites -randomizeAllSpecs -slowSpecThreshold=15 -r -v ./test/e2e
	./_integration/testsuite/cleanup.sh

check-ingress-conformance: ## Run Ingress controller conformance
	./_integration/testsuite/make-kind-cluster.sh
	./_integration/testsuite/install-contour-working.sh
	./_integration/testsuite/run-ingress-conformance.sh
	./_integration/testsuite/cleanup.sh

help: ## Display this help
	@echo Contour high performance Ingress controller for Kubernetes
	@echo
	@echo Targets:
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9._-]+:.*?## / {printf "  %-25s %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort
