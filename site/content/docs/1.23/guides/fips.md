---
title: FIPS 140-2 in Contour
---

The [Federal Information Processing Standard (FIPS) 140-2][0] publication describes United States government approved security requirements for cryptographic modules.
Software that is validated by an accredited [Cryptographic Module Validation Program (CVMP) laboratory][1] can be suitable for use in applications for US governmental departments or in industries subject to US Federal regulations.

As a full application is not often tested by a CVMP laboratory, we cannot say that Contour is FIPS validated.
Rather, Contour can be built and configured in a manner that adheres to the standards FIPS 140-2 establishes.

For a fully FIPS compliant deployment of Contour a few things are required:
- Contour must be compiled with a FIPS validated cryptographic module
- Envoy must be compiled with a FIPS validated cryptographic module
- Contour must be configured to use FIPS approved cryptographic algorithms

This guide will run through an example of the process for building and configuring Contour and Envoy for adherence to FIPS 140-2.
Specifically, we will show how Contour and Envoy can be built with the FIPS validated BoringCrypto module of BoringSSL and configured to use FIPS approved TLS ciphers.

Please note that this guide makes no guarantees about Contour FIPS 140-2 approval, validation, or the like.
Interested parties should still evaluate the processes given as example here and the suitability for their purposes.
The Contour project does not have any plans to distribute any binaries compiled in the manner described by this guide.

## Notes on BoringCrypto

This guide shows how Contour and Envoy can be built with [BoringSSL][2] as the cryptographic module.
BoringSSL is Google's fork of OpenSSL and as a whole is not FIPS validated, but a specific core library called BoringCrypto is.
For more detailed information about BoringCrypto see [this document][3].

We are using BoringSSL/BoringCrypto in this example because Contour is written in Go and there is an open source [BoringCrypto flavor of Go][4] readily available.
In addition, Envoy uses BoringSSL at its core and already has well defined build processes for building in a FIPS compliant mode.

One could possibly perform the same sort of operations with another library with FIPS 140-2 a validated cryptographic module (e.g. OpenSSL).
However, that is out of the scope of this guide and interested users will have to come up with their own solutions for that use case, possibly using this document as a template.

## Building Contour

In this section we will describe how the [`projectcontour/contour`][5] container image can be compiled and linked to BoringCrypto for FIPS compliance.
We will be modifying the standard build process by setting up some dependencies and passing additional arguments to the same `make` target used to build the standard, non-FIPS image distributed by the project.

You will need some software downloaded and installed on the computer you are performing the Contour FIPS build on:
- Contour source code checked out to the version you would like to build
- [GNU Make][6]
- [Docker][7]

The Contour [Dockerfile][8] uses a multistage build that performs compilation in an image that contains the necessary build tools and dependencies and then exports compiled artifacts to a final image.
In order to minimize the `projectcontour/contour` image footprint, the final output image only consists of a single layer, containing a lone file: the statically compiled `contour` binary.
The standard Contour build uses the upstream `golang` image as a build base, however we will have to swap that out to build Contour with BoringCrypto.

### Go 1.19 and higher

Starting with Go 1.19, you can simply add [`BUILD_GOEXPERIMENT=boringcrypto`][18] and some related arguments to enable integrating BoringCrypto for standard Go.

```bash
make container \
  BUILD_GOEXPERIMENT=boringcrypto \
  BUILD_CGO_ENABLED=1 \
  BUILD_EXTRA_GO_LDFLAGS="-linkmode=external -extldflags=-static"
```

### Go 1.18 and lower

For the Go version under 1.19, we can use the Google-provided Go implementation that has patches on top of standard Go to enable integrating BoringCrypto.
This is available to us in the [`goboring/golang`][9] container image we can use as a build base. 
Note that the latest version of  [`goboring/golang`][9] image on the Docker hub is `1.16.7b7`, find more versions [here][19] and pull the images on Google Artifact Registry following [this document][20].

In addition, to ensure we can statically compile the `contour` binary when it is linked with the BoringCrypto C library, we must pass some additional arguments to the `make container` target.

To perform the Contour image build with BoringCrypto, change directories to where you have the Contour source code checked out and run the following (replacing `<goboring-version-tag>` with the appropriate version of Go and BoringCrypto, see [here][10] for version specifics):

```bash
make container BUILD_CGO_ENABLED=1 BUILD_BASE_IMAGE=goboring/golang:<goboring-version-tag> BUILD_EXTRA_GO_LDFLAGS="-linkmode=external -extldflags=-static"
```

The command above can be broken down as follows:
- `make container` invokes the container image build target
- `BUILD_CGO_ENABLED=1` ensures `cgo` is enabled in the Contour compilation process
- `BUILD_BASE_IMAGE=goboring/golang:<goboring-version-tag>` ensures we use the BoringCrypto flavor of Go
- `BUILD_EXTRA_GO_LDFLAGS` contains the additional linker flags we need to perform a static build
  - `-linkmode=external` tells the Go linker to use an external linker
  - `-extldflags=-static"` passes the `-static` flag to the external link to ensure a statically linked executable is produced

The container image build process should fail before export of the `contour` binary to the final image if the compiled binary is not statically linked.

### Validation

