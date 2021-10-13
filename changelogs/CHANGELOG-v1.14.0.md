We are delighted to present version 1.14.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

# Major Changes

## Global Rate Limiting

This release adds a new boolean configuration that defines whether to include the `X-RateLimit` headers `X-RateLimit-Limit`, `X-RateLimit-Remaining`, and `X-RateLimit-Reset` (as defined by this [IETF Internet-Draft](https://tools.ietf.org/id/draft-polli-ratelimit-headers-03.html)), on responses to clients when the Rate Limit Service is consulted for a request.

Related Issues and PRs: #3431 #3457

This release also fixes a bug whereby applying a `rateLimitPolicy` on an HTTPProxy using a `genericKey` entry, the key field of the `genericKey` was ignored and the default value (`generic_key`) was passed on to the RLS.

Related Issues and PRs: #3443 #3445

## Ingress v1 Support

Contour now supports filtering Ingress resources using the [IngressClass](https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-class) name in the Ingress spec. Previously, users could only select certain Ingress resources with the now-deprecated annotation. Support for using the annotation to specify an IngressClass name to watch has not been removed. The existing `contour serve` `--ingress-class-name` flag can still be used to specify an IngressClass name to use as an Ingress filter. The rules around this flag are as follows:
- If the flag is not passed to `contour serve` Contour will accept any Ingress resource that specifies the IngressClass name `contour` in annotation or spec fields or does not specify one at all.
- If the flag is passed to `contour serve` Contour will only accept Ingress resources that exactly match the specified IngressClass name via annotation or spec field, with the value in the annotation taking precedence

Related Issues and PRs: #2146 #3520 

Users can now specify different path matching modes for HTTP requests according to the [Ingress v1 spec](https://kubernetes.io/docs/concepts/services-networking/ingress/#path-types). Contour supports `Exact` path matching that will match a request path exactly, `Prefix` path matching that will match path prefixes, and an `ImplementationSpecific` path matching type.

`Prefix` path matches follow the Ingress spec and do not match using a "string prefix" but rather a path segment prefix (e.g. a prefix `/foo/bar` will match the path `/foo/bar/baz` but not `/foo/barbaz`). This is a difference from Contour's existing Ingress support which implemented "string prefix" matching for all path matches. For backwards compatibility the `ImplementationSpecific` match type retains the existing pre-1.14.0 behavior and will do a "string prefix" match if a plain path is specified or a regex match if a path is specified that contains regex meta-characters. Users who did not specify a path matching type on their Ingress resources should require no intervention as the API server defaults those rules to `ImplementationSpecific`, however anyone using an explicit `Prefix` match may have to review their Ingress resources to ensure "segment prefix" matches work for them.

Related Issues and PRs: #2135 #3471

## Default Header Policy

Contour can be configured to set or remove HTTP request and response headers by default through parameters in the config file. These defaults will apply to all HTTP requests and responses unless overridden by users in an HTTPProxy.

Related Issues and PRs: #3258 #3270 #3519

## Bootstrap Generated SDS Resources Permissions

SDS resources written by the `contour bootstrap` command are now written with more a permissive mode to ensure Envoy running as a non-root user is able to access them.

Related Issues and PRs: #3264 #3390

## Port Stripped from Hostname Sent to Upstreams

HTTP requests with a port in the hostname header are now configured to be stripped by Envoy in internal processing and when forwarded to upstream services.

Related Issues and PRs: #3458

## Example Deployment Envoy Service Ports

The Envoy service in the Contour example deployment YAML has been updated to use target ports of `8080` and `8443` (replacing the original ports `80` and `443`). Contour will also configure Envoy to now use those ports (its default values) for HTTP and HTTPS listener ports.

Related Issues and PRs: #3393

# Deprecation & Removal Notices
- The following Prometheus Gauges have been removed in favor of Gauges added in Contour 1.13.0 with more idiomatic names. Any dashboard and alert queries referring to the old names must be updated to use the new metrics.
   ```
   contour_httpproxy_total -> contour_httpproxy
   contour_httpproxy_invalid_total  -> contour_httpproxy_invalid
   contour_httpproxy_orphaned_total  -> contour_httpproxy_orphaned
   contour_httpproxy_valid_total  -> contour_httpproxy_valid
   contour_httpproxy_root_total  -> contour_httpproxy_root
   ```

# Upgrading
Please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

## Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For version 1.14, special thanks go out to the following contributors:
- @abhide
- @erwbgy
- @furdarius 
- @nak3 
- @arthurlm 
- @prondubuisi 

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
