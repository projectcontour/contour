## Default xDS Server Implementation is now Envoy

As of this release, Contour now uses the `envoy` xDS server implementation by default.
This xDS server implementation is based on Envoy's [go-control-plane project](https://github.com/envoyproxy/go-control-plane) and will eventually be the only supported xDS server implementation in Contour.
This change is expected to be transparent to users.

### I'm seeing issues after upgrading, how to I revert to the contour xDS server?

If you encounter any issues, you can easily revert to the `contour` xDS server with the following configuration:

(if using Contour config file)
```yaml
server:
  xds-server-type: contour
```

(if using ContourConfiguration CRD
```yaml
...
spec:
  xdsServer:
    type: contour
```

You will need to restart Contour for the changes to take effect.
