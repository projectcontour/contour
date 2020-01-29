# NOTE: errcheck flags calls to fmt.Fprintf() as errors, so it's too
# pedantic for normal use.

.PHONY: check-errcheck
check-errcheck:
	@go run github.com/kisielk/errcheck ./...

