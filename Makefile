PROJ_NAME=contour
JEKYLL_VERSION=3.5
IMAGE_NAME=doc-test

DIR := ${CURDIR}

test:
	docker run \
	-v $(DIR):/project \
	-e PROJ_NAME=$(PROJ_NAME) \
	gcr.io/heptio-images/$(IMAGE_NAME):latest

serve:
	docker run \
	--rm \
	-v $(DIR):/srv/jekyll \
	-it -p 4000:4000 \
	jekyll/jekyll:$(JEKYLL_VERSION) \
	jekyll serve

clean:
	rm -rf logs/
