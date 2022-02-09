# Annotations Reference

<div id="toc" class="navigation"></div>

Annotations are used in Ingress Controllers to configure features that are not covered by the Kubernetes Ingress API.

Some of the features that have been historically configured via annotations are supported as first-class features in Contour's [HTTPProxy API][15], which provides a more robust configuration interface over annotations.

However, Contour still supports a number of annotations on the Ingress resources.

## Standard Kubernetes Ingress annotations

The following Kubernetes annotations are supported on `Ingress` objects:

### Ingress Class

The Ingress class annotation can be used to specify which Ingress controller should serve a particular Ingress object.
This annotation may be specified as the standard `kubernetes.io/ingress.class` or a Contour-specific `projectcontour.io/ingress.class`.
In both cases, they will behave as follows, by default:

* If not set, then all Ingress controllers serve the Ingress.
* If specified as `kubernetes.io/ingress.class: contour`, then Contour serves the Ingress.
* If any other value, Contour ignores the Ingress definition.

You can override the default class `contour` by providing the `--ingress-class-name` flag to Contour. 
This can be useful while you are migrating from another controller, or if you need multiple instances of Contour.
If you do this, the behavior is as follows:
* If the annotation is not set, Contour will ignore the Ingress.
* If the annotation is set to any value other than the one passed to the `--ingress-class-name` flag, Contour will ignore the Ingress.
* If the annotation matches the value that you passed to `--ingress-class-name` flag, Contour will serve the Ingress.

This same logic applies for these annotations on HTTPProxy objects.

_Note: Both `Ingress` and `HTTPProxy` now have an `IngressClassName` field in their spec. Going forward this is the preferred way to specify an ingress class, rather than using an annotation. If both the annotation and the spec field are specified on an object, the annotation takes preference for backwards compatibility._

_Note: The `--ingress-class-name` value can be a comma-separated list of class names to match against.  Contour will serve the Ingress or HTTPProxy if the annotation or IngressClassName matches any of the specified class name values.

### Other annotations 

 - `ingress.kubernetes.io/force-ssl-redirect`: Requires TLS/SSL for the Ingress to Envoy by setting the [Envoy virtual host option require_tls][16].
 - `kubernetes.io/ingress.allow-http`: Instructs Contour to not create an Envoy HTTP route for the virtual host. The Ingress exists only for HTTPS requests. Specify `"false"` for Envoy to mark the endpoint as HTTPS only. All other values are ignored.

The `ingress.kubernetes.io/force-ssl-redirect` annotation takes precedence over `kubernetes.io/ingress.allow-http`. If they are set to `"true"` and `"false"` respectively, Contour *will* create an Envoy HTTP route for the Virtual host, and set the `require_tls` virtual host option.

