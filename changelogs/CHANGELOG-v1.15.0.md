We are delighted to present version 1.15.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

# Major Changes

## Gateway API
Contour v1.15.0 supports [GatewayAPI v0.3.0](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.3.0) which was released on 04/29/21. Not all of the features in GatewayAPI are fully supported yet in Contour, however, v1.15.0 adds some new features outlined below:
- Filter on Gateway.Listener.Protocol for supported types (#3612): Filters on the protocols that Contour supports.
- Fix processing of httproutes validation (#3598): Fixes a bug where additional HTTPRoutes would not get processed after the first one was processed. 
- Add support for TLS on Gateway (#3472): For HTTPRoutes, TLS can be specified on the Gateway.Listener which terminates TLS at Envoy before proxying to any Kubernetes service.
- Upgrade to gateway-api v0.3.0 (#3623): Contour supports the latest version of gateway-api, v0.3.0.
- Process HTTPRoute.Spec.Gateways (#3618): Implement support for HTTPRoutes.Spec.Gateways to allow an HTTPRoute to define how it binds to a Gateway via "All", "SameNamespace", or "FromList". 
- Update HTTPRoute status to only allow a single Type (#3600): Updates the HTTPRoute status logic to only allow a single condition "Type". Previously multiple Types would be allowed in Contour v1.14.
- Update RBAC permissions for gateway-api types to allow for Status to be updated (#3570): Note this will require re-applying the Contour RBAC permissions for status to work correctly. 

If you're interested in following the GatewayAPI implementation progress in Contour, issue #2287 is the Epic issue which outlines all the features that have been implemented as well as those that are still not yet complete. 

## IPv6
Contour 1.15 improves its IPv6 support. Bootstrap clusters with IP addresses are now configured as STATIC in Envoy, to fix an issue where IPv6 addresses were not properly parsed (#3572). Also, IPv6 addresses are now consistently accepted as flag values (#3579). Thanks @sunjayBhatia for fixing these issues!

## Global rate limiting
Contour 1.15 adds support for the `HeaderValueMatch` rate limiting descriptor type, which allows for generation of descriptor entries based on complex matching against header values. As part of this, "notpresent" is now a supported header match operator (#3599 #3602). For more information, see the [global rate limiting](https://projectcontour.io/docs/v1.15.0/config/rate-limiting/#descriptors--descriptor-entries) documentation. Thanks to @skriss for implementing this feature.

## Ingress wildcard hosts
As part of fully supporting the Ingress v1 API, Contour now supports `Ingresses` with wildcard hosts. The first DNS label in a host name can be a wildcard, e.g. `*.projectcontour.io`, which will match `foo.projectcontour.io` but not `foo.bar.projectcontour.io`. You can read more about the API spec in the [Kubernetes Ingress documentation](https://kubernetes.io/docs/concepts/services-networking/ingress/#hostname-wildcards). This was implemented by @sunjayBhatia in #3381.

## Skipping client cert verification for downstream TLS
`HTTPProxy` now supports skipping verification when client certificates are requested for downstream TLS, via the `spec.virtualhost.tls.clientValidation.skipClientCertValidation` field. This is not enabled by default. This can be used in conjunction with an external authorization server as Envoy will still require client certificates to be supplied, and will pass them along to the external authorization server for verification (#3611). Thanks to @sunjayBhatia for implementing this feature!

## Customizable admin port for shutdown-manager
Contour's shutdown-manager now supports a customizable admin port, via the `--admin-port` flag (#3501). Thanks to @alessandroargentieri for implementing this!

## Envoy 1.18.2
Contour 1.15 is compatible with Envoy 1.18.2 (#3589). For more information, see the [Contour compatibility matrix](https://projectcontour.io/resources/compatibility-matrix/).

## Kubernetes 1.21
Contour 1.15 is supported on Kubernetes 1.21 (#3581 #3609). For more information, see the [Contour compatibility matrix](https://projectcontour.io/resources/compatibility-matrix/).

# Bugs Fixed
- Fixed a bug where default header policies were not being applied unless an `HTTPProxy` also had headers being set (#3550). Thanks @mattatcha for finding and fixing!

# Upgrading
Please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

## Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:
- @nak3
- @mattatcha
- @alessandroargentieri

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
