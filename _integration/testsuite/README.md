# Contour integration tests

For now, run integration tests manually.

The [make-kind-cluster.sh](./make-kind-cluster.sh) script will bring up
a local kind cluster. It will build Contour from the working repository
and install it, along with [cert-manager](https://cert-manager.io),
which is needed for tests that use TLS.

You will need to install the [Integration Tester for Kubernetes](https://github.com/projectcontour/integration-tester).

To run the tests, use the [run-test-case.sh](./run-test-case.sh)
script. The test output can be verbose, so prefer to run one at a time.

The tests for the HTTPProxy API are in the [httpproxy](./httpproxy)
directory, with one feature tested per test document. The
[fixtures](./fixtures) and [policies](./policies) directories contain YAML
fixtures and Rego helpers respectively.

