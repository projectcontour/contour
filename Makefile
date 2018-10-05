PROJECT = contour
REGISTRY ?= gcr.io/heptio-images
IMAGE := $(REGISTRY)/$(PROJECT)
SRCDIRS := ./cmd ./internal ./apis
PKGS := $(shell go list ./cmd/... ./internal/...)

GIT_REF = $(shell git rev-parse --short=8 --verify HEAD)
VERSION ?= $(GIT_REF)

test: install
	go test ./...

test-race: | test
	go test -race ./...

vet: | test
	go vet ./...

check: test test-race vet gofmt staticcheck unused misspell unconvert gosimple ineffassign
	@echo Checking rendered files are up to date
	@(cd deployment && bash render.sh && git diff --exit-code . || (echo "rendered files are out of date" && exit 1))

install:
	go install -v -tags "oidc gcp" ./...

dep:
	dep ensure -vendor-only -v

container:
	docker build . -t $(IMAGE):$(VERSION)

push: container
	docker push $(IMAGE):$(VERSION)
	@if git describe --tags --exact-match >/dev/null 2>&1; \
	then \
		docker tag $(IMAGE):$(VERSION) $(IMAGE):latest; \
		docker push $(IMAGE):latest; \
	fi

staticcheck:
	@go get honnef.co/go/tools/cmd/staticcheck
	staticcheck $(PKGS)

unused:
	@go get honnef.co/go/tools/cmd/unused
	unused -exported $(PKGS)

misspell:
	@go get github.com/client9/misspell/cmd/misspell
	misspell \
		-i clas \
		-locale US \
		-error \
		cmd/* internal/* docs/* design/* *.md

unconvert:
	@go get github.com/mdempsky/unconvert
	unconvert -v $(PKGS)

gosimple:
	@go get honnef.co/go/tools/cmd/gosimple
	gosimple $(PKGS)

ineffassign:
	@go get github.com/gordonklaus/ineffassign
	find $(SRCDIRS) -name '*.go' | xargs ineffassign

pedantic: check unparam errcheck

unparam:
	@go get mvdan.cc/unparam
	unparam ./...

errcheck:
	@go get github.com/kisielk/errcheck
	errcheck $(PKGS)

render:
	@echo Rendering deployment files...
	@(cd deployment && bash render.sh)

updategenerated:
	@echo Updating CRD generated code...
	@(bash hack/update-generated-crd-code.sh)

gofmt:  
	@echo Checking code is gofmted
	@test -z "$(shell gofmt -s -l -d -e $(SRCDIRS) | tee /dev/stderr)"
