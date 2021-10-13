We are delighted to present version 1.16.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

# Major Changes

## Gateway API

### Added Initial Support for TLSRoute
PR https://github.com/projectcontour/contour/pull/3627 added support for TLSRoute to enable Passthrough TCP Proxying to pods via SNI. See https://github.com/projectcontour/contour/issues/3440 for additional information on TLSRoute feature requirements.

### GatewayClass Support
PR https://github.com/projectcontour/contour/pull/3659 added GatewayClass support through the gateway.controllerName configuration parameter. The gateway.namespace and gateway.name parameters are required when setting gateway.controllerName. When the cluster contains Gateway API CRDs and this parameter is set, Contour will reconcile GatewayClass resources with the spec.controller field that matches gateway.controllerName. ControllerName should take the form of `projectcontour.io/<namespace>/contour`, where `<namespace>` is the namespace that Contour runs in.

## CA is No Longer Ignored when Downstream "Skip Verify" is True
With PR https://github.com/projectcontour/contour/pull/3661, Contour will no longer ignore a certificate under the following conditions:
  - If no CA is set and "skip verify" is true, client certs are not required by Envoy.
  - If CA set and "skip verify" is true, client certs are required by Envoy.
  - CA is still required if "skip verify" is false.
  - `caCert` is now optional since skipClientCertValidation can be true. PR https://github.com/projectcontour/contour/pull/3658 added an `omitempty` JSON tag to omit the `caCert` field when serializing to JSON and it hasn't been specified.  

## Website Update
PR https://github.com/projectcontour/contour/pull/3704 revamps the Contour website based on [Hugo](https://gohugo.io/). Check out the fresh new look and tell us what you think.

# Deprecation & Removal Notices
- PR https://github.com/projectcontour/contour/pull/3642 removed the `experimental-service-apis` flag has been removed. The gateway.name & gateway.namespace in the Contour configuration file should be used for configuring Gateway API (formerly Service APIs).
- PR https://github.com/projectcontour/contour/pull/3645 removed support for Ingress v1beta1. Ingress v1 resources should be used with Contour.

# Upgrading
Please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

## Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:
- @geoffcline 
- @pandeykartikey
- @pyaillet

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
