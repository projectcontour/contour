# Executive Summary

**Status**: _Accepted_

This document describes the design of a new resource in IngressRoute for custom request timeout and retries. 

# Goals

- Generic resource name for configuring custom request timeouts per IngressRoute
- Generic resource name for configuring custom retry attempts per IngressRoute

# Non-goals

- Even though Virtual Hosts in Envoy supports Retry Policy, this design doc will not address it.

# Background

Contour supports custom request timeout and custom retry attempts via [Ingress Annotations](https://github.com/projectcontour/contour/blob/main/docs/annotations.md).
We wish to expose the same same functionality has been requested via IngressRoute as well.

Additionally, request and retry behavior apply to any interaction that 

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
          # the timeout applied to the requests to "/contour" only, independent of other routes.
          timeoutPolicy:
              # the request timeout applied on this route, it is the maximum time that a client will
              # await a response to its request.
              request: 1s
          # the retryPolicy described here are local to match and apply for "/contour" only
          retryPolicy:
              # the number of retries it should make on failure, in conjunction with status codes
              # mentioned for this route.
              count: 7
              # timeout between the requests, if greater than request timeout, this field is ignored
              perTryTimeout: 150ms
        - match: /static-site
          # the timeout applied for the requests to "/static-site" only, independent of other routes.
          timeoutPolicy:
              request: 200ms
              idle: 1s
          # retry for this route is independent of "/contour" retryPolicy.
          retryPolicy:
          	count: 2
```

The naming conventions of the proposed YAML fields are inspired from [HTTP Timeouts](https://tools.ietf.org/id/draft-thomson-hybi-http-timeout-00.html#rfc.section.4).

These proposed YAML resources are defined with being generic as motivation so that any ingress controller can use these 
fields of IngressRoute and setup the ingress, not just limited to contour-envoy only.

# Detailed Design

## Reading from YAML
The ingressroute spec struct will be updated to contain Timeout and Retry struct members


```go
// contour/apis/contour/v1beta1/ingressroute.go

// Route contains the set of routes for a virtual host
type Route struct {
    [... other members ...]
	// The timeout policy for this route
	TimeoutPolicy *TimeoutPolicy `json:"timeoutPolicy,omitempty"`

	// The retry policy for this route
	RetryPolicy *RetryPolicy `json:"retryPolicy,omitempty"`
}

// TimeoutPolicy define the attributes associated with timeout
type TimeoutPolicy struct {
	// Timeout for receiving a response from the server after processing a request from client.
	// If not supplied the timeout duration is undefined.
	Request	*JsonDuration `json:"request"`
}

// RetryPolicy define the attributes associated with retrying policy
type RetryPolicy struct {
	// NumRetries is maximum allowed number of retries.
	// If not supplied, the number of retries is zero.
	NumRetries	int `json:"count"`

	// PerTryTimeout specifies the timeout per retry attempt.
	// Ignored if NumRetries is not supplied.
	PerTryTimeout *JsonDuration `json:"perTryTimeout"`
}

// JsonDuration parses a string as a time.Duration.
type JsonDuration struct {
	*time.Duration
}

func (d *JsonDuration) UnmarshalJSON(b []byte) (err error) {
	
	timeStr := strings.Trim(string(b), `"`)
	duration, err := time.ParseDuration(timeStr)
	
	if err != nil && timeStr == "infinity" {
		duration = -1
		err = nil
	}
	
	if err == nil {
		// only if correctly parsed, we update the Duration. Else, Duration is initialized as nil
		d.Duration = &duration
	}
	return
}

func (d JsonDuration) MarshalJSON() (b []byte, err error) {
	return []byte(fmt.Sprintf(`"%s"`, d.String())), nil
}

// Time interprets all forms of input received from the YAML file for Contour.
func (d *JsonDuration) Time() (timeout time.Duration, valid bool) {
	if d == nil {
		// this means the timeout field (like request, idle, etc.) was not specified in the YAML file
		// not specifying the timeout field is a valid input
		timeout, valid = 0, true
	} else if d.Duration == nil {
		// this means the timeout field was incorrectly specified, hence it couldn't be casted to time.Duration
		// incorrectly parsed timeout is an invalid input
		timeout, valid = -1, false
	} else {
		// timeout was correctly parsed, hence valid input
		timeout, valid = *d.Duration, true
	}
	
	return
}
```

## Intermediate form in the DAG for the routes
We store unmarshalled structs from ingress' and ingressroute's yaml into a common dag struct route for both. 

Consolidate timeout and retry members of dag's Route struct to their own respective struct. Though the names
`TimeoutPolicy` and `RetryPolicy` are reused, this isn't the same as described in the previous section.

```go
// Approach: contour/internal/dag/dag.go

type TimeoutPolicy struct {
    Request *time.Duration
    [... more members can be added based on requirements ...]
}

type RetryPolicy struct {
    NumRetries      int
    PerTryTimeout   *time.Duration
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
    TimeoutPolicy   *TimeoutPolicy
    // new member replacing current RetryOn, NumRetries, PerTryTimeout
    RetryPolicy     *RetryPolicy
}
```

This will involve slightly more changes in the code and their unit tests to be corrected, but hopefully,
the future additions if made can be neatly encapsulated.

## Communicating from DAG to Envoy

Now that we have the DAG built, we need to communicate the state of the routes to Envoy. This happens in `RouteRoute`
function of `contour/internal/envoy/route.go` 

We would use the following to map value from DAG's route to protobuf

### TimeoutPolicy for Route - Envoy
- [Upstream Timeout for the Route](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-timeout)

        RouteAction.Timeout = Route.TimeoutPolicy.Request.Duration

- [Idle Timeout for the Route](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-idle-timeout)

        RouteAction.IdleTimeout = Route.TimeoutPolicy.Idle.Duration

### RetryPolicy for Route - Envoy
- [Specifies the conditions under which retry takes place](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#route-retrypolicy-retry-on)
- RetryOn is hard coded to ["5xx"](https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/router_filter#config-http-filters-router-x-envoy-retry-on)

        // for route
        RouteAction.RetryPolicy.RetryOn = "5xx"

- [Specifies the allowed number of retries](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#route-retrypolicy-num-retries)

        RouteAction.RetryPolicy.NumEntries = Route.Retry.NumRetries

- [Specifies a non-zero upstream timeout per retry attempt](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#route-retrypolicy-per-try-timeout)

        RouteAction.RetryPolicy.PerTryTimeout = Route.Retry.PerTryTimeout

### Why only `5xx` Status Codes?

`4xx` are user based errors and `5xx` are server based.
Status codes of type `4xx` will continue to cause error unless the user corrects it.
Whereas `5xx` errors on the server can occur due to a fault in the server side which may have been caused by unusual circumstances like network packet becoming corrupted while being transmitted or due to intermittent failure in the application that's unlikely to be repeated.
The intent with this feature is to start small by supporting 5xx status codes, and then adding more status codes on need basis.

Citing the Retry Pattern described in [Azure Architecture - Retry Pattern](https://docs.microsoft.com/en-us/azure/architecture/patterns/retry), if the fault indicates that the failure isn't transient or is unlikely to be successful if repeated, the application should cancel the operation and report exception.
If the specific fault reported is unusual or rare, it could be considered as an intermittent failure and the application could retry the failing request, which could probably be successful.

# Testing

Beside the unit tests in the contour repo, we can use httpbin to test Retry and Timeout functionality.

## Using httpbin
httpbin simplifies testing of HTTP request and response. We will be using [/status](https://httpbin.org/#/Status_codes)
and [/delay](https://httpbin.org/#/Dynamic_data) of httpbin to test our feature's functionality. This is the application
deployed **after** contour-envoy has been up and running in your kubernetes cluster.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: httpbin
  name: httpbin
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin
  template:
    metadata:
      labels:
        app: httpbin
    spec:
      containers:
      - image: docker.io/kennethreitz/httpbin
        imagePullPolicy: IfNotPresent
        name: httpbin
        ports:
        - containerPort: 80
          name: http
        command: ["gunicorn"]
        args: ["-b", "0.0.0.0:8080", "httpbin:app"]
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
      
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin
spec:
  ports:
  - port: 8080
    protocol: TCP
    targetPort: 8080
  selector:
    app: httpbin
---
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  labels:
    app: httpbin
  name: httpbin
  namespace: default
spec:
  virtualhost:
    fqdn: contour.example.com
  routes:
    - match: /
      services:
        - name: httpbin
          port: 8080
      timeoutPolicy:
        request: 1s
      retryPolicy:
        maxRetries: "3"
        onStatusCodes:
          - "500"
          - "501"
    - match: /httpbin
      prefixRewrite: "/"
      services:
        - name: httpbin
          port: 8080
      retryPolicy:
        maxRetries: "10"
        perTryTimeout: 200ms
```

From the YAML we can see `http://contour.example.com/` has different policy than `http://contour.example.com/httpbin/`

Based on the arguments passed with `/status/{status}` and `/delay/{delay}` we will see the corresponding stats in envoy
increase by sending a curl request inside the Envoy's container - `curl http://localhost:9001/stats`

# References

1. [Azure Architecture - Retry Pattern](https://docs.microsoft.com/en-us/azure/architecture/patterns/retry)
2. [Envoy API Docs](https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/router_filter)
3. [Learn automatic retries - Envoy](https://www.envoyproxy.io/learn/automatic-retries)
4. [Better performances: the case for timeouts](https://odino.org/better-performance-the-case-for-timeouts/)
5. [Microservices Pattern with Envoy - Timeout and Retries](https://blog.christianposta.com/microservices/02-microservices-patterns-with-envoy-proxy-part-ii-timeouts-and-retries/)
6. [httpbin.org](https://httpbin.org/#/)
6. [Appscode Sample for Timeout Configuration](https://appscode.com/products/voyager/8.0.1/guides/ingress/configuration/default-timeouts/) 

# Future work
_TBD_

# Security Concerns

_TBD_
