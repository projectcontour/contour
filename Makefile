PROJECT = contour
REGISTRY ?= gcr.io/heptio-images
IMAGE := $(REGISTRY)/$(PROJECT)
SRCDIRS := ./cmd ./internal ./apis
PKGS := $(shell GO111MODULE=on go list -mod=readonly ./cmd/... ./internal/...)
LOCAL_BOOTSTRAP_CONFIG = localenvoyconfig.yaml
SECURE_LOCAL_BOOTSTRAP_CONFIG = securelocalenvoyconfig.yaml
PHONY = gencerts

TAG_LATEST ?= false
# Used to supply a local Envoy docker container an IP to connect to that is running
# 'contour serve'. On MacOS this will work, but may not on other OSes. Defining
# LOCALIP as an env var before running 'make local' will solve that.
LOCALIP ?= $(shell ifconfig | grep inet | grep -v '::' | grep -v 127.0.0.1 | head -n1 | awk '{print $$2}')

GIT_REF = $(shell git rev-parse --short=8 --verify HEAD)
VERSION ?= $(GIT_REF)

export GO111MODULE=on

test: install
	go test -mod=readonly ./...

test-race: | test
	go test -race -mod=readonly ./...

vet: | test
	go vet ./...

check: test test-race vet gofmt staticcheck misspell unconvert unparam ineffassign
	@echo Checking rendered files are up to date
	@(cd examples && bash render.sh && git diff --exit-code . || (echo "rendered files are out of date" && exit 1))

install:
	go install -mod=readonly -v -tags "oidc gcp" ./...

download:
	go mod download

container:
	docker build . -t $(IMAGE):$(VERSION)

push: container
	docker push $(IMAGE):$(VERSION)
ifeq ($(TAG_LATEST), true)
	docker tag $(IMAGE):$(VERSION) $(IMAGE):latest
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
		docker.io/envoyproxy/envoy:v1.10.0 \
		envoy \
		--config-path /config/$< \
		--service-node node0 \
		--service-cluster cluster0

local: $(LOCAL_BOOTSTRAP_CONFIG)
	docker run \
		-it \
		--mount type=bind,source=$(CURDIR),target=/config \
		--net bridge \
		docker.io/envoyproxy/envoy:v1.10.0 \
		envoy \
		--config-path /config/$< \
		--service-node node0 \
		--service-cluster cluster0

staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck
	staticcheck \
		-checks all,-ST1003 \
		$(PKGS)

misspell:
	go install github.com/client9/misspell/cmd/misspell
	misspell \
		-i clas \
		-locale US \
		-error \
		cmd/* internal/* docs/* design/* *.md

unconvert:
	go install github.com/mdempsky/unconvert
	unconvert -v $(PKGS)

ineffassign:
	go install github.com/gordonklaus/ineffassign
	find $(SRCDIRS) -name '*.go' | xargs ineffassign

pedantic: check errcheck

unparam:
	go install mvdan.cc/unparam
	unparam -exported $(PKGS)

errcheck:
	go install github.com/kisielk/errcheck
	errcheck $(PKGS)

render:
	@echo Rendering example deployment files...
	@(cd examples && bash render.sh)

updategenerated:
	@echo Updating CRD generated code...
	@(bash hack/update-generated-crd-code.sh)

gofmt:
	@echo Checking code is gofmted
	@test -z "$(shell gofmt -s -l -d -e $(SRCDIRS) | tee /dev/stderr)"

gencerts: certs/contourcert.pem certs/envoycert.pem
	@echo "certs are generated."

applycerts: gencerts
	@kubectl create secret -n heptio-contour generic cacert --from-file=./certs/CAcert.pem
	@kubectl create secret -n heptio-contour tls contourcert --key=./certs/contourkey.pem --cert=./certs/contourcert.pem
	@kubectl create secret -n heptio-contour tls envoycert --key=./certs/envoykey.pem --cert=./certs/envoycert.pem

cleancerts:
	@kubectl delete secret -n heptio-contour cacert contourcert envoycert

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
