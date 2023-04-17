## IP Filter Support 

Contour's HTTPProxy now supports configuring Envoy's [RBAC filter](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/rbac/v3/rbac.proto) for allowing or denying requests by IP.

An HTTPProxy can optionally include one or more IP filter rules, which define CIDR ranges to allow or deny requests based on origin IP.
Filters can indicate whether the direct IP should be used or whether a reported IP from `PROXY` or `X-Forwarded-For` should be used instead.
If the latter, Contour's `numTrustedHops` setting will be respected when determining the source IP.
Filters defined at the VirtualHost level apply to all routes, unless overridden by a route-specific filter.

For more information, see:
- [HTTPProxy API documentation](https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1.HTTPProxy)
- [IPFilterPolicy API documentation](https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1.IPFilterPolicy)
- [Envoy RBAC filter documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/rbac/v3/rbac.proto)
