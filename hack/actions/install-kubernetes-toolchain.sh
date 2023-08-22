#! /usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

readonly KUBECTL_VERS="v1.28.0"
readonly KIND_VERS="v0.20.0"
readonly SONOBUOY_VERS="0.19.0"

readonly PROGNAME=$(basename $0)
readonly CURL=${CURL:-curl}

# Google storage is case sensitive, so we we need to lowercase the OS.
readonly OS=$(uname | tr '[:upper:]' '[:lower:]')

usage() {
  echo "Usage: $PROGNAME INSTALLDIR"
}

download() {
    local -r url="$1"
    local -r target="$2"

    echo Downloading "$target" from "$url"
    ${CURL} --progress-bar --location  --output "$target" "$url"
}

case "$#" in
  "1")
    mkdir -p "$1"
    readonly DESTDIR=$(cd "$1" && pwd)
    ;;
  *)
    usage
    exit 64
    ;;
esac

echo "Installing Kubernetes toolchain..."

download \
   "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERS}/kind-${OS}-amd64" \
   "${DESTDIR}/kind"

chmod +x  "${DESTDIR}/kind"

download \
    "https://storage.googleapis.com/kubernetes-release/release/${KUBECTL_VERS}/bin/${OS}/amd64/kubectl" \
    "${DESTDIR}/kubectl"

chmod +x "${DESTDIR}/kubectl"

download \
    "https://github.com/vmware-tanzu/sonobuoy/releases/download/v${SONOBUOY_VERS}/sonobuoy_${SONOBUOY_VERS}_linux_amd64.tar.gz" \
    "${DESTDIR}/sonobuoy.tgz"

tar -C "${DESTDIR}" -xf "${DESTDIR}/sonobuoy.tgz" sonobuoy
rm "${DESTDIR}/sonobuoy.tgz"
