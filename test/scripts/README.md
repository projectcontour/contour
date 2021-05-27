# Contour E2E tests

## Cluster setup
The [make-kind-cluster.sh](./make-kind-cluster.sh) script will bring up
a local kind cluster. This underlying VM [config](./kind-expose-port.yaml)
forwards the Envoy ports 80 and 443 locally as port 9080 and 9443.
The script installs [cert-manager](https://cert-manager.io), which is
needed for tests that use TLS.

The [install-contour-working.sh](.install-contour-working.sh) script
builds and installs Contour from the working repository.

The [install-contour-release.sh](.install-contour-release.sh) script
installs a specified Contour release. This is useful for doing upgrade
testing. For example:

```bash
$ ./install-contour-release.sh v1.9.0
...
```

## Running tests

To run the tests, it's best to install [ginkgo](https://onsi.github.io/ginkgo/) on your development machine.

To run a single test (spec):
```
ginkgo -tags=e2e -r -v -focus "001-required-field-validation" ./test/e2e
```

To run all tests for a given API:
```
ginkgo -tags=e2e -keepGoing -randomizeSuites -randomizeAllSpecs -r -v ./test/e2e/httpproxy
```

To run all tests for all APIs:
```
ginkgo -tags=e2e -keepGoing -randomizeSuites -randomizeAllSpecs -r -v ./test/e2e
```
