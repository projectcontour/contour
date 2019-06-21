# Secure Communication for Contour and Envoy

Status: Accepted

This document outlines a design for securing gRPC communication between Contour and Envoy.

## Goals

- Achieve secure communication between Contour and Envoy, which will allow a secure split deployment.

## Non Goals

- Certificate management, including creation or rotation. These must be provided by the user. In the future we plan to automated this process to allow split deployments to become the default configuration.

## Definitions

### mTLS

In this document, we use **mTLS** as shorthand for 'mutual TLS client certificate authentication', without implying a key lifetime. That is, we're just talking about both sides having TLS certs, not implying that we're using the more modern form which implies dynamic, short-lifetime certificates managed by an automated mechanism.

## Background

Currently, the default deployment of Contour colocates the Envoy and Contour containers in the same pod communicating over the pod's loopback interface. This uses the pod-level security boundary to provide transport security (no-one can listen in) and authentication (no-one other than Envoy can connect).

In order to be able to split the deployment of Contour and Envoy, we must be able to provide an acceptable level of both transport security and authentication between the different pods.

Currently, if you do split the deployment, Contour will assume that anything that talks to the xDS endpoints is authorized for all information in them. Notably, this includes any certificates used in any endpoint. Antyhing that can speak SDS can connect to contour and ask for all the certificates it knows about via SDS. This design is intended to mitigate this problem.

It's also important to note that the certificates talked about in this document are **only** the certificates used to secure the gRPC channel between Contour and Envoy, not any certificates used to serve TLS out of Envoy. As such, this design is wholly separate to any certificate functionality on either Ingress or IngressRoute.

## High-Level Design

We will add configuration to Contour, both for `contour serve` and `contour bootstrap`, to have Contour and Envoy use a shared CA and a keypair each. This will allow a basic split deployment, using simple Secret mounts into file locations in both a Contour Deployment and Envoy deployment or DaemonSet.

The generation of the certificates and their placement into Secrets is not in scope for this change, only a basic illustration in the example deployment YAMLs and Howto. However, see [Future Work](#future-work) for more info.

## Detailed Design - mTLS

In order for mTLS to work, each end must know:
- The CA that the TLS relationship will descend from.
- Its certificate
- Its key

This applies both to Contour and Envoy.

To accomplish this, we will add the following new command line options:

To `contour serve`, covering the Contour side:
- `--contour-cafile`: the CA Bundle for the mutual TLS relationship
- `--contour-mtls-cert`: The cert for Contour to use to identify itself. (`serve`)
- `--contour-mtls-key`: The key for Contour to use for its side of the mTLS pairing. (`serve`)

(Note that these `contour serve` options may also move to a configuration file under [#1130][1])

And to `contour bootstrap`, covering Envoy configuration:
- `--envoy-cafile`: The CA Bundle for the mutual TLS relationship
- `--envoy-mtls-cert`: The cert for Envoy to use to identify itself.
- `--envoy-mtls-key`: The key for Envoy to use to identify itself.

If any one of the three options is passed to either `serve` or `bootstrap`, the other two are required, and we will error out if they are not passed.

We will wire these options into the gRPC server establishment process on the Contour side, and supply them in the bootstrap on the Envoy side. This is where the most significant code changes will be required.

We will update the example deployment manifests for the separate deployment options to use these new options, and provide a basic sample howto on generating a CA keypair and mTLS keypairs using command line tools like openssl.

## Security Considerations

In the current design, the process must be restarted to pick up a change to these certs. Because of this, they should be long-lived certs (days, weeks, or months), not short-lived (minutes or hours). The CA Keypair should be very long-lived and very tightly controlled, as Contour/Envoy connection security is only as secure as the CA keypair.

As noted above, this is an initial design to *start* the process of being able to run reasonably secure, authenticated, separate deployments of Contour and Envoy. Some things that we *will* address at a later date, but are out of scope for now:

- Certificate generation (apart from a very simple howto to allow users to see how the feature works)
- Certificate rotation

Implementing this change will, however, enable a secure split-deployment scenario.

## Future Work

This work is the first phase of a planned three-phase rollout.

### Phase 1: mTLS

This document - allows a more secure split deployment in a very basic way.

### Phase 2: User Experience

This phase will add additional functionality and/or tools to help with the certificate generation and rolling process. Watch [#1184][2] to track this phase.

### Phase 3: Secure by default

This phase will flip the example deployments to all use a secure mode, make those command line options default, and include an option to restore the current behavior. Watch [#1185][3] to track this phase.

[1]: https://github.com/heptio/contour/issues/1130
[2]: https://github.com/heptio/contour/issues/1184
[3]: https://github.com/heptio/contour/issues/1185