## Contour specific Ingress annotations

 - `projectcontour.io/ingress.class`: The Ingress class that should interpret and serve the Ingress. See the [main Ingress class annotation section](#ingress-class) for more details.
 - `projectcontour.io/num-retries`: [The maximum number of retries][1] Envoy should make before abandoning and returning an error to the client. Applies only if `projectcontour.io/retry-on` is specified. Set to -1 to disable retries.
 - `projectcontour.io/per-try-timeout`: [The timeout per retry attempt][2], if there should be one. Applies only if `projectcontour.io/retry-on` is specified.
 - `projectcontour.io/response-timeout`: [The Envoy HTTP route timeout][3], specified as a [golang duration][4]. By default, Envoy has a 15 second timeout for a backend service to respond. Set this to `infinity` to specify that Envoy should never timeout the connection to the backend. Note that the value `0s` / zero has special semantics for Envoy.
 - `projectcontour.io/retry-on`: [The conditions for Envoy to retry a request][5]. See also [possible values and their meanings for `retry-on`][6].
 - `projectcontour.io/tls-minimum-protocol-version`: [The minimum TLS protocol version][7] the TLS listener should support. Valid options are `1.3`, `1.2` (default), `1.1`.
 - `projectcontour.io/websocket-routes`: [The routes supporting websocket protocol][8], the annotation value contains a list of route paths separated by a comma that must match with the ones defined in the `Ingress` definition. Defaults to Envoy's default behavior which is `use_websocket` to `false`.
 - `projectcontour.io/tls-cert-namespace`: The namespace where all TLS secrets of this Ingress are searched. This is necessary to use [TLS Certificate Delegation][18] with Ingress v1 because the slash notation (ex: different-ns/app-cert) used by HTTPProxy and Ingress v1beta1 is not accepted. See [this issue][19] for details.

## Contour specific Service annotations

A [Kubernetes Service][9] maps to an [Envoy Cluster][10]. Envoy clusters have many settings to control specific behaviors. These annotations allow access to some of those settings.

- `projectcontour.io/max-connections`: [The maximum number of connections][11] that a single Envoy instance allows to the Kubernetes Service; defaults to 1024.
- `projectcontour.io/max-pending-requests`: [The maximum number of pending requests][13] that a single Envoy instance allows to the Kubernetes Service; defaults to 1024.
- `projectcontour.io/max-requests`: [The maximum parallel requests][13] a single Envoy instance allows to the Kubernetes Service; defaults to 1024
- `projectcontour.io/max-retries`: [The maximum number of parallel retries][14] a single Envoy instance allows to the Kubernetes Service; defaults to 3. This is independent of the per-Kubernetes Ingress number of retries (`projectcontour.io/num-retries`) and retry-on (`projectcontour.io/retry-on`), which control whether retries are attempted and how many times a single request can retry.
- `projectcontour.io/upstream-protocol.{protocol}` : The protocol used to proxy requests to the upstream service.
  The annotation value contains a comma-separated list of port names and/or numbers that must match with the ones defined in the `Service` definition.
  This value can also be specified in the `spec.routes.services[].protocol` field on the HTTPProxy object, where it takes precedence over the Service annotation.
  Supported protocol names are: `h2`, `h2c`, and `tls`:
  - The `tls` protocol allows for requests which terminate at Envoy to proxy via TLS to the upstream.
    This protocol should be used for HTTP/1.1 services over TLS.
    _Note that validating the upstream TLS certificate requires additionally setting the [validation][17] field._
  - The `h2` protocol proxies requests to the upstream using HTTP/2 over TLS.
  - The `h2c` protocol proxies requests to the the upstream using cleartext HTTP/2.

## Contour specific HTTPProxy annotations
- `projectcontour.io/ingress.class`: The Ingress class that should interpret and serve the HTTPProxy. See the [main Ingress class annotation section](#ingress-class) for more details.

[1]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#config-http-filters-router-x-envoy-max-retries
[2]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-retrypolicy-retry-on
[3]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-timeout
[4]: https://golang.org/pkg/time/#ParseDuration
[5]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-retrypolicy-retry-on
[6]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/router_filter#config-http-filters-router-x-envoy-retry-on
[7]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/common.proto.html#extensions-transport-sockets-tls-v3-tlsparameters
[8]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-routeaction-upgrade-configs
[9]: https://kubernetes.io/docs/concepts/services-networking/service/
[10]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/intro/terminology
[11]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/circuit_breaker.proto#envoy-v3-api-field-config-cluster-v3-circuitbreakers-thresholds-max-connections
[12]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/circuit_breaker.proto#envoy-v3-api-field-config-cluster-v3-circuitbreakers-thresholds-max-pending-requests
[13]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/circuit_breaker.proto#envoy-v3-api-field-config-cluster-v3-circuitbreakers-thresholds-max-requests
[14]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/cluster/v3/circuit_breaker.proto#envoy-v3-api-field-config-cluster-v3-circuitbreakers-thresholds-max-retries
[15]: fundamentals.md
[16]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#envoy-v3-api-field-config-route-v3-virtualhost-require-tls
[17]: api/#projectcontour.io/v1.UpstreamValidation
[18]: ../config/tls-delegation/
[19]: https://github.com/projectcontour/contour/issues/3544