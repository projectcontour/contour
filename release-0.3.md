# Contour 0.3

Heptio is pleased to announce the release of Contour 0.3.

## New and improved

The headline features of this release are:

### TLS support

Contour now supports HTTP and HTTPS ingress.
Contour TLS support works with the [standard ingress object][1].
You can read more about [Contour's TLS support here][2].  

### More supported annotations

Support is added for the following annotations on the `service` and `ingress` objects:

- `kubernetes.io/ingress.allow-http: "false"` removes the ingress configration from the non TLS listener. Envoy does not serve traffic for your ingress's vhost on port 80.
- `ingress.kubernetes.io/force-ssl-redirect: "true"` causes Envoy to issue an unconditional 301 redirect to the HTTPS version of your site.

### gRPC API is now GA

Support for Envoy's version 2 gRPC based API, introduced in Contour 0.2, is now marked GA.
The REST API does not support new Envoy features such as SNI.
Support for the REST API is deprecated and will be removed in Contour 0.4.

### Other improvements in this release

- Contour no longer sends updates to Envoy periodically. Changes in the Kubernetes API are streamed to Envoy as they occur. In a steady state, no traffic flows from Contour to Envoy.
- The address and port for Envoy's HTTP and HTTPS listeners are now configurable. This will be useful for anyone using a Daemonset with host networking. This can also be used to force Envoy to bind to IPv6.
- Contour now supports the PROXY protocol to recover the remote IP of connections via an ELB in TCP mode. To enable this, add the `--use-proxy-protocol` flag to the flags for your `contour` Deployment or Daemonset.
- Update to client-go release 6.

## Bug fixes (compared to Contour 0.2.1)

- The `glog` library is now properly initalised on `contour serve` startup. Fixes #113. Thanks @willmadison 

## Upgrading

Contour 0.3 makes the YAML v2 bootstrap configuration format the default.
In Contour 0.4 the JSON v1 bootstrap configuration option will be removed.
Consult the [upgrade notes][0] for how to update your Deployment or Daemonset manifests to the YAML bootstrap configuration format.

[0]: docs/upgrade.md
[1]: https://kubernetes.io/docs/concepts/services-networking/ingress/#tls
[2]: docs/tls.md
