# Annotations

Contour supports a couple of standard kubernetes ingress annotations, as well as some contour-specific ones. Below is a listing of each supported annotation and a brief description


## Standard Ingress Annotations

 - `kubernetes.io/ingress.class`: The ingress class which should interpret and serve the ingress. If this isn't set, then all ingress controllers will serve the ingress. If specified as `kubernetes.io/ingress.class: contour` then contour will serve the ingress. If it has any other value, contour will ignore the ingress definition.
 - `ingress.kubernetes.io/force-ssl-redirect`: Marks the ingress to envoy as requiring TLS/SSL by setting the [envoy virtual host option require_tls](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html#envoy-api-field-route-virtualhost-require-tls)
 - `kubernetes.io/allow-http`: Instructs contour to not create an envoy http route for the virtual host at all. The ingress will only exist for HTTPS requests. This should be given the value `"true"` to cause envoy to mark the endpoint as http only. All other values are ignored.


## Contour Specific Ingress Annotations

 - `contour.heptio.com/request-timeout`: Set the [envoy HTTP route timeout](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html#envoy-api-field-route-routeaction-timeout) to the given value, specified as a [golang duration](https://golang.org/pkg/time/#ParseDuration). By default envoy has a 15 second timeout for a backend service to respond. Set this to `infinity` to specify envoy should never timeout the connection to the backend. Note the value `0s` / zero has special semantics to envoy.
 - `contour.heptio.com/retry-on`: Specify under which conditions Envoy should retry a request. See [Envoy retry_on](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-retrypolicy-retry-on) for basic description, as well as [possible values and their meanings](https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/router_filter.html#config-http-filters-router-x-envoy-retry-on)
 - `contour.heptio.com/num-retries`: Specify the [maximum number of retries](https://www.envoyproxy.io/docs/envoy/latest/configuration/http_filters/router_filter.html#config-http-filters-router-x-envoy-max-retries) Envoy should make before abandoning and returning an error to the client. Only applies if `contour.heptio.com/retry-on` is specified.
 - `contour.heptio.com/per-try-timeout`: Specify the [timeout per retry attempt](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-retrypolicy-retry-on), if there should be one. Only applies if `contour.heptio.com/retry-on` is specified.

## Contour Specific Service Annotations

A [Kubernetes Service](https://kubernetes.io/docs/concepts/services-networking/service/) maps to an [Envoy Cluster](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/terminology). Envoy clusters have many settings to control specific behaviors. These annotations allow access to some of those settings.

- `contour.heptio.com/max-connections`: [The maximum number of connections](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster/circuit_breaker.proto#envoy-api-field-cluster-circuitbreakers-thresholds-max-connections) that a single Envoy instance will allow to the Kubernetes service; defaults to 1024.
- `contour.heptio.com/max-pending-requests`: [The maximum number of pending requests](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster/circuit_breaker.proto#envoy-api-field-cluster-circuitbreakers-thresholds-max-pending-requests) that a single Envoy instance will allow to the Kubernetes service; defaults to 1024.
- `contour.heptio.com/max-requests`: [The maximum parallel requests](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster/circuit_breaker.proto#envoy-api-field-cluster-circuitbreakers-thresholds-max-requests) a single Envoy instance will allow to the Kubernetes service; defaults to 1024
- `contour.heptio.com/max-retries` : [The maximum number of parallel retries](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster/circuit_breaker.proto#envoy-api-field-cluster-circuitbreakers-thresholds-max-retries) a single Envoy instance will allow to the kubernetes service; defaults to 1024. This is independent of the per-kubernetes ingress number of retries (`contour.heptio.com/num-retries`) and retry-on (`contour.heptio.com/retry-on`), which control whether or not retries are attempted, as well as how many times a single request can retry at most.