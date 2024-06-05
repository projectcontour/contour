#! /usr/bin/env bash

readonly PROGNAME="codespell"

if command -v ${PROGNAME} >/dev/null; then
	# TODO(jpeach): check this won't self-exec ...
	exec ${PROGNAME} "$@"
fi

if command -v docker >/dev/null; then
	exec docker run \
		--rm \
		--volume $(pwd):/workdir \
		--workdir=/workdir \
		--entrypoint=/usr/local/bin/codespell \
		ghcr.io/codespell-project/actions-codespell/stable:v2.0@sha256:9e7b6311f8126aa7c314b47bcb788f4930acedcc0da9dd00ef715b75b38cff1d "$@"
fi

cat <<EOF
Unable to run codespell. Please check installation instructions:
	https://github.com/codespell-project/codespell#installation
EOF

exit 69 # EX_UNAVAILABLE
