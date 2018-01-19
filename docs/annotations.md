# Annotations

Contour supports a couple of standard kubernetes ingress annotations, as well as some contour-specific ones. Below is a listing of each supported annotation and a brief description


## Standard Ingress Annotations

 - `kubernetes.io/ingress.class`: The ingress class which should interpret and serve the ingress. If this isn't set, then all ingress controllers will serve the ingress. If specified as `kubernetes.io/ingress.class: contour` then contour will serve the ingress. If it has any other value, contour will ignore the ingress definition.
 - `ingress.kubernetes.io/force-ssl-redirect`: Marks the ingress to envoy as requiring TLS/SSL by setting the [envoy virtual host option require_tls](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html#envoy-api-field-route-virtualhost-require-tls)
 - `kubernetes.io/allow-http`: Instructs contour to not create an envoy http route for the virtual host at all. The ingress will only exist for HTTPS requests. This should be given the value `"true"` to cause envoy to mark the endpoint as http only. All other values are ignored.


## Contour Specific Ingress Annotations

 - `contour.heptio.com/request-timeout`: Set the [envoy HTTP route timeout](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto.html#envoy-api-field-route-routeaction-timeout) to the given value, specified as a [golang duration](https://golang.org/pkg/time/#ParseDuration). By default envoy has a 15 second timeout for a backend service to respond. Set this to `infinity` to specify envoy should never timeout the connection to the backend. Note the value `0s` / zero has special semantics to envoy.
