# Control the conditions of a `RetryPolicy`

Status: Draft

## Abstract

When specifying a `retryPolicy` in an `HTTPProxy`, Envoy is configured to retry all 5xx status codes.
This proposal seeks to add fields to the `RetryPolicy` that give the user the ability to specify the conditions under which a retry should occur (e.g. certain status codes).

## Background

When Kubernetes registers and deregisters Pods to receive network traffic, it takes Contour a non-zero amount of time (less than a second) to update the upstreams in Envoy.
For services with a high number of requests per second (RPS), this can result in a handful of 503s during this period.

At present, users can specify a `retryPolicy` in their `HTTPProxy` that will retry all 5xx status codes.
When configured properly (i.e. with an appropriate number of retries and time between retries) this mitigates the delay of Contour updating Envoy upstreams.
However, retrying all 5xx is heavy handed when one only wants to retry upstream connection errors.

One could simply add an option to the `retryPolicy` that tells Envoy to retry upstream connection errors, but this seems too specific to the problem described above.

One could also expose all the conditional options of retries in Envoy, but this seems too broad a solution, may complicate the `HTTPProxy` spec too much, and may enable users to do more than is reasonable.

Thus, this proposal suggests a way to expose the conditions for retrying a request in a general but limited fashion.
It is designed to be applicable to the specific problem above while also meeting the needs of others who may wish to control the conditions of their retries.
It also tries to avoid exposing too broad a set of configuration options for retries.

## Goals

- Offer a general but limited solution to configurable conditions for retries
- In doing so, solve the particular problem of 503s when Kubernetes endpoints and Envoy upstreams are out of sync (e.g. during the rollout of a Deployment)

## Non Goals

- Support all configurable conditions of an Envoy retry policy
- Support global retry policy that apply to all proxies, with the potential to override per `HTTPProxy` manifest

## High-Level Design

