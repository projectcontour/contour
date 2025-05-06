# IP Filtering

Contour supports filtering requests based on the incoming ip address using Envoy's [RBAC Filter][1].

Requests can be either allowed or denied based on a CIDR range specified on the virtual host and/or individual routes.

If the request's IP address is allowed, the request will be proxied to the appropriate upstream.
If the request's IP address is denied, an HTTP 403 (Forbidden) will be returned to the client.

## Specifying Rules

Rules are specified with the `ipAllowPolicy` and `ipDenyPolicy` fields on `virtualhost` and `route`:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: basic
spec:
  virtualhost:
    fqdn: foo-basic.bar.com
    ipAllowPolicy:
      # traffic is allowed if it came from localhost (i.e. co-located reverse proxy)
      - cidr: 127.0.0.1/32
        source: Peer
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
      # route-level ip filters override the virtualhost-level filters
      ipAllowPolicy:
      # traffic is allowed if it came from localhost (i.e. co-located reverse proxy)
      - cidr: 127.0.0.1/32
        source: Peer
      # and the request originated from an IP in this range 
      - cidr: 99.99.0.0/16
        source: Remote
```

### Specifying CIDR Ranges

CIDR ranges may be ipv4 or ipv6. Bare IP addresses are interpreted as the CIDR range containing that one ip address only.

Examples:
- `1.1.1.1/24` 
- `127.0.0.1`
- `2001:db8::68/24`
- `2001:db8::68`

### Allow vs Deny

Filters are specified as either allow or deny:

- `ipAllowPolicy` only allows requests that match the ip filters.
- `ipDenyPolicy` denies all requests unless they match the ip filters.

Allow and deny policies cannot both be specified at the same time for a virtual host or route.

### IP Source

The `source` field controls how the ip address is selected from the request for filtering.

- `source: Peer` filter rules will filter using Envoy's [direct_remote_ip][2], which is always the physical peer.
- `source: Remote` filter rules will filter using Envoy's [remote_ip][3], which may be inferred from the X-Forwarded-For header or proxy protocol.

If using `source: Remote` with `X-Forwarded-For`, it may be necessary to configure Contour's `numTrustedHops` in [Network Parameters][4].

### Virtual Host and Route Filter Precedence

IP filters on the virtual host apply to all routes included in the virtual host, unless the route specifies its own rules.

Rules specified on a route override any rules defined on the virtual host, they are not additive.

[1]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rbac_filter.html
[2]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/rbac/v3/rbac.proto#envoy-v3-api-field-config-rbac-v3-principal-direct-remote-ip
[3]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/rbac/v3/rbac.proto#envoy-v3-api-field-config-rbac-v3-principal-remote-ip
[4]: api/#projectcontour.io/v1.NetworkParameters

