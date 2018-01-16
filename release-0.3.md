# Contour 0.3

Heptio is pleased to announce the release of Contour 0.3.

## New and improved

The headline features of this release are,

### TLS support

Contour now supports HTTP and HTTPS ingress.
Contour TLS support works with the [standard ingress object][1].
You can read more about [Contour's TLS support here][2].  

### More supported annotations

Support for the following annotations on the `service` and `ingress` object has been added in this release.

- `kubernetes.io/ingress.allow-http: "false"` will remove the ingress configration from the non TLS listener. Envoy will not serve traffic for your ingress's vhost on port 80.
- `ingress.kubernetes.io/force-ssl-redirect: "true"` will cause Envoy to issue an unconditional 301 redirect to the HTTPS version of your site.

### gRPC API is not GA

Support for Envoy's version 2 gRPC based API, introduced in Contour 0.2, has been marked GA.
The previous REST API does not support new Envoy features such as SNI.
The REST API is now deprecated and will be removed completely in Contour 0.4.

### Other improvements in this release

- Contour no longer sends updates to Envoy periodically. Changes in the Kubernetes API are streamed to Envoy as they occur. In a steady state, no traffic flows from Contour to Envoy.
- The address and port for Envoy's HTTP and HTTPS listeners are now configurable. This will be useful for anyone using a daemonset with host networking. This can also be used to force Envoy to bind to IPv6.
- Contour now supports the PROXY protocol to recover the remote IP of connections via an ELB in TCP mode. To enable this, add the `--use-proxy-protocol` flag to your `contour` deployment or daemonset's container's flags.
- Update to client-go release 6.

## Bug fixes (compared to Contour 0.2.1)

- The `glog` library is now properly initalised on `contour serve` startup. Fixes #113. Thanks @willmadison 

## Upgrading

Contour 0.3 has made the YAML v2 bootstrap configuration format the default.
In Contour 0.4 the JSON v1 bootstrap configuration option will be removed.
Please consult the [upgrade notes][0] for how to update your deployment or daemonset manifests to the YAML bootstrap configuration format.

[0]: docs/upgrade.md
[1]: https://kubernetes.io/docs/concepts/services-networking/ingress/#tls
[2]: docs/tls.md
