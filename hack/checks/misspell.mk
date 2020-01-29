.PHONY: check-misspell
check-misspell:
	@go run github.com/client9/misspell/cmd/misspell \
		-i clas \
		-locale US \
		-error \
		cmd/* internal/* design/* site/*.md site/_{guides,posts,resources} site/docs/**/* *.md

