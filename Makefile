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
JEKYLL_IMAGE := jekyll/jekyll:3.8.5
JEKYLL_PORT := 4000

TAG_LATEST ?= false
# Used to supply a local Envoy docker container an IP to connect to that is running
# 'contour serve'. On MacOS this will work, but may not on other OSes. Defining
# LOCALIP as an env var before running 'make local' will solve that.
LOCALIP ?= $(shell ifconfig | grep inet | grep -v '::' | grep -v 127.0.0.1 | head -n1 | awk '{print $$2}')

# Sets GIT_REF to a tag if it's present, otherwise the short rev.
GIT_REF = $(shell git describe --tags || git rev-parse --short=8 --verify HEAD)
VERSION ?= $(GIT_REF)
# Used for the tag-latest action.
# The tag-latest action will be a noop unless this is explicitly
# set outside this Makefile, as a safety valve.
LATEST_VERSION ?= NOLATEST

export GO111MODULE=on

test: install
	go test -mod=readonly $(MODULE)/...

test-race: | test
	go test -race -mod=readonly $(MODULE)/...

vet: | test
	go vet $(MODULE)/...

check: ## Run tests and CI checks
check: test test-race vet gofmt staticcheck misspell unconvert unparam ineffassign yamllint check-stale

.PHONY: check-stale
check-stale: ## Check for stale generated content
check-stale: metrics-docs render rendercrds
	@if git status -s site/_metrics examples/render examples/contour 2>&1 | grep -E -q '^\s+[MADRCU]'; then \
		echo Uncommitted changes in generated sources: ; \
		git status -s site/_metrics examples/render examples/contour; \
		exit 1; \
	fi

install: ## Build and install the contour binary
	go install -mod=readonly -v -tags "oidc gcp" $(MODULE)/cmd/contour

race:
	go install -mod=readonly -v -race -tags "oidc gcp" $(MODULE)/cmd/contour

download: ## Download Go modules
	go mod download

container: ## Build the Contour container image
	docker build . -t $(IMAGE):$(VERSION)

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
		docker.io/envoyproxy/envoy:v1.11.2 \
		envoy \
		--config-path /config/$< \
		--service-node node0 \
		--service-cluster cluster0

local: $(LOCAL_BOOTSTRAP_CONFIG)
	docker run \
		-it \
		--mount type=bind,source=$(CURDIR),target=/config \
		--net bridge \
		docker.io/envoyproxy/envoy:v1.11.2 \
		envoy \
		--config-path /config/$< \
		--service-node node0 \
		--service-cluster cluster0

staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck
	staticcheck \
		-checks all,-ST1003 \
		$(MODULE)/{cmd,internal}/...

misspell:
	go install github.com/client9/misspell/cmd/misspell
	misspell \
		-i clas \
		-locale US \
		-error \
		cmd/* internal/* docs/* design/* site/*.md site/_{guides,posts,resources} *.md

unconvert:
	go install github.com/mdempsky/unconvert
	unconvert -v $(MODULE)/{cmd,internal}/...

ineffassign:
	go install github.com/gordonklaus/ineffassign
	find $(SRCDIRS) -name '*.go' | xargs ineffassign

pedantic: check errcheck

unparam:
	go install mvdan.cc/unparam
	unparam -exported $(MODULE)/{cmd,internal}/...

errcheck:
	go install github.com/kisielk/errcheck
	errcheck $(MODULE)/...

render:
	@echo Rendering example deployment files...
	@(cd examples && bash render.sh)

rendercrds:
	@echo Rendering CRDs...
	@(cd examples && bash rendercrds.sh)

updategenerated: ## Update generated CRD code
	@echo Updating generated CRD code...
	@(bash hack/update-generated-crd-code.sh)

yamllint:
	docker run --rm -ti -v $(CURDIR):/workdir giantswarm/yamllint examples/ site/examples/

gofmt:
	@echo Checking code is gofmted
	@test -z "$(shell gofmt -s -l -d -e $(SRCDIRS) | tee /dev/stderr)"

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
	docker run --publish $(JEKYLL_PORT):$(JEKYLL_PORT) -v $$(pwd)/site:/site -it $(JEKYLL_IMAGE) \
		bash -c "cd /site && bundle install && bundle exec jekyll serve --host 0.0.0.0 --port $(JEKYLL_PORT) --livereload"

.PHONY: metrics-docs
metrics-docs: ## Regenerate documentation for metrics
	@echo Generating metrics documentation...
	@cd site/_metrics && rm -f *.md && go run ../../hack/generate-metrics-doc.go

help: ## Display this help
	@echo Contour high performance Ingress controller for Kubernetes
	@echo
	@echo Targets:
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9._-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort
