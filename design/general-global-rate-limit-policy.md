# General Global RateLimit Policy

Status: Draft

## Abstract
Define a general rate limit policy in the Contour configuration to be used as a global rate limit policy by all HTTPProxy.

## Background
See https://github.com/projectcontour/contour/issues/5357 for context.

## Goals
- Define a general L7 global rate limit policy which is used by all HTTPProxy on the `VirtualHost` level.

## Non Goals
- General rate limit policy for Route level `rateLimitPolicy`.
- L4 rate limiting support. 

## High-Level Design
Currently, the global `rateLimitPolicy` for `VirtualHost` is set per HTTPProxy. This option allows each service to configure its own global `rateLimitPolicy`. However, for a single tenant setup/general usecase, this means you have to edit all the HTTPProxy objects with the same ratelimit policies.

A new field, `GeneralRateLimitPolicy` (optional), will be added to `RateLimitServiceConfig` which is part of `ContourConfigurationSpec`. This field will define rate limit descriptors that will be added to every `VirtualHost`.

HTTPProxy has to opt-in **explicitly** to use the general rate limit policy using `rateLimitPolicy.generalRateLimitPolicyEnabled` flag and still can have its own global `rateLimitPolicy` which overrides the general one.

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
      generalRateLimitPolicy:
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
      generalRateLimitPolicyEnabled: true
  routes:
  - conditions:
    - prefix: /
    services:
    - name: ingress-conformance-echo
      port: 80
```

## Detailed Design

### Contour Configuration changes

A new field, `GeneralRateLimitPolicy` of type `GlobalRateLimitPolicy` (optional), will be added to `RateLimitServiceConfig` as part of `ContourConfigurationSpec`
```go
...
type RateLimitService struct {
	...
	// GeneralRateLimitPolicy allows setting global rate limit policy for all HTTPProxy
	// HTTPProxy can overwrite this configuration.
	GeneralRateLimitPolicy contour_api_v1.GlobalRateLimitPolicy `yaml:"generalRateLimitPolicy,omitempty"`
}
...
```

### HTTPProxy Configuration changes
A new field `GeneralRateLimitPolicyEnabled` (optional), will be added to HTTPProxy `RateLimitPolicy`
```go
...
type RateLimitPolicy struct {
  ...
	// GeneralRateLimitPolicyEnabled configures the HTTPProxy to use
	// the general rate limit policy defined by the Contour configuration
	// as a global rate limit policy for its defined virtual hosts entries.
	// +optional
	GeneralRateLimitPolicyEnabled bool `json:"generalRateLimitPolicyEnabled,omitempty"`
}
...
```

HTTPProxy processor will check this flag and attach the general rate limit policy as a global rate limit policy in case it is set to `true`.

## Compatibility
HTTPProxy will opt-in to use the general rate limit policy optionally and the `GeneralRateLimitPolicy` in the Contour configuration is optional. This solution should not introduce any regressions and is backward-compatible.
