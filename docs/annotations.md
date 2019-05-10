# Annotations

Annotations are used in Ingress Controllers to configure features that are not
covered by the Kubernetes Ingress API.

Some of the features that have been historically configured via annotations are
supported as first-class features in Contour's [IngressRoute
API](ingressroute.md), which provides a more robust configuration interface over
annotations.

However, Contour still supports a number of annotations on the Ingress resources.

## Standard Kubernetes Ingress annotations

 - `kubernetes.io/ingress.class`: The Ingress class that should interpret and serve the Ingress. If not set, then all Ingress controllers serve the Ingress. If specified as `kubernetes.io/ingress.class: contour`, then Contour serves the Ingress. If any other value, Contour ignores the Ingress definition. You can override the default class `contour` with the `--ingress-class-name` flag at runtime. This can be useful while you are migrating from another controller, or if you need multiple instances of Contour.
 - `ingress.kubernetes.io/force-ssl-redirect`: Requires TLS/SSL for the Ingress to Envoy by setting the [Envoy virtual host option require_tls](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html#envoy-api-field-route-virtualhost-require-tls)
 - `kubernetes.io/ingress.allow-http`: Instructs Contour to not create an Envoy HTTP route for the virtual host. The Ingress exists only for HTTPS requests. Specify `"false"` for Envoy to mark the endpoint as HTTPS only. All other values are ignored.

The `ingress.kubernetes.io/force-ssl-redirect` annotation takes precedence over `kubernetes.io/ingress.allow-http`. If they are set to `"true"` and `"false"` respectively, Contour *will* create an Envoy HTTP route for the Virtual host, and set the `require_tls` virtual host option.

## Contour specific Ingress annotations

 - `contour.heptio.com/ingress.class`: The Ingress class that should interpret and serve the Ingress. If not set, then all Ingress controllers serve the Ingress. If specified as `contour.heptio.com/ingress.class: contour`, then Contour serves the Ingress. If any other value, Contour ignores the Ingress definition. You can override the default class `contour` with the `--ingress-class-name` flag at runtime. This can be useful while you are migrating from another controller, or if you need multiple instances of Contour.
 - `contour.heptio.com/request-timeout`: [The Envoy HTTP route timeout](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html#envoy-api-field-route-routeaction-timeout), specified as a [golang duration](https://golang.org/pkg/time/#ParseDuration). By default, Envoy has a 15 second timeout for a backend service to respond. Set this to `infinity` to specify that Envoy should never timeout the connection to the backend. Note that the value `0s` / zero has special semantics for Envoy.
 - `contour.heptio.com/retry-on`: [The conditions for Envoy to retry a request](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-retrypolicy-retry-on). See also [possible values and their meanings for `retry-on`](https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/router_filter.html#config-http-filters-router-x-envoy-retry-on).
 - `contour.heptio.com/num-retries`: [The maximum number of retries](https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/router_filter.html#config-http-filters-router-x-envoy-max-retries) Envoy should make before abandoning and returning an error to the client. Applies only if `contour.heptio.com/retry-on` is specified.
 - `contour.heptio.com/per-try-timeout`: [The timeout per retry attempt](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-retrypolicy-retry-on), if there should be one. Applies only if `contour.heptio.com/retry-on` is specified.
- `contour.heptio.com/tls-minimum-protocol-version` : [The minimum TLS protocol version](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/auth/cert.proto#envoy-api-msg-auth-tlsparameters) the TLS listener should support.
 - `contour.heptio.com/websocket-routes`: [The routes supporting websocket protocol](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-use-websocket), the annotation value contains a list of route paths separated by a comma that must match with the ones defined in the `Ingress` definition. Defaults to Envoy's default behavior which is `use_websocket` to `false`. The IngressRoute API has [first-class support for websockets](ingressroute.md#websocket-support).

## Contour specific Service annotations

A [Kubernetes Service](https://kubernetes.io/docs/concepts/services-networking/service/) maps to an [Envoy Cluster](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/terminology). Envoy clusters have many settings to control specific behaviors. These annotations allow access to some of those settings.

- `contour.heptio.com/max-connections`: [The maximum number of connections](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster/circuit_breaker.proto#envoy-api-field-cluster-circuitbreakers-thresholds-max-connections) that a single Envoy instance allows to the Kubernetes Service; defaults to 1024.
- `contour.heptio.com/max-pending-requests`: [The maximum number of pending requests](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster/circuit_breaker.proto#envoy-api-field-cluster-circuitbreakers-thresholds-max-pending-requests) that a single Envoy instance allows to the Kubernetes Service; defaults to 1024.
- `contour.heptio.com/max-requests`: [The maximum parallel requests](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster/circuit_breaker.proto#envoy-api-field-cluster-circuitbreakers-thresholds-max-requests) a single Envoy instance allows to the Kubernetes Service; defaults to 1024
- `contour.heptio.com/max-retries` : [The maximum number of parallel retries](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster/circuit_breaker.proto#envoy-api-field-cluster-circuitbreakers-thresholds-max-retries) a single Envoy instance allows to the Kubernetes Service; defaults to 1024. This is independent of the per-Kubernetes Ingress number of retries (`contour.heptio.com/num-retries`) and retry-on (`contour.heptio.com/retry-on`), which control whether retries are attempted and how many times a single request can retry.
- `contour.heptio.com/upstream-protocol.{protocol}` : The protocol used in the upstream. The annotation value contains a list of port names and/or numbers separated by a comma that must match with the ones defined in the `Service` definition. For now, just `h2`, `h2c`, and `tls` are supported: `contour.heptio.com/upstream-protocol.h2: "443,https"`. Defaults to Envoy's default behavior which is `http1` in the upstream.
  - The `tls` protocol allows for requests which terminate at Envoy to proxy via tls to the upstream. _Note: This does not validate the upstream certificate._

## Contour specific IngressRoute annotations

- `contour.heptio.com/ingress.class`: The Ingress class that should interpret and serve the IngressRoute. If not set, then all all Contour instances serve the IngressRoute. If specified as `contour.heptio.com/ingress.class: contour`, then Contour serves the IngressRoute. If any other value, Contour ignores the IngressRoute definition. You can override the default class `contour` with the `--ingress-class-name` flag at runtime.
