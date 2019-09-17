**note: do not commit this file, copy and paste the text into the release page**

# Contour 1.0.0-beta.1
VMware is proud to present version 1.0.0-beta.1 of Contour, our layer 7 HTTP reverse proxy for Kuberentes clusters. As always, without the help of the many community contributors this release would not have been possible. Thank you!

Contour 1.0.0-beta.1 is a beta release. The current stable release at this time is Contour 0.15.0.

## New and improved 

Contour 1.0.0-beta.1 contains many bug fixes and improvements.

### HTTPProxy CRD

TODO(dfc,stevesloka) describe HTTPProxy

### IngressRoute deprecation

With the introduction of HTTPProxy, IngressRoute CRD is now marked as deprecated. 

The IngressRoute CRD will be supported in its current state until the Contour 1.0.0 release and will be removed shortly after.

### CLF logging

TODO(youngnick) please document the CLF default.

### JSON logging

Contour now supports JSON formatted logs.

Please see the [documention](/docs/structured-logs.md) and [design document](/design/envoy-json-logging.md) for more information.

Fixes #624. Thanks @youngnick. 

### Leadership improvements

TODO(youngnick) please document the improvements to leadership elections.

### Contour image registry changes

Contour's image registry has moved from `gcr.io/hepto-images/contour` to `docker.io/projectcontour/contour`.

The `v1.0.0-beta.1` tag is only available in `docker.io/projectcontour/contour`. 

For convenience the `:v0.15.0` and `:latest` tags are available in both repositories. Once Contour 1.0.0 final is release the `:latest` tag will move to `docker.io/projectcontour/contour`. Even if you are remaiing on `:latest` or `:v0.15.0` until the final release of Contour 1.0.0 please update your image locations to `docker.io/projectcontour/contour:v0.15.0` or `docker.io/projectcontour/contour:latest` respectively.

### GitHub organization changes

Contour's source code has moved from `github.com/heptio/contour` to `github.com/projectcontour/contour`.

GitHub is pretty good about redirecting people for a time, but eventually the `github.com/heptio` organization will go away and redirects will cease. Please update your bookmarks.

### Contour namespace changes

Contour's default namespace has changed from `heptio-contour` to `projectcontour`.

### Deprecated `examples/`

Several of the `examples/` sample manifests have been removed as part of the preparations for the 1.0.0 release.

### Contour will no longer serve an a broken TLS virtualhost over HTTP

In the case where an IngressRoute had a missing or invalid TLS secret Contour would serve the IngressRoute over HTTP. Contour now detects the case where a TLS enabled IngressRoute is missing its certificate and will not present the virtualhost over HTTP or HTTPS.

Fixes #1452

### TLS Passthrough and HTTP redirect

Under certain circumstances it is now possible to combine TLS passthrough on port 443 with port 80 served from the same service. The use case for this feature is the application on port 80 can provide a helpful message when the service on port 443 does not speak HTTPS.

For more information see #910 and #1450.

### Per route traffic mirroring

Per route a service can be nominated as a mirror. The mirror service will receive a copy of the read traffic sent to any non mirror service. The mirror traffic is considered _read only_, any response by the mirror will be discarded.

Fixes #459

### Contour ignores unrelated Secrets

Contour now ignores Secrets which are not related to Ingress, IngressRoute, HTTPProxy, or TLSCertificateDelegation operations.
This substantially reduces the number of updates processed by Contour.

Fixes #1372

### Contour filters Endpoint updates

Contour now supports filtering update notifications in some circumstances. Specifically Envoy's EDS watches will no longer fire unless the specific EDS entry requested is updated. This should significantly reduce the number of spurious EDS updates send to Envoy.

Updates #426, #499

### Minor improvements

- The `contour` binary now executes a graceful shutdown when sent SIGTERM. Thanks @alexbrand. Fixes #1364.
- Contour now preserves the `X-Request-Id` header if present. Fixes #1509.
- Contour's quickstart documentation now references the current stable version of Contour. Fixes #952.
- Contour will no longer present a secret via SDS if that secret is not referenced by a valid virtualhost. #1165
- The `envoyproxy/go-control-plane` package has nbeen upgraded to version 0.9.0. `go-control-plane` 0.9.0 switches to the `google/protobuf` library which results in a 4mb smaller binary. Neat.
- Our `CONTRIBUTING` documentation has been updated to encourage contributors to squash their commits. Thanks @stevesloka.
- The markup of several of our pages has been corrected to render properly on GitHub. Thanks @sudeeptoroy.
- Envoy's `/healthz` endpoint has been replaced with `/ready` for Pod readiness. Fixes #1277. Thanks @rochacon.
- IngressRoute objects now forbid `*` anywhere in the `spec.virtualhost.fqdn` field. Fixes #1234.
- Contour is built with Go 1.13.

## Bug fixes

- Contour now rejects IngressRoute and HTTPProxy objects that delegate to another root IngressRoute or HTTPProxy object. Fixes #865.
- An error where IngressRoute's status is not set when it references an un-delegated TLS cert has been fixed. Fixes #1347.

## Upgrading

Please consult the [Upgrading](/docs/upgrading.md) document for further information on upgrading from Contour 0.15 to Contour 1.0.0-beta.1
