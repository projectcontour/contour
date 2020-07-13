ORG = projectcontour
PROJECT = contour
MODULE = github.com/$(ORG)/$(PROJECT)
REGISTRY ?= projectcontour
IMAGE := $(REGISTRY)/$(PROJECT)
SRCDIRS := ./cmd ./internal ./apis
LOCAL_BOOTSTRAP_CONFIG = localenvoyconfig.yaml
SECURE_LOCAL_BOOTSTRAP_CONFIG = securelocalenvoyconfig.yaml
PHONY = gencerts

# The version of Jekyll is pinned in site/Gemfile.lock.
# https://docs.netlify.com/configure-builds/common-configurations/#jekyll
JEKYLL_IMAGE := jekyll/jekyll:4
JEKYLL_PORT := 4000
JEKYLL_LIVERELOAD_PORT := 35729

TAG_LATEST ?= false
# Used to supply a local Envoy docker container an IP to connect to that is running
# 'contour serve'. On MacOS this will work, but may not on other OSes. Defining
# LOCALIP as an env var before running 'make local' will solve that.
LOCALIP ?= $(shell ifconfig | grep inet | grep -v '::' | grep -v 127.0.0.1 | head -n1 | awk '{print $$2}')

# Sets GIT_REF to a tag if it's present, otherwise the short git sha will be used.
GIT_REF = $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short=8 --verify HEAD)
VERSION ?= $(GIT_REF)
# Used for the tag-latest action.
# The tag-latest action will be a noop unless this is explicitly
# set outside this Makefile, as a safety valve.
LATEST_VERSION ?= NOLATEST

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

GO_TAGS := -tags "oidc gcp"
GO_LDFLAGS := -s -w $(patsubst %,-X %, $(GO_BUILD_VARS))

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

install: ## Build and install the contour binary
	go install -mod=readonly -v -ldflags="$(GO_LDFLAGS)" $(GO_TAGS) $(MODULE)/cmd/contour

race:
	go install -mod=readonly -v -race $(GO_TAGS) $(MODULE)/cmd/contour

download: ## Download Go modules
	go mod download

container: ## Build the Contour container image
	docker build \
		--build-arg "BUILD_VERSION=$(BUILD_VERSION)" \
		--build-arg "BUILD_BRANCH=$(BUILD_BRANCH)" \
		--build-arg "BUILD_SHA=$(BUILD_SHA)" \
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

tag-latest: ## Tag the Docker registry container image at $LATEST_VERSION as :latest
ifeq ($(LATEST_VERSION), NOLATEST)
	@echo "LATEST_VERSION not set, not proceeding"
else
	docker pull $(IMAGE):$(LATEST_VERSION)
	docker tag $(IMAGE):$(LATEST_VERSION) $(IMAGE):latest
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
lint: lint-golint lint-yamllint lint-flags lint-misspell

.PHONY: check-misspell
lint-misspell:
	@echo Running spell checker ...
	@go run github.com/client9/misspell/cmd/misspell \
		-locale US \
		-error \
		-i mitre,Mitre,cancelled \
		-source=text \
		$$(git ls-files | grep -E '(md|html)$$')

.PHONY: check-golint
lint-golint:
	@echo Running Go linter ...
	@./hack/golangci-lint run

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
generate: generate-rbac generate-crd-deepcopy generate-deployment generate-crd-yaml generate-api-docs generate-metrics-docs

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
	@echo Generating CRD YAML documents ...
	@./hack/generate-crd-yaml.sh

.PHONY: generate-api-docs
generate-api-docs:
	@echo Generating API documentation ...
	@./hack/generate-api-docs.sh

.PHONY: generate-metrics-docs
generate-metrics-docs:
	@echo Generating metrics documentation ...
	@cd site/_metrics && rm -f *.md && go run ../../hack/generate-metrics-doc.go

.PHONY: check-generate
check-generate: generate
	@./hack/travis/check-uncommitted-codegen.sh

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
		docker.io/envoyproxy/envoy:v1.12.2 \
		envoy \
		--config-path /config/$< \
		--service-node node0 \
		--service-cluster cluster0

local: $(LOCAL_BOOTSTRAP_CONFIG)
	docker run \
		-it \
		--mount type=bind,source=$(CURDIR),target=/config \
		--net bridge \
		docker.io/envoyproxy/envoy:v1.12.2 \
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

.PHONY: site-devel
site-devel: ## Launch the website in a Docker container
	docker run --rm -p $(JEKYLL_PORT):$(JEKYLL_PORT) -p $(JEKYLL_LIVERELOAD_PORT):$(JEKYLL_LIVERELOAD_PORT) -v $$(pwd)/site:/site -it $(JEKYLL_IMAGE) \
		bash -c "cd /site && bundle install --path bundler/cache && bundle exec jekyll serve --host 0.0.0.0 --port $(JEKYLL_PORT) --livereload_port $(JEKYLL_LIVERELOAD_PORT) --livereload"

.PHONY: site-check
site-check: ## Test the site's links
	docker run --rm -v $$(pwd)/site:/site -it $(JEKYLL_IMAGE) \
		bash -c "cd /site && bundle install --path bundler/cache && bundle exec jekyll build && htmlproofer --assume-extension /site/_site"

help: ## Display this help
	@echo Contour high performance Ingress controller for Kubernetes
	@echo
	@echo Targets:
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9._-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort
