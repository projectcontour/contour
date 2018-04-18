test: install
	go test ./...

vet: | test
	go vet ./...

check: test vet
	@echo Checking code is gofmted
	@bash -c 'if [ -n "$(gofmt -s -l .)" ]; then echo "Go code is not formatted:"; gofmt -s -d -e .; exit 1;fi'
	@echo Checking rendered files are up to date
	@(cd deployment && bash render.sh && git diff --exit-code . || (echo "rendered files are out of date" && exit 1))

install:
	go install -v -tags "oidc gcp" ./...

