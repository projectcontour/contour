.PHONY: check-staticcheck
check-staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck \
		-checks all,-ST1003 \
		./{cmd,internal}/...

