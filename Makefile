PROJECT = contour
REGISTRY ?= gcr.io/heptio-images
IMAGE := $(REGISTRY)/$(PROJECT)
PKGS := $(shell go list ./...)

GIT_REF = $(shell git rev-parse --short=8 --verify HEAD)
VERSION ?= $(GIT_REF)

test: install
	go test ./...

vet: | test
	go vet ./...

check: test vet staticcheck unused
	@echo Checking code is gofmted
	@bash -c 'if [ -n "$(gofmt -s -l .)" ]; then echo "Go code is not formatted:"; gofmt -s -d -e .; exit 1;fi'
	@echo Checking rendered files are up to date
	@(cd deployment && bash render.sh && git diff --exit-code . || (echo "rendered files are out of date" && exit 1))

install:
	go install -v -tags "oidc gcp" ./...

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
	unused $(PKGS)