To be fully sure the produced `contour` binary has been compiled with BoringCrypto you must remove the `-s` flag from the base Contour `Makefile` to stop stripping symbols and run through the build process above.
Then you will be able to inspect the `contour` binary with `go tool nm` to check for symbols containing the string `_Cfunc__goboringcrypto_`. 
Also, you can use the program [rsc.io/goversion][21]. It will report the crypto implementation used by a given binary when invoked with the `-crypto` flag.

Once you have a `projectcontour/contour` image built, you can re-tag it if needed, push the image to a registry, and reference it in a Contour deployment to use it!

## Building Envoy

Envoy has support for building in a FIPS compliant mode as [documented here][11].
The upstream project does not distribute a FIPS compliant Envoy container image, but combining the documented process with the processes for building the Envoy executable and container image, we can produce one.

Again we will need the Envoy source code checked out to the version to build and Docker installed on your computer.
The simplest way to build Envoy without having to learn [Bazel][12] and set up a C++ toolchain on your computer is to build using the Envoy build container image which contains the necessary tools pre-installed.
Note that if you do build with FIPS mode outside of the build container, you can only do so on a Linux-amd64 architecture.

We can first compile the Envoy binary by running the following in a `bash` shell from the Envoy source directory:

```bash
BAZEL_BUILD_EXTRA_OPTIONS="--define boringssl=fips" ENVOY_DOCKER_BUILD_DIR=<envoy-output-dir> ./ci/run_envoy_docker.sh './ci/do_ci.sh bazel.release.server_only'
```

Replace `<envoy-output-dir>` with a directory you would like the build output to be placed on your host computer.
Once that build completes, you should have a file named `envoy_binary.tar.gz` in your specified output directory.

If you would like to build an image with Envoy according to your own specifications, you can unpack the resulting tar archive and you will find a stripped Envoy binary in the `build_release_stripped` directory and a unstripped Envoy with debug info in the `build_release` directory.

To build an image matching the canonical Envoy upstream release image ([`envoyproxy/envoy`][13]), run the following:

```bash
# Make ./linux/amd64 directories.
mkdir -p ./linux/amd64
# Untar tar archive from build step.
tar xzf <envoy-output-dir>/envoy_binary.tar.gz -C ./linux/amd64
# Run the Docker image build.
docker build -f ./ci/Dockerfile-envoy .
```

Once you have an image built, you can tag it as needed, push the image to a registry, and use it in an Envoy deployment.

## Configuring TLS Ciphers

Now that we have Contour and Envoy compiled with BoringCrypto, we can turn our attention to ensuring encrypted communication paths in Contour are configured to use FIPS approved cryptographic algorithms.
Using a FIPS flavor of Envoy will do most of the heavy lifting here without any user configuration needed.

The critical communication paths and how they are set up to be FIPS compliant are enumerated below:
- Contour -> k8s API
  - Contour uses [`client-go`][14] to communicate with the k8s API
  - `client-go` uses the default Golang cipher suites configuration
  - When compiled with BoringCrypto Go, this set of ciphers is FIPS compliant and not configurable by users
- Envoy -> Contour xDS Server, extension services, upstream services
  - A FIPS compliant build of Envoy will choose FIPS approved TLS ciphers when negotiating TLS 1.2 as documented [here][15]
  - The set of ciphers is not configurable
- TLS client -> Envoy
  - As of [Contour 1.13.0][16], the ciphers Envoy will accept as a server when negotiating TLS 1.2 are configurable
  - The [default set of ciphers Contour configures][17] includes some ciphers that are not FIPS approved
  - Users must configure FIPS approved ciphers from the list [here][15]

[0]: https://csrc.nist.gov/publications/detail/fips/140/2/final
[1]: https://csrc.nist.gov/projects/testing-laboratories
[2]: https://boringssl.googlesource.com/boringssl/
[3]: https://boringssl.googlesource.com/boringssl/+/master/crypto/fipsmodule/FIPS.md
[4]: https://go.googlesource.com/go/+/dev.boringcrypto/README.boringcrypto.md
[5]: https://hub.docker.com/r/projectcontour/contour
[6]: https://www.gnu.org/software/make/
[7]: https://www.docker.com/
[8]: {{< param github_url >}}/blob/main/Dockerfile
[9]: https://hub.docker.com/r/goboring/golang/
[10]: https://go.googlesource.com/go/+/dev.boringcrypto/misc/boring/README.md#version-strings
[11]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/security/ssl.html#fips-140-2
[12]: https://bazel.build/
[13]: https://hub.docker.com/r/envoyproxy/envoy
[14]: https://github.com/kubernetes/client-go
[15]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/common.proto#envoy-v3-api-field-extensions-transport-sockets-tls-v3-tlsparameters-cipher-suites
[16]: https://github.com/projectcontour/contour/releases/tag/v1.13.0
[17]: https://pkg.go.dev/github.com/projectcontour/contour/pkg/config#pkg-variables
[18]: https://pkg.go.dev/internal/goexperiment@go1.19
[19]: https://go-boringcrypto.storage.googleapis.com/
[20]: https://go.googlesource.com/go/+/dev.boringcrypto/misc/boring/README.md#releases
[21]: https://godoc.org/rsc.io/goversion