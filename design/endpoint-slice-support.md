# Contour k8s EndpointSlice support

Contour currently supports Kubernetes endpoints.
This document proposes a process to enable support for k8s EndpointSlices and, eventually, migrate to exclusively using it.

## Goals

- Contour will support k8s EndpointSlices.
- The EndpointSlices will be gated by a feature flag in the Contour config.
- Kubernetes endpoints will still be the default mode of operation.

## Non-goals

- Removal of k8s endpoints.
- Support advanced features such as topology aware routing/hints offered by k8s endpoints.

## Background

See [#2696](https://github.com/projectcontour/contour/issues/2696)

Before the introduction of the EndpointSlices, the only method for a service to expose the IP addresses and ports of the pads it fronted was through Kubernetes endpoints.

However, Kubernetes endpoints have a few challenges with scaling, such as
- transmission of the entire endpoint object even if one endpoint changes, leading to network contention and heavy recalculation on the consumers of this data.
- As of k8s v1.22, a hard limit of 1000 addresses per endpoint object.
- No topology aware information for more intelligent routing.

To solve the above problems, EndpointSlices was introduced in 1.16 and, as of k8s v1.21 in [stable state](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/).


## High-Level Design

A feature flag will gate the usage of EndpointSlices, and the implementation will use the same pattern as used in the `endpointtranslator.go` with a few differences listed in the detailed design.

## Detailed Design

### Contour Configuration changes
EndpointSlices will be configured by a boolean feature flag, which defaults to false and allows users to opt to use EndpointSlices.

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
    // from k8s EndpointSlices. defaults to false and reading endpoint
    // data from the k8s endpoints.
    useEndpointSlice bool
}
```
### Implementation

#### Context
The implementation of k8s EndpointSlices is fundamentally different from how EndpointSlices work.

Below is an example of a Kubernetes service represented in as endpoint and EndpointSlice objects

endpoints
```
NAME                       ENDPOINTS                                                          AGE
ingress-conformance-echo   10.244.0.10:3000,10.244.0.153:3000,10.244.0.154:3000 + 7 more...   3d
```

EndpointSlice
```
NAME                             ADDRESSTYPE   PORTS   ENDPOINTS                   AGE
ingress-conformance-echo-8bcrv   IPv4          3000    10.244.0.156,10.244.0.157   18h
ingress-conformance-echo-dr6d2   IPv4          3000    10.244.0.158,10.244.0.159   18h
ingress-conformance-echo-dt27v   IPv4          3000    10.244.0.153,10.244.0.154   2d14h
ingress-conformance-echo-l2j48   IPv4          3000    10.244.0.10,10.244.0.155    3d
ingress-conformance-echo-z4bcw   IPv4          3000    10.244.0.160,10.244.0.161   18h
```

#### EndpointSlice cache

Because of the above mentioned differences, there is a need to adapt the cache logic from `endpointtranslator.go` such as;

Instead of using  `endpoints map[types.NamespacedName]*v1.Endpoints`, use the below format with a nested map `k,v` where `k` is the EndpointSlice name and the `v` is the EndpointSlice itself.

```go
type EndpointSliceCache struct {
    endpointSlice: map[types.NamespacedName]map[string]*discoveryv1.EndpointSlice{},
}
```

The above structure lets us keep track of each EndpointSlice and change/update/delete an endpoint without shuffling all the existing endpoints.

#### EndpointSlice conditions
The EndpointSlice has information about the condition of the endpoint, namely, `ready`, `serving` and `terminating`. The EndpointSlice implementation will only support the `ready` condition.
This means that only pods with `ready` status will be considered since `ready` on the EndpointSlice maps to `ready` status on the pods. This also keep contour inline with the existing implementation.

refer [ready status](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/#ready)

#### FQDN Address types
FQDN address types on EndpointSlices will not be supported. Contour will continue to support `externalName` service types.

#### Disabled Mirroring
If a service has [endpoint mirroring](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/#endpointslice-mirroring) disabled, it will not be processed by Contour.

## Tests

To ensure the feature is well tested, this feature will have
- e2e tests
- unit tests
- daily build run with EndpointSlices enabled.

## Migration Plan

Once we have enough consensus on the correctness and performance, we can make the move to EndpointSlices permanent and deprecate use of endpoints. This can be a combination of daily builds, burn time and feedback from the community.


## Alternatives Considered

N/A
