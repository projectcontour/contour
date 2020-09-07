# Backend TLS: client authentication

Status: Draft

This proposal describes how to implement support for client certificate for Envoy for the TLS connection between Envoy and backend services.

## Goals

- Envoy shall be able authenticate itself to the backend
- Backend service shall be able to request clients to present their client certificate

## Non Goals

- Configure several client certificates for Envoy(s) e.g. different client certificate per each backend or different client certificate for each Envoy.
- Specify mechanism how the CA certificate for validating Envoy's client certificate is distributed to the backend developers.

## Background

It is already possible to use TLS for the connection between Envoy and the backend service.
However only the backend is authenticated and the backend is not able to authenticate Envoy.
Since the backend has no means to differentiate between clients, any client within the cluster network is able to access backend's resources.
This could potentially include vulnerable software which might be running within the cluster and remotely exploited to access the backend.

After external client certificate validation was introduced to Contour 1.4.0, it makes sense to allow configuring client certificates for the TLS connection between Envoy and backends.
This completes securing the end-to-end connection all the way from the external client to the backend.

Securing the connection between Envoy and backend can have similar importance as securing the xDS TLS connection between Contour and Envoy.

## High-Level Design

Configuring the client certificate shall follow the same configuration pattern as `fallback-certificate`.
The client certificate and key are stored in Secret of type `tls` in PEM format.
The client certificate file `tls.crt` shall contain the client certificate as the first PEM block, followed by optional chain of CA certificates, up to but not including the root CA certificate.
New configuration option is added to the Contour configuration file in the optional `tls.envoy-client-certificate` location.
The value of the option refers to a Secret.
The reference contains the namespace and the Secret name e.g. `namespace/name`.

Certificate delegation is not needed since client certificate is configured within configuration file at deployment level, not in `HTTPProxy` resources on the application level.

## Detailed Design

New field is added to `dag.Cluster` that holds the client certificate for Envoy.
Note that since only single client certificate is supported, the secret is the same for all instances of type `dag.Cluster`.
When `CommonTlsContext` is constructed for the upstream connection, the client certificate is added using `SdsSecretConfig`, that is, the secret is streamed over SDS.
Updates in `SecretCache` triggers secrets to be streamed to Envoy, which will reload the updated certificates and keys.

When client certificate is defined in Contour configuration, it can be added to all Envoy clusters regardless if it will be used or not.
Envoy will send the client certificate only when the backend requests the client to present its certificate during the TLS handshake.

When [external authorization](external-authorization-design.md) is implemented in the future, the gRPC connection to the external authorization server shall also be protected by client authentication by the same principle laid out in this document.

## Alternatives Considered

Alternatively, client certificates could be configured in `HTTPProxy.spec.routes[].services[]` similarly as server validation is currently configured in `HTTPProxy.spec.routes[].services[].validation`.
This allows different client certificate to be used towards each backend, and it allows teams in multitenant cluster to manage both client and server certificates by themselves.
However, single client certificate is sufficient currently and allowing override per `HTTPProxy` can be considered as future extension.

## Security Considerations

Envoys can be considered as single "distributed" TLS client, all equally authorized to connect to the upstream services.
Therefore single client certificate is enough for the use case.
Separate certificates could be needed e.g. if upstream needs to tell each Envoy apart but this is not valid use case currently.
