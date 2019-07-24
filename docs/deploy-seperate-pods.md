# Contour Deployment with Split Pods

This is an advanced deployment guide to configure Contour in a Deployment separate from Envoy which allows for easier scaling of each component.
This configuration has several advantages:

1. Envoy runs as a daemonset which allows for distributed scaling across workers in the cluster
2. Envoy runs on host networking which exposes Envoy directly without additional networking hops
3. Communication between Contour and Envoy is secured by mutually-checked self-signed certificates.

## Moving parts

- Contour is run as Deployment and Envoy as a Daemonset
- Envoy runs on host networking
- Envoy runs on ports 80 & 443
- In our example deployment, the following certificates must be present as Secrets in the `heptio-contour` namespace for the example YAMLs to apply:
    - `cacert`: must contain a ` cacert.pem` key that contains a CA certificate that signs the other certificates.
    - `contourcert`: be a Secret of type `kubernetes.io/tls` and must contain `tls.crt` and `tls.key` keys that contain a certificate and key for Contour. The certificate must be valid for the name `contour` either via CN or SAN.
    - `envoycert`: be a Secret of type `kubernetes.io/tls` and must contain `tls.crt` and `tls.key` keys that contain a certificate and key for Envoy.

For detailed instructions on how to configure the required certs manually, see the [step-by-step TLS HOWTO][2].

## Deploy Contour

1. [Clone the Contour repository][1] and cd into the repo.
2. Run `kubectl apply -f examples/ds-hostnet-split/`

This will:
- set up RBAC
- run a Kubernetes Job that will generate one-year validity certs and put them into `heptio-contour`
- Install Contour and Envoy in a separate fashion.

**NOTE**: The current configuration exposes the `/stats` path from the Envoy Admin UI so that Prometheus can scrape for metrics.

# Test

1. Install a workload (see the kuard example in the [main deployment guide][3]).

[1]: ../CONTRIBUTING.md
[2]: ./grpc-tls-howto.md
[3]: deploy-options.md#test
