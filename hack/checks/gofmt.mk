.PHONY: check-gofmt
check-gofmt:
	@test -z "$(shell gofmt -s -l -d -e $$(find . -type d -depth 1 | grep -v vendor) | tee /dev/stderr)"

