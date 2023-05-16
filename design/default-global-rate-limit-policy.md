# Default Global RateLimit Policy

Status: Reviewing

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

HTTPProxy has to opt-in **explicitly** to use the default global rate limit policy using `rateLimitPolicy.defaultGlobalRateLimitPolicyEnabled` flag and still can have its own global `rateLimitPolicy` which overrides the default one.

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
      defaultGlobalRateLimitPolicyEnabled: true
  routes:
  - conditions:
    - prefix: /
    services:
    - name: ingress-conformance-echo
      port: 80
```

## Detailed Design

### Contour Configuration changes

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

### HTTPProxy Configuration changes
A new field `DefaultGlobalRateLimitPolicyEnabled` (optional), will be added to HTTPProxy `RateLimitPolicy`
```go
...
type RateLimitPolicy struct {
  ...
	// DefaultGlobalRateLimitPolicyEnabled configures the HTTPProxy to use
	// the default global rate limit policy defined by the Contour configuration
	// as a global rate limit policy for its defined virtual hosts entries.
	// +optional
	DefaultGlobalRateLimitPolicyEnabled bool `json:"defaultGlobalRateLimitPolicyEnabled,omitempty"`
}
...
```

HTTPProxy processor will check this flag and attach the default global rate limit policy as a global rate limit policy in case it is set to `true`.

## Compatibility
HTTPProxy will opt-in to use the default global rate limit policy optionally and the `DefaultGlobalRateLimitPolicy` in the Contour configuration is optional. This solution should not introduce any regressions and is backward-compatible.
