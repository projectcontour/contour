## Default Global RateLimit Policy

This Change adds the ability to define a default global rate limit policy in the Contour configuration 
to be used as a global rate limit policy by all HTTPProxy objects.
HTTPProxy object can decide to opt out and disable this feature using `defaultGlobalRateLimitPolicyDisabled` config.

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
