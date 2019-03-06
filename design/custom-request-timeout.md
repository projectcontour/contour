# Executive Summary

**Status**: _Draft_

This document describes the design of a new resource in IngressRoute for custom request timeout and retries. 

This new resource will be integrated into Contour 0.11.

# Goals

- Generic resource name for configuring custom request timeouts per IngressRoute
- Generic resource name for configuring custom retry attempts per IngressRoute

# Non-goals

- None

# Background

Contour supports custom request timeout and custom retry attempts via Ingress. The same functionality has been
requested via IngressRoute as well.

# High-level design

At a high level, this document proposes the changes in the IngressRoute's CRD to also support custom request
timeouts and retries. It covers:

- Fields added for timeout and retries in the YAML file.
- A high-level coverage on the struct changes required in the code.
- Arguments passed for Envoy API calls.

## Proposed YAML fields

```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
    name: github.com
    namespace: prod
spec:
    virtualhost:
        fqdn: www.github.com
        routes:
        - match: /contour
          # the timeout applied to the requests in this route
          timeoutPolicy:
              # the timeout applied
              connect: 500ms
              # the timeout applied
              response: 2s
          retryPolicy:
              # the number of retries it should make on failure
              maxRetries: 7
              # the conditions for which it should reattempt
              onStatusCodes:
              - 501
              - 502
              # timeout between requests
              perTryTimeout: 75ms
        - match: /static-site
          timeoutPolicy:
              connect: 200ms
              response: 1s
          retryPolicy:
              maxRetries: 2
              # for all statusCodes but 504, retry the request
              onStatusCodes:
              - 5xx
              perTryTimeout: 75ms
```

# Detailed Design

## Reading from YAML
The ingressroute spec struct will be updated to contain Timeout and Retry struct members


```golang
// contour/apis/contour/v1beta1/ingressroute.go

// Route contains the set of routes for a virtual host
type Route struct {
    [... other members ...]
	// The request timeout for this route
	Timeout TimeoutPolicy `json:"timeoutPolicy,omitempty"`
	// The retry attempts for this route
	Retry RetryPolicy `json:"retryPolicy,omitempty"`
}

// TimeoutPolicy defines the attributes associated with timeout
type TimeoutPolicy struct {
	// Timeout for establishing a connection in milliseconds
	Connect	time.Duration `json:"connect"`
	// Timeout for receiving a response in seconds
	Response time.Duration `json:"response"`
}

// RetryPolicy defines the attributes associated with retrying policy
type RetryPolicy struct {
	// MaxRetries is maximum allowed number of retries
	MaxRetries	int `json:"maxRetries"`
	// Perform retry on failed requests with the matched status codes or aggregated as 5xx
	OnStatusCodes []string `json:"onStatusCodes"`
	// PerTryTimeout specifies the timeout per retry attempt. Ignored if OnStatusCodes are empty
	PerTryTimeout time.Duration `json:"perTryTimeout"`
}
```

## Intermediate form in the DAG
We store unmarshalled structs from ingress' and ingressroute's yaml into a common dag struct route for both. 
There are three approaches to do change:

### Approach 1
Just add ResponseTimeout as another member in the struct as below as Response Timeout is the only new member
in the _current_ proposed CRD.

```golang
// Approach 1: contour/internal/dag/dag.go

type Route struct {
    [... other members ...]
    ResponseTimeout time.Duration
}

```

### Approach 2

Consolidate timeout and retry members of dag's Route struct to their own respective struct.

```golang
// Approach 2: contour/internal/dag/dag.go

type RouteTimeout struct {
    Connect     time.Duration
    Response    time.Duration
    [... more members can be added based on requirements ...]
}

type RouteRetry struct {
    MaxRetries      int
    OnStatusCodes   []string
    PerTryTimeout   time.Duration
    [... more members can be added based on requirements ...]
}

type Route struct {
    Prefix          string
    object          interface{}
    httpServices    map[servicemeta]*HTTPService
    HTTPSUpgrade    bool
    Websocket       bool
    PrefixRewrite   string

    // new member replacing current Timeout
    Timeout         RouteTimeout
    // new member replacing current RetryOn, NumRetries, PerTryTimeout
    Retry           RouteRetry
}
```

