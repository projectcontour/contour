# Support Secret Discovery Service on Contour

_Status_: Draft

This document outlines what changes are needed on Contour to support SDS. [0]

## Goals

- Implement SDS server as a gRPS service
- Support auth.SdsSecretConfig configuration on Listener & Cluster under auth.CommonTlsContext
- Support contour cli for SDS

## Non-goals

- NA

## Unknowns

- Changes to be done to take care of TLS delegation functionality
- Any backward compatibility requirement

## Background

Currently, contour supports fetching TLS cert/keys from k8s secrets and populates
the values for DownstreamTlsContext in LDS & UpstreamTlsContext in CDS.
Now that Envoy 1.8 and later releases support Secret Discovery Service (SDS) to fetch 
secrets remotely, we want to add support in Contour to parse SDS secret config
and stream secrets using SDS GRPc service. Please refer to [1] to understand more on 
how SDS works on Envoy.

## High level design

- Create GRPC server for SDS
- Create SecretCache to be used by Contour Internally to identify any changes.
- Add support for field `tls_certificate_sds_secret_configs` under Listener and Cluster.
- Support contour cli for SDS

## Detailed design

### Create GRPC server for SDS

Create new resource SDS of `Cache` Interface type which implements the SDS v2 gRPS API.
Create new xdsHandler of secret type. Register & Implement FetchSecrets() & StreamSecrets().

### Create SecretCache to be used by Contour Internally
In /internal/contour create secret.go with SecretCache struct
Implement Register(), Update() and notify()
Create secretVisitor and implement visit() and visitSecrets() to produce a map with v2.secrets

### Add support for field `tls_certificate_sds_secret_configs` under Listener.
Since currently contour only supports Ingress Route with Secret Name, cert and key
will be embedded in the Listener Config & CLuster Config of the corresponding discovery service.
In order to any Listener or Cluster to start using the SDS service exposed by Contour, we have 
to add support to the Ingress Route spec to start parsing `tls_certificate_sds_secret_configs` 
i.e. auth.SdsSecretConfig [2] which will allow envoy to start learning secrets through SDS.

### Support contour cli for SDS

In order to facilitate debugging and to find out exactly the data that is being sent to Envoy,
will add support to contour cli sub command. This cmd shd be used stream changes to the SDS api endpoint
to the terminal.
`kubectl -n heptio-contour exec $CONTOUR_POD -c contour contour cli sds`


[0]: https://github.com/heptio/contour/issues/898
[1]: https://www.envoyproxy.io/docs/envoy/v1.9.0/configuration/secret
[2]: https://www.envoyproxy.io/docs/envoy/v1.9.0/api-v2/api/v2/auth/cert.proto#auth-sdssecretconfig

