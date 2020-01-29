.PHONY: check-yamllint
check-yamllint:
	docker run --rm -ti -v $(CURDIR):/workdir giantswarm/yamllint examples/ site/examples/

