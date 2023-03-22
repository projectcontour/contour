## Add support for Global External Authorization for HTTPProxy.

Contour now supports external authorization for all hosts by setting the config as part of the `contourConfig` like so: 

```yaml
globalExtAuth:
  extensionService: projectcontour-auth/htpasswd
  failOpen: false
  authPolicy:
    context:
      header1: value1
      header2: value2
  responseTimeout: 1s
```

Individual hosts can also override or opt out of this global configuration. 
You can read more about this feature in detail in the [guide](https://projectcontour.io/docs/v1.25.0/guides/external-authorization/#global-external-authorization). 