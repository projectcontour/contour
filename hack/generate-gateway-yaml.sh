#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Wrap sed to deal with GNU and BSD sed flags.
run::sed() {
    local -r vers="$(sed --version < /dev/null 2>&1 | grep -q GNU && echo gnu || echo bsd)"
    case "$vers" in
        gnu) sed -i "$@" ;;
        *) sed -i '' "$@" ;;
    esac
}


kubectl kustomize -o examples/gateway/00-crds.yaml "github.com/kubernetes-sigs/gateway-api/config/crd/experimental?ref=${GATEWAY_API_VERSION}"
echo "Generating Gateway API webhook documents..."
curl -s -o examples/gateway/00-namespace.yaml https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${GATEWAY_API_VERSION}/config/webhook/0-namespace.yaml
curl -s -o examples/gateway/01-admission_webhook.yaml https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${GATEWAY_API_VERSION}/config/webhook/admission_webhook.yaml
curl -s -o examples/gateway/02-certificate_config.yaml https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/${GATEWAY_API_VERSION}/config/webhook/certificate_config.yaml
