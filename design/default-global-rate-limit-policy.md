# Default Global RateLimit Policy

Status: Accepted

## Abstract
Define a default global rate limit policy in the Contour configuration to be used as a global rate limit policy by all HTTPProxy objects.

## Background
See https://github.com/projectcontour/contour/issues/5357 for context.

## Goals
- Define a default global L7 rate limit policy which is used by all HTTPProxy on the `VirtualHost` level.

## Non Goals
- Default rate limit policy for Route level `rateLimitPolicy`.
- L4 rate limiting support. 

## High-Level Design
Currently, the global `rateLimitPolicy` for `VirtualHost` is set per HTTPProxy. This option allows each service to configure its own global `rateLimitPolicy`. However, for a single tenant setup/general usecase, this means you have to edit all the HTTPProxy objects with the same ratelimit policies.

A new field, `DefaultGlobalRateLimitPolicy` (optional), will be added to `RateLimitServiceConfig` which is part of `ContourConfigurationSpec`. This field will define rate limit descriptors that will be added to every `VirtualHost`.

HTTPProxy has to opt-out **explicitly** to not use the default global rate limit policy using `rateLimitPolicy.GlobalRateLimitPolicy.disabled` flag and still can have its own global `rateLimitPolicy` which overrides the default one.

### Sample Configurations
#### contour.yaml
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    rateLimitService:
      extensionService: projectcontour/ratelimit
      domain: contour
      failOpen: false
      defaultGlobalRateLimitPolicy:
        descriptors:
          - entries:
              - remoteAddress: {}
          - entries:
              - genericKey:
                  value: foo
```

#### HTTPProxy
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    rateLimitPolicy:
      global:
        descriptors:
          - entries:
            - remoteAddress: {}
          - entries:
            - genericKey:
              value: foo
  routes:
  - conditions:
    - prefix: /
    services:
    - name: ingress-conformance-echo
      port: 80
```

#### HTTPProxy With Local RateLimit
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    rateLimitPolicy:
      global:
        descriptors:
          - entries:
            - remoteAddress: {}
          - entries:
            - genericKey:
              value: foo
      local:
        requests: 100
        unit: hour
        burst: 20
  routes:
  - conditions:
    - prefix: /
    services:
    - name: ingress-conformance-echo
      port: 80
```

#### HTTPProxy Opted-out
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    rateLimitPolicy:
      global:
        disabled: true
      local:
        requests: 100
        unit: hour
        burst: 20
  routes:
  - conditions:
    - prefix: /
    services:
    - name: ingress-conformance-echo
      port: 80
```

## Detailed Design

### Contour Configuration Changes
A new field, `DefaultGlobalRateLimitPolicy` of type `GlobalRateLimitPolicy` (optional), will be added to `RateLimitServiceConfig` as part of `ContourConfigurationSpec`
```go
...
type RateLimitService struct {
	...
	// DefaultGlobalRateLimitPolicy allows setting a default global rate limit policy for all HTTPProxy
	// HTTPProxy can overwrite this configuration.
	DefaultGlobalRateLimitPolicy contour_api_v1.GlobalRateLimitPolicy `yaml:"defaultGlobalRateLimitPolicy,omitempty"`
}
...
```

### HTTPProxy Configuration Changes
A new field `Disabled`, will be added to HTTPProxy `RateLimitPolicy.GlobalRateLimitPolicy`. `GlobalRateLimitPolicy` is part of Contour API v1 definition for HTTPProxy's global `rateLimitPolicy`
```go
// GlobalRateLimitPolicy defines global rate limiting parameters.
type GlobalRateLimitPolicy struct {
  // Disabled configures the HTTPProxy to not use
	// the default global rate limit policy defined by the Contour configuration
	// as a global rate limit policy for its defined virtual hosts entries.
	// +optional
	Disabled bool `json:"disabled,omitempty"`

	// Descriptors defines the list of descriptors that will
	// be generated and sent to the rate limit service. Each
	// descriptor contains 1+ key-value pair entries.
	// +required
	// +kubebuilder:validation:MinItems=1
	Descriptors []RateLimitDescriptor `json:"descriptors,omitempty"`
}

``` 

HTTPProxy processor will check this flag and won't attach the default global rate limit policy as a global rate limit policy in case it is set to `true`.
If `virtualhost` defines its own global `rateLimitPolicy`, `defaultGlobalRateLimitPolicy` won't be considered at all.

## Compatibility
HTTPProxy will opt-out to use the default global rate limit policy explicitly and the `DefaultGlobalRateLimitPolicy` in the Contour configuration is optional. This solution should not introduce any regressions or breaking changes.