This could involve more changes in the code and their unit tests to be corrected, but hopefully,
the future additions if made can be neatly encapsulated.

### Approach 3
No change in the dag's Route struct, pick the required new member from the `object` interface when needed.

Depending on the above changes, `prefixRoute` for Ingress object and `processRoutes` for IngressRoute object
would need to be visited for filling in the decided Route structure.

## Communicating from DAG to Envoy

Now that we have the DAG built, we need to communicate the state of the routes to Envoy. This happens `RouteRoute`
function of `contour/internal/envoy/route.go` 

We would use the following to map value from DAG's route to protobuf

```
RouteAction.Timeout = Route.Timeout.Connect
RouteAction.IdleTimeout = Route.Timeout.Response

RouteAction.RetryPolicy.RetryOn = Route.Retry.OnStatusCodes // list joined into a string
RouteAction.RetryPolicy.NumEntries = Route.Retry.MaxRetries
RouteAction.RetryPolicy.PerTryTimeout = Route.Retry.PerTryTimeout

```

## Validation rules

Let's consider the following topology for communication

+---------+      +---------+       +----------+

| Client  |----->| Proxy   |------>| Server   |

+---------+      +---------+       +----------+


### Route.Timeout

- `Connect` : The time that spans between the point at which complete client request has been processed by the proxy, and when the
              response from the server has been completely processed.
              0 or unspecified will be used as Proxy's default (15s for Envoy)
              -1 for "infinity"
              Any other positive number is the actual time in milliseconds.

- `Response` : The idle timeout for the route, ie, no request events have occurred in the route. The Request Timeout error code returns
               with HTTP error code 408.
               Unspecified will not use any timeout for this route, besides the proxy's connection manager default (if present) will apply.
               0 no timeout for this route and also the proxy's connection manager default (if present) will be not applicable.
               Any other positive number is the actual time.

### Route.Retry

- `MaxRetries` : Maximum number of allowed retries. It applies in conjunction with `OnStatusCodes` if specified, otherwise retries all
                 5xx, 408 and 409 (Conflict response) status code responses by default.

                 If unspecified, it defaults to one retry.

- `OnStatusCodes` : HTTP Status codes to retry upon can be specified. It applies in conjunction with `MaxRetries` if specified, otherwise
                    defaults to 1 retry attempt.

                    If both MaxRetries and OnStatusCodes are unspecified, it defaults to one retry on all failures, ie, 5xx, 408, 409.

- `PerTryTimeout` : This is the timeout that applies per try of a request from proxy to server. The time specified here must be <=
                    Route.Timeout.Connect timeout to ensure multiple attempts can be made within the described Route.Timeout.Connect 

                    If unspecified, there will be no per try timeout.
                    If specified (in milliseconds), this is always in conjunction with default / mentioned MaxRetries:OnStatusCodes:Connect-timeout

# Example Use-Cases

## Resiliency of the System
Failed Requests can be retried without any negative consequences, shielding users from transient issues.
A configurable timeout allow applications to be reactive sooner to a down service in an attempt to match the SLA.


# References

1. https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/router_filter 
1. https://www.envoyproxy.io/learn/automatic-retries
1. https://odino.org/better-performance-the-case-for-timeouts/ 
1. https://appscode.com/products/voyager/8.0.1/guides/ingress/configuration/default-timeouts/ 
1. https://blog.christianposta.com/microservices/02-microservices-patterns-with-envoy-proxy-part-ii-timeouts-and-retries/

# Future work
_TBD_

# Security Concerns

_TBD_
