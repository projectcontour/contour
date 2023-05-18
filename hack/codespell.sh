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
		ghcr.io/codespell-project/actions-codespell/stable:v1.0 "$@"
fi

cat <<EOF
Unable to run codespell. Please check installation instructions:
	https://github.com/codespell-project/codespell#installation
EOF

exit 69 # EX_UNAVAILABLE
