# Contour k8s endpoint slice support

Contour currently supports Kubernetes endpoints.
This document proposes a process to enable support for k8s endpoint slices and, eventually, migrate to exclusively using it.

## Goals

- Contour will support k8s endpoint slices.
- The endpoint slices will be gated by a feature flag in the Contour config.
- Kubernetes endpoints will still be the default mode of operation.

## Non-goals

- Removal of k8s endpoints.
- Support advanced features such as topology aware routing/hints offered by k8s endpoints.

## Background

See [#2696](https://github.com/projectcontour/contour/issues/2696)

Before the introduction of the endpoint slices, the only method for a service to expose the IP addresses and ports of the pads it fronted was through Kubernetes endpoints.

However, Kubernetes endpoints have a few challenges with scaling, such as
- transmission of the entire endpoint object even if one endpoint changes, leading to network contention and heavy recalculation on the consumers of this data.
- As of k8s v1.22, a hard limit of 1000 addresses per endpoint object.
- No topology aware information for more intelligent routing.

To solve the above problems, endpoint slices was introduced in 1.16 and, as of k8s v1.21 in [stable state](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/).


## High-Level Design

A feature flag will gate the usage of endpoint slices, and the implementation will use the same pattern as used in the `endpointtranslator.go` with a few differences listed in the detailed design.

## Detailed Design

### Contour Configuration changes
Endpoint slices will be configured by a boolean feature flag, which defaults to false and allows users to opt to use endpoint slices.

```yaml
...
server:
    xds-server-type: contour
use-endpoint-slice: true
...
```

```go
type Parameters struct {
    // useEndpointSlice configures contour to fetch endpoint data
    // from k8s endpoint slices. defaults to false and reading endpoint
    // data from the k8s endpoints.
    useEndpointSlice bool
}
```
### Implementation

#### Context
The implementation of k8s endpoint slices is fundamentally different from how endpoint slices work.

Below is an example of a Kubernetes service represented in as endpoint and endpoint slice objects

endpoints
```
NAME                       ENDPOINTS                                                          AGE
ingress-conformance-echo   10.244.0.10:3000,10.244.0.153:3000,10.244.0.154:3000 + 7 more...   3d
```

endpoint slice
```
NAME                             ADDRESSTYPE   PORTS   ENDPOINTS                   AGE
ingress-conformance-echo-8bcrv   IPv4          3000    10.244.0.156,10.244.0.157   18h
ingress-conformance-echo-dr6d2   IPv4          3000    10.244.0.158,10.244.0.159   18h
ingress-conformance-echo-dt27v   IPv4          3000    10.244.0.153,10.244.0.154   2d14h
ingress-conformance-echo-l2j48   IPv4          3000    10.244.0.10,10.244.0.155    3d
ingress-conformance-echo-z4bcw   IPv4          3000    10.244.0.160,10.244.0.161   18h
```

#### Endpoint slice cache

Because of the above mentioned differences, there is a need to adapt the cache logic from `endpointtranslator.go` such as;

Instead of using  `endpoints map[types.NamespacedName]*v1.Endpoints`, use the below format with a nested map `k,v` where `k` is the endpoint slice name and the `v` is the endpoint slice itself.

```go
type EndpointSliceCache struct {
    endpointSlice: map[types.NamespacedName]map[string]*discoveryv1.EndpointSlice{},
}
```

The above structure lets us keep track of each endpoint slice and change/update/delete an endpoint without shuffling all the existing endpoints.

## Tests

To ensure the feature is well tested, this feature will have
- e2e tests
- unit tests

## Alternatives Considered

N/A