This proposal would add two new fields to the `retryPolicy` -- `retryOn` and `retriableStatusCodes`.
These fields would map to the `retry_on` and `retriable_status_codes` fields of the [Envoy v2 `route.RetryPolicy`](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route_components.proto#route-retrypolicy), respectively.

Below is an example YAML of what this would look like, as an extension of the existing example of a `retryPolicy` from the [`HTTPProxy` reference](https://projectcontour.io/docs/main/httpproxy/#response-timeout):

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: response-timeout
  namespace: default
spec:
  virtualhost:
    fqdn: timeout.bar.com
  routes:
    - timeoutPolicy:
        response: 1s
        idle: 10s
      retryPolicy:
        count: 3
        perTryTimeout: 150ms
        retryOn:
          - connect-failure
        retriableStatusCodes:
          - 503
          - 504
      services:
        - name: s1
          port: 80
```

In the above example, a request would be retried on the following conditions:

- An upstream connection error occurs (connection failure or timeout)
- The response status code is 503 or 504

## Detailed Design

Two new fields will be added to the `v1.RetryPolicy` of the `HTTPProxy` spec:

- `RetryOn []RetryOn`
  Optional
  Slice of [`x-envoy-retry-on`](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#x-envoy-retry-on) conditions
- `RetriableStatusCodes []uint32`
  Optional
  Slice of HTTP status codes

The `RetryOn` field uses a string type alias with kubebuilder validation.
This ensures that the `HTTPProxy` only contains valid values for the `v1.RetryPolicy.RetryOn` field:

```go
// +kubebuilder:validation:Enum=5xx;gateway-error;reset;connect-failure;retriable-4xx;refused-stream;retriable-status-codes;retriable-headers
type RetryOn string
```

One new field will be added to the `dag.RetryPolicy`:

- `RetriableStatusCodes []uint32`
  Slice of HTTP status codes

New logic will be introduced when transforming a `v1.RetryPolicy` into a `dag.RetryPolicy`:

- If `v1.RetryPolicy.RetryOn` is nil or empty, it will be set to the default value of `[]string{"5xx"}` for backwards compatibility
- The values of `v1.RetryPolicy.RetryOn` will be joined into a string, with values separate by commas, and set as the value of `dag.RetryPolicy.RetryOn`
- The value of `v1.RetryPolicy.RetriableStatusCodes` will be set as the value of `dag.RetryPolicy.RetriableStatusCodes`

New logic will be introduced when transforming a `dag.RetryPolicy` into a Envoy v2 `route.RetryPolicy`:

- The value of `dag.RetryPolicy.RetryOn` will be set as the value of `route.RetryPolicy.RetryOn`
- The value of `dag.RetryPolicy.RetriableStatusCodes` will be set as the value of `route.RetryPolicy.RetriableStatusCodes`

No further changes should be necessary to propagate these new configurable fields to Envoy.

## Alternatives Considered

### Exposing a single option for retrying only connection upstream errors

As discussed earlier, we could expose a single option for only retrying connection upstream errors.
Example:

```yaml
retryPolicy:
  count: 3
  perTryTimeout: 150ms
  retryOnConnectFailure: true
```

Arguments for this approach:

- Solves the exact problem encountered and described in [Background](#background)

Arguments against this approach:

- Too specific to the author's problem, not general enough to apply to other use cases
- This specific design for `HTTPProxy` is too short sighted and does not provide a path adding additional conditions in an elegant way

### Explicit booleans for every valid `retry_on` value for Envoy

Instead of a list of strings for `retryOn`, we could make every possible value a unique boolean field.
Example:

```yaml
    retryPolicy:
      count: 3
      perTryTimeout: 150ms
      retryOn:
        5xx: false
      	connectFailure: true
      	gatewayError: false
      	...
```

Arguments for this approach:

- Contour can control the options that it will allow users to populate Envoy's `retry_on` field with
- It ensures that only valid values are allowed in Envoy's `retry_on` field

Arguments against this approach:

- Contour becomes responsible for keeping track of and maintaining every possible value for Envoy's `retry_on` field
- Seems too verbose compared to a list of strings, but this may be a subjective opinion

### Global retry policy for all proxies

Instead of configuring retry policies per `HTTPProxy`, we could allow configuration of a global retry policy that applies to all proxies.
This would solve the author's issue of mitigating upstream connect errors during rollouts without having to configure all `HTTPProxy` manifests with the same retry policy.

If implemented, this global retry policy would live in Contour's [global configuration file](https://projectcontour.io/docs/main/configuration/) and likely resemble the same structure at the `retryPolicy` section of the `HTTPProxy` manifest.

Taking this approach would require consideration around how retry policies defined in `HTTPProxy` would consolidate with a global retry policy:

- Would the proxy-specific retry policy override or somehow merge with the global retry policy?
  (For example, the `RetryOn` field could be a good candidate for merging elements.)
- Would certain fields of the retry policy override, while others merge?

The desired behavior here does not seem clear cut enough to apply to all use cases for retry policies, therefore it seems more reasonable to forego global retry policies for the time being until the point if/when a standard or common use case becomes clear.

Unfortunately this may result in multiple `HTTPProxy` manifests sharing the same retry policy. However, existing tools such as Helm are well equipped to manage and this type of repetition.

## Security Considerations

### Multiple retry policies at scale

With tens of hundreds of `HTTPProxy` manifests, each with their own retry policy, it's important to consider the impact that many defined retry policies will have at scale.

One concern is the possibility that retry policies might conflict with one another.
This is a non-issue, as each `HTTPProxy` creates unique routes in Envoy, and the retry policies specified therein will exist and operate independently of each other without conflict, allowing thousands of these policies to coexist without conflict.

Another concern is the impact that many retry policies can have on the performance of Envoy -- can many retries negatively impact Envoy's performance?
Fortunately, Envoy uses circuit breakers which limit the potential for negative performance from an increase in requests.
In Envoy access logs, requests that breach a circuit breaker include the response flag `UO`.
You'll see this response flag show up for requests that were the result of a retry policy, thus Envoy also applies circuit breakers to retry policies.

## Compatibility

The new fields of the `retryPolicy` -- `retryOn` and `retriableStatusCodes` -- are both optional, so existing `HTTPProxy` manifests will be syntactically compatible.

Furthermore, a nil or empty value for `retryOn` will result in a default value that is backwards compatible with Contour's existing logic: retrying all 5xx status codes.

## Open Issues

- https://github.com/projectcontour/contour/issues/2369
