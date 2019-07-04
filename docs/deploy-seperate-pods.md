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
- The following certificates must be present as Secrets in the `heptio-contour` namespace:
    - `cacert`: must contain a `CAcert.pem` key that contains a CA certificate that signs the other certificates.
    - `contourcerts`: must contain `contourcert.pem` and `contourkey.pem` keys that contain a certificate and key for Contour. The certificate must be valid for the name `contour` either via CN or SAN.
    - `envoycerts`: must contain `envoycert.pem` and `envoykey.pem` keys that contain a certificate and key for Envoy.

## Deploy Contour

1. [Clone the Contour repository][1] and cd into the repo.
2. Ensure you have `openssl` installed (or at least something that provides that command.`)
2. Generate the keypairs required to secure the TLS connection between Contour and Envoy. You can do that using `make gencerts` and then `make applycerts`, assuming that your kubectl will connect to the cluster you want. If not, follow the [detailed generation directions][2]. 
3. Run `kubectl apply -f examples/ds-hostnet-split/`

**NOTE**: The current configuration exposes the `/stats` path from the Envoy Admin UI so that Prometheus can scrape for metrics.

# Test

1. Install a workload (see the kuard example in the [main deployment guide][3]).

[1]: ../CONTRIBUTING.md
[2]: ./grpc-tls-howto.md
[3]: deploy-options.md#test
