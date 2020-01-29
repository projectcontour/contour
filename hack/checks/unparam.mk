.PHONY: check-unparam
check-unparam:
	@go run mvdan.cc/unparam \
		-exported ./{cmd,internal}/...

