# Secure Communication between Contour and Envoy

_Status_: Draft, in review.

This document outlines how to secure the communication between Contour
and Envoy containers.

## Goals

- Achieve secure communication between Contour and Envoy
- Authenticate Envoy connecting to Contour
- Rotating and invalidating the credentials

## Non-goals

- Support for dynamic credentials
- Support for use of Secret Discovery Service for credential management

## Background

Currently, Contour and Envoy containers are co-located within a pod.
Current security model is relying on the fact that Contour exposes the management
server on loopback interface which prevents any unauthorized access from outside
the pod.

With the need to move away from the [current deployment pattern of co-locating
Contour and Envoy][0], it is necessary to secure the communication channel between
the containers.

## High level design

- Use mutual TLS to secure communication channel between Contour and Envoy
- Bootstrap phase of Contour will generate necessary TLS certificates

## Detailed design

### Default options for secure communication

By default, mutual TLS is enabled to secure the communication between Contour and Envoy
containers. The default `contour bootstrap` will generate the bootstrap configuration that
enables secure communication between the containers. If `contour bootstrap` is passed with
the parameter `--disable-tls`, the bootstrap configuration generated will make use of insecure
communication between the containers.

### Bootstrapping process

This bootstrap phase is independent of `contour bootstrap` and uses `contour bootstrap-tls`.
`contour bootstrap-tls` is run as an init container to bootstrap the TLS configuration.
The bootstrap phase will create three pairs of the TLS certificates. The details about
each pair of certificates are mentioned below:
1. The first pair is used by Contour to serve HTTPS.
2. The second pair is used by Envoy to authenticate with Contour.
3. The last pair serves as the CA certificate which signs the above certificates.

Each of these TLS certificates will be stored as Kubernetes secrets in namespace `heptio-contour`. The
secrets will be named as `contour-tls`, `envoy-tls` and `contour-ca`, respectively. This ensures that
even when the `contour` and `envoy` containers are not part of the same pod, the certificates are
available for consumption by each of those.

Note: In order for Contour to create the Kubernetes secrets in the `heptio-contour` namespace, the
existing RBAC needs to be enhanced to grant permissions to `create` secrets in the namespace.

### Setting up secure communication channel

1. Contour container will pick the certificates from the Kubernetes secret `contour-tls` which is
mounted at `/etc/tls/contour` and pick `contour-ca` mounted at `/etc/tls/ca` to start listening for
HTTPS traffic.
2. The bootstrap configuration yaml of Envoy will have [`tls_context`][1] added as part of `cluster` object.
Envoy container will pick the certificates from the Kubernetes secret `envoy-tls` which is mounted at
`/etc/tls/envoy` and pick `contour-ca` mounted at `/etc/tls/ca` to connect to Contour as configured by the
bootstrap configuration file.

### Rotation and invalidation of credentials

This design assumes that to rotate or invalidate the credentials, Contour has to be redeployed which will
regenerate a new set of certificates.

## Future enhancements

As discussed in [issue 881][2], this design uses static credentials and to support dynamic
credentials, [Secret Discovery Service][3] needs to be implemented by Contour. This work
is tracked [here][4].

[0]: https://github.com/heptio/contour/issues/881
[1]: https://www.envoyproxy.io/docs/envoy/v1.9.0/intro/arch_overview/ssl.html#enabling-certificate-verification
[2]: https://github.com/heptio/contour/issues/862#issuecomment-464601450
[3]: https://www.envoyproxy.io/docs/envoy/v1.9.0/configuration/secret
[4]: https://github.com/heptio/contour/issues/898
