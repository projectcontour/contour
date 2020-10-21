# Contour integration tests

For now, run integration tests manually.

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

To run the integration tests, you will need to install
[Integration Tester for Kubernetes](https://github.com/projectcontour/integration-tester)
on your development machine.
Use the [run-test-case.sh](./run-test-case.sh) script to actually run the tests.
The test output can be verbose, so prefer to run one at a time.
This script assumes that HTTP and HTTPS ports are forwarded by the
[make-kind-cluster.sh](./make-kind-cluster.sh) script.

The tests for the HTTPProxy API are in the [httpproxy](./httpproxy)
directory, with one feature tested per test document. The
[fixtures](./fixtures) and [policies](./policies) directories contain YAML
fixtures and Rego helpers respectively.

