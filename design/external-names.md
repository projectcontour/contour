# External Names Service

_Status_: Accepted

This document looks to allow Contour to process Kubernetes service types `ExternalName` so that traffic can be proxied to dns hosts rather than only pod endpoints.

## Goals

- Allow for Contour to proxy traffic via Envoy to external dns endpoints
- Utilize built-in Kubernetes object types (i.e. ExternalName service)

## Non-goals

- `externalIPs` will not be used, users who want to proxy to custom IP's should build their own service/endpoint object
- Utilize a new CRD or ConfigMap style mapping
- Only support HTTP at the moment, TCP will be addressed in a further design doc

## Background

Contour currently watches Kubernetes services and endpoints, streaming them to via Envoy xDS CDS & EDS endpoints.
Kubernetes pods are identified by their presence in the Endpoint document and streamed to Envoy via EDS.
Requests to envoy are routed directly to Kubernetes pods (i.e. endpoints).
This proposal looks to add a second way to identify the members of a Envoy clustr object, using a DNS entry to supply the cluster members rather than EDS.

## High-level Design

Contour will need to watch for and process Kubernetes service types of `ExternalName` (https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.13/#servicespec-v1-core).
When configuring a Route, Contour will use the contents of the `spec.externalName` of the service as well as the port from the `spec.ports` section of the Kubernetes service.

If ports are not supplied for the Kubernetes service, then the IngressRoute status will be set accordingly with an error message.

Allowing for TLS proxying and validation should still be applied as it exists already so users can proxy to a TLS upstream as well as validate the certificate+subject alt name of the upstream service.

## Detailed Design

- The `TCPService` struct in `internal/dag/dag.go` will have a new `externalName string` field added.
- When services are added in `internal/dag/builder.go`, if `svc.Spec.Type == v1.ServiceTypeExternalName` then the value of `svc.Spec.ExternalName` will be set to the TCPService struct's `externalName`.
- Since the service does not have any Kubernetes Endpoints, we cannot rely on having an EDS cluster to reference from the CDS cluster.
For this type of service (i.e. `ExternalName`) when creating the cluster in `internal/envoy/cluster.go`, instead of setting a `EdsClusterConfig`, we'll configure a `LoadAssignment` which should allow us to specify the endpoint defined in `externalName` dynamically.

## Alternatives

Potentially, we could avoid using a Service of type `ExternalName` by simply just extending the `IngressRoute` CRD as proposed here: https://github.com/heptio/contour/issues/825#issue-386869426

The downside to this approach is it couldn't be used with Ingress resources.

## Security Considerations

Potentially someone could use an `ExternalName` service to reference services in another namespace by setting the `externalName` (ex: `other-service.namespace-other.svc.cluster.local`).

This scenario exists in a cluster regardless if Contour is deployed or not, other security restrictions should be applied if users want to avoid this from happening.
