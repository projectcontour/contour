# Internal Redirect

Status: Accepted

## Abstract
This document describes the API changes needed to support internal redirect policy support in contour HTTPProxy.

## Background
Internal redirect is a way to let the proxy server intercept 3xx redirect response, synthesizing a new request, sending it to the upstream specified by the new route match, and returning the redirected response as the response to the original request.
Internal redirect is supported by Envoy since v1.10, but there is no way to configure an internal redirect policy using Contour.

## Goals
- Configuring per route internal redirect policy.
- Supporting envoy `previous route` and `safe cross scheme` predicates.

## Non Goals
- Supporting `allow listed routes` predicate.
- Supporting user defined 'internal redirect policy' predicate.

## High-Level Design

Envoy Route supports [internal_redirect_policy](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_connection_management#internal-redirects) and it can be configured [using the API](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-internal-redirect-policy). The changes proposed in this document will allow the configuration of internal redirect policies in Contour.

At a high level the proposed changes will imply:
- Adding new fields at virtual host level to configure the internal redirect policy in the YAML.
- Changing some structs in the code.
- Generating the corresponding Envoy configuration.

### Proposed YAML fields

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
 name: myservice
 namespace: prod
spec:
 virtualhost:
   fqdn: www.example.com
   routes:
     - conditions:
         - prefix: /echo
       services:
         - name: echo
           port: 8080
       internalRedirectPolicy:
         maxInternalRedirects: 5
         redirectResponseCodes: [ 301, 302, 303 ]
         allowCrossSchemeRedirect: SafeOnly
         denyRepeatedRouteRedirect: true
```

## Detailed Design

Add a new type: `HTTPInternalRedirectPolicy` to define when the proxy should intercept and handle redirect responses internally.

```Go
// HTTPInternalRedirectPolicy defines when to handle redirect responses internally.
type HTTPInternalRedirectPolicy struct {
    // MaxInternalRedirects An internal redirect is not handled, unless the number of previous internal redirects that a downstream request has encountered is lower than this value.
    // +optional
    MaxInternalRedirects uint32 `json:"maxInternalRedirects,omitempty"`

    // RedirectResponseCodes If unspecified, only 302 will be treated as internal redirect.
    // Only 301, 302, 303, 307 and 308 are valid values.
    // +optional
    RedirectResponseCodes []uint32 `json:"redirectResponseCodes,omitempty"`

    // AllowCrossSchemeRedirect Allow internal redirect to follow a target URI with a different scheme than the value of x-forwarded-proto.
    // SafeOnly allows same scheme redirect and safe cross scheme redirect, which means if the downstream scheme is HTTPS, both HTTPS and HTTP redirect targets are allowed, but if the downstream scheme is HTTP, only HTTP redirect targets are allowed.
    // +kubebuilder:validation:Enum=Always;Never;SafeOnly
    // +kubebuilder:default=Never
    // +optional
    AllowCrossSchemeRedirect string `json:"allowCrossSchemeRedirect,omitempty"`

    // If DenyRepeatedRouteRedirect is true, rejects redirect targets that are pointing to a route that has been followed by a previous redirect from the current route.
    // +optional
    DenyRepeatedRouteRedirect bool `json:"denyRepeatedRouteRedirect,omitempty"`
}

```

Update `Route` accordingly. 

```Go
type Route struct {
    [... other members ...]

    // InternalRedirectPolicy defines when to handle redirect responses internally.
    // +optional
    InternalRedirectPolicy *InternalRedirectPolicy `json:"internalRedirectPolicy,omitempty"`
}
```

The common DAG struct will be updated accordingly:

```Go

// InternalRedirectPolicy defines if envoy should handle redirect response internally instead of sending it downstream.
// https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-msg-config-route-v3-internalredirectpolicy
type InternalRedirectPolicy struct {
    // MaxInternalRedirects An internal redirect is not handled, unless the number of previous internal redirects that a downstream request has encountered is lower than this value
    MaxInternalRedirects uint32

    // RedirectResponseCodes If unspecified, only 302 will be treated as internal redirect.
    // Only 301, 302, 303, 307 and 308 are valid values
    RedirectResponseCodes []uint32

    // AllowCrossSchemeRedirect Allow internal redirect to follow a target URI with a different scheme than the value of x-forwarded-proto.
    AllowCrossSchemeRedirect string

    DenyRepeatedRouteRedirect bool
}

type Route struct {
    [... other members ...]

    // InternalRedirectPolicy defines if envoy should handle redirect response internally instead of sending it downstream.
    InternalRedirectPolicy *InternalRedirectPolicy
}
```

### Enabling internal redirect in Envoy
In order to enable internal redirect in Envoy we will update `contour/internal/envoy/route.go` and map the values from DAG's route to [protobuf](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-internal-redirect-policy).


## Alternatives Considered

Supporting `Allow Listed Routes` predicate too. 

```
    // AllowedRouteNames is the list of routes that are allowed as redirect target, identified by the route’s name.
    // An empty list means all routes are allowed.
    // +optional
    AllowedRouteNames []string `json:"allowedRouteNames,omitempty"`
```

This predicate takes a list of route names, but the concept of route name is not exposed in Contour.
While it is possible to also add a `Name` field to the Route struct, it has to be carefully design to prevent issues in multitenant deployments, and so should have its own design proposal.
In the case of multitenant Contour deployment, 2 teams may declare routes using the same name, defeating the purpose of the allowed routes list.

Note that the proposed design does not prevent adding `AllowedRouteNames` support in the future if needed.

## Security Considerations

N/A

## Compatibility
This change should be additive, so there should be no compatibility issues.

## Implementation
An experimental implementation can be found here (including e2e test):

https://github.com/Jean-Daniel/contour/tree/internal_redirect

## Open Issues

https://github.com/projectcontour/contour/issues/4843

