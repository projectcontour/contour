# Internal Redirect

Status: Draft

## Abstract
This document describes the API changes needed to support internal redirect policy support in contour HTTPProxy.

## Background
Internal redirect is a way to let the proxy server intercept 3xx redirect response, synthesizing a new request, sending it to the upstream specified by the new route match, and returning the redirected response as the response to the original request.
Internal redirect is supported by Envoy since v1.10, but there is no way to configure an internal redirect policy using Contour.

## Goals
- Configuring per route internal redirect policy.
- Supporting envoy builtin 'internal redirect policy' predicates.

## Non Goals
- Supporting user defined 'internal redirect policy' predicate.

## High-Level Design

Envoy Route supports [internal_redirect_policy] (https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/http/http_connection_management#internal-redirects) and it can be configured [using the API](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-internal-redirect-policy). The changes proposed in this document will allow the configuration of internal redirect policies in Contour.

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
         predicates:
           - name: SafeCrossScheme
           - name: PreviousRoutes
           - name: AllowListedRoutes
             allowedRouteNames: [ "other" ]
         allowCrossSchemeRedirect: true
     - name: other
       conditions:
         - prefix: /other
       services:
         - name: other
           port: 8090
```

## Detailed Design

Add a new type: `HTTPInternalRedirectPolicy` to define when the proxy should intercept and handle redirect responses internally.

```Go
// HTTPInternalRedirectPolicy defines when to handle redirect responses internally.
type HTTPInternalRedirectPolicy struct {
    // MaxInternalRedirects An internal redirect is not handled, unless the number of previous internal redirects that a downstream request has encountered is lower than this value.
    // +kubebuilder:default=0
    // +optional
    MaxInternalRedirects uint32 `json:"maxInternalRedirects,omitempty"`

    // RedirectResponseCodes If unspecified, only 302 will be treated as internal redirect.
    // Only 301, 302, 303, 307 and 308 are valid values.
    // +optional
    RedirectResponseCodes []uint32 `json:"redirectResponseCodes,omitempty"`

    // Predicates are queried when an upstream response is deemed to trigger an internal redirect by all other criteria.
    // +optional
    Predicates []HTTPInternalRedirectPredicate `json:"predicates"`

    // AllowCrossSchemeRedirect Allow internal redirect to follow a target URI with a different scheme than the value of x-forwarded-proto.
    // +kubebuilder:default=false
    // +optional
    AllowCrossSchemeRedirect bool `json:"allowCrossSchemeRedirect,omitempty"`
}

// HTTPInternalRedirectPredicate defines the predicate used to accept or reject an internal redirection.
type HTTPInternalRedirectPredicate struct {
    // Name of the predicate to apply
    Name HTTPInternalRedirectPredicateName `json:"name"`

    // AllowedRouteNames is the list of routes that’s allowed as redirect target
    // by this predicate, identified by the route’s name.
    //
    // Note: AllowedRouteNames is ignored if Name is not AllowListedRoutes.
    //
    // +optional
    AllowedRouteNames []string `json:"allowedRouteNames,omitempty"`
}

// InternalRedirectPredicate defines the predicate used to accept or reject an internal redirection.
//
// Supported predicates are AllowListedRoutes, PreviousRoutes and SafeCrossScheme.
//
// +kubebuilder:validation:Enum=AllowListedRoutes;PreviousRoutes;SafeCrossScheme
type HTTPInternalRedirectPredicateName string

const (
    // An internal redirect predicate that accepts only explicitly allowed target routes.
    AllowListedRoutes HTTPInternalRedirectPredicateName = "AllowListedRoutes"

    // An internal redirect predicate that rejects redirect targets that are pointing to a route that has been followed by a previous redirect from the current route.
    PreviousRoutes HTTPInternalRedirectPredicateName = "PreviousRoutes"

    // An internal redirect predicate that checks the scheme between the downstream url and the redirect target url.
    SafeCrossScheme HTTPInternalRedirectPredicateName = "SafeCrossScheme"
)
```

Update `Route` accordingly. 
The `AllowListedRoutes` predicates declares a list of allowed route's name, so a `Name` field is required on `Route` to be able to use it.

```Go
type Route struct {
    [... other members ...]

    // Name of the route.
    // +optional
    Name string `json:"name,omitempty"`

    // InternalRedirectPolicy defines when to handle redirect responses internally.
    // +optional
    InternalRedirectPolicy *InternalRedirectPolicy `json:"internalRedirectPolicy,omitempty"`
}
```

The common DAG struct will be updated accordingly:

```Go
// contour/internal/dag/dag.go
InternalRedirectPredicate interface {
    Is_InternalRedirectPredicate()
}

// AllowListedRoutesPredicate accepts only explicitly allowed target routes.
type AllowListedRoutesPredicate struct {
    AllowedRouteNames []string
}

// PreviousRoutesPredicate rejects redirect targets that are pointing to a route that has been followed by a previous redirect
type PreviousRoutesPredicate struct {
}

// SafeCrossSchemePredicate checks the scheme between the downstream url and the redirect target url
type SafeCrossSchemePredicate struct {
}

func (*AllowListedRoutesPredicate) Is_InternalRedirectPredicate() {}
func (*PreviousRoutesPredicate) Is_InternalRedirectPredicate()    {}
func (*SafeCrossSchemePredicate) Is_InternalRedirectPredicate()   {}

// InternalRedirectPolicy defines if envoy should handle redirect response internally instead of sending it downstream.
// https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-msg-config-route-v3-internalredirectpolicy
type InternalRedirectPolicy struct {
    // MaxInternalRedirects An internal redirect is not handled, unless the number of previous internal redirects that a downstream request has encountered is lower than this value
    MaxInternalRedirects uint32

    // RedirectResponseCodes If unspecified, only 302 will be treated as internal redirect.
    // Only 301, 302, 303, 307 and 308 are valid values
    RedirectResponseCodes []uint32

    // Predicates list of predicates that are queried when an upstream response is deemed to trigger an internal redirect by all other criteria
    Predicates []InternalRedirectPredicate

    // AllowCrossSchemeRedirect Allow internal redirect to follow a target URI with a different scheme than the value of x-forwarded-proto.
    AllowCrossSchemeRedirect bool
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

N/A

## Security Considerations

N/A

## Compatibility
This change should be additive, so there should be no compatibility issues.

## Implementation
An experimental implementation can be found here (including e2e test):

https://github.com/Jean-Daniel/contour/tree/internal_redirect

## Open Issues

https://github.com/projectcontour/contour/issues/4843

