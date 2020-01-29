.PHONY: check-ineffassign
check-ineffassign:
	@go run github.com/gordonklaus/ineffassign \
		$$(find . -path ./vendor -prune -o -name '*.go' | grep -v vendor)


