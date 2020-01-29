.PHONY: check-unconvert
check-unconvert:
	@go run github.com/mdempsky/unconvert \
		-v \
		./{cmd,internal}/...


