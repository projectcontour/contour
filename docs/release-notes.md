# Contour 1.0.0-rc.1

VMware is proud to present version 1.0.0-rc.1 of Contour, our layer 7 HTTP reverse proxy for Kuberentes clusters. As always, without the help of the many community contributors this release would not have been possible. Thank you!

Contour 1.0.0-rc.1 is the first release candidate on the path to Contour 1.0.

The current stable release at this time remains Contour 0.15.1.

## New and improved 

Contour 1.0.0-rc.1 contains many bug fixes and improvements, and moves the HTTPProxy CRD to v1.

### HTTPProxy CRD v1

Contour 1.0.0-rc.1 promotes the HTTPProxy CRD to v1. HTTPProxy is now considered stable and our sincere hope is that with the move v1 any changes to the CRD will be done in a backwards compatible manner.  

The move from alpha1 to v1 has resulted in changes to per service health checking, load balancing strategy, and per route prefix rewriting.

Please see the [upgrading document](/docs/upgrading.md) and [HTTPProxy documentation](/docs/httpproxy.md) for advice on upgrading HTTPProxy alpha1 CRDs to v1. 

#### Prefix rewrite support removed

HTTPProxy v1 removes prefix rewriting support. The feature as it was implemented in alpha1, and IngressRoute before it, was badly designed and it was not possible to address its limitations without a backwards incompatible change. Our intention is to design a more capable prefix rewrite replacement. 

Prefix rewrite support continues to exist in the deprecated IngressRoute CRD. We won't be removing IngressRoute support until we have a replacement for prefixRewriting available in HTTPProxy.

Please follow #899 for the status of this issue.

### networking.k8s.io/v1beta1 Ingress support

Support for the networking.k8s.io/v1beta1.Ingress object has been added.

Fixes #1685

### `contour.heptio.com` annotations deprecated

As part of the move to the `projectcontour.io` namespace the Heptio branded `contour.heptio.com` annotations have been migrated to their respective `projectcontour.io` versions. The previous `contour.heptio.com` annotations should be considered deprecated. Contour will continue to be supported these deprecated forms for the moment. They will be removed at some point after Contour 1.0.

## Client request timeout

The ability to specify a Contour wide request timeout has been added to the configuration file.

See the [configuration file example](/examples/contour/01-contour-config.yaml#L14) for more information.

Fixes #1073. Thanks @youngnick.

## TLS certificate validation

Contour 0.15.1 now attempts to validate the contents of a TLS certificate before presenting it to Envoy.
This validation only extends to asserting the certificate is well formed. Expired, incorrect hostname details, or otherwise well formed but invalid certificates are not rejected. IngressRoutes that reference invalid secrets will have their `Status: ` fields set accordingly. 

Fixes #1065

## Envoy 1.11.2

[See the Envoy 1.11.2 announcement for details on the vulnerabilities](https://groups.google.com/forum/#!topic/envoy-announce/Zo3ZEFuPWec).

### Minor improvements

- `make help` target added. Thanks @jpeach.
- `prefix` conditions must start with a slash. Fixes #1628. Thanks @youngnick.
- Duplicate HTTPProxy `header` conditions are now rejected. Fixes #1559. Thanks @youngnick.
- HTTPProxy `route` or `include` blocks with more than one `prefix` condition are now rejected. Fixes #1611. Thanks @stevesloka.
- The `X-Request-Id` header is now no longer sanitized. Fixes #1487.
- `HTTPProxy` `include`s no longer require a `namespace` key. If no `namespace` is provided, the included HTTPProxy is inferred to be in the same namespace as its parent. Fixes #1574. Thanks @youngnick.

## Bug fixes

### Minor bug fixes

- `prefix` conditions no longer strip trailing slashes. Fixes #1597. Thanks @youngnick.
- TCPProxy support now works with HTTPProxy. Fixes #1626. Thanks @stevesloka.
- HTTPProxy TLSCertificateValidation was borken in beta.1, now it's not. Fixes #1639. Thanks @stevesloka.
- We have published a [supported release version policy](/docs/support.md). Fixes #1581.

## Upgrading

Please consult the [Upgrading](/docs/upgrading.md) document for further information on upgrading from Contour 0.15.1 to Contour 1.0.0-rc.1