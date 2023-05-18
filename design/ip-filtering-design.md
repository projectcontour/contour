# IP Filtering 

Status: Approved 

## Abstract

Allow or deny traffic to services behind contour with an IP allowlist or an IP denylist.

## Background

There have been [long standing requests](https://github.com/projectcontour/contour/issues/62) to support some form of IP filtering.
IP filtering is a useful tool for network segregation that is supported by other ingress controllers like [ingress-nginx](https://github.com/kubernetes/ingress-nginx/blob/87d1b8bbf28265386b339e65d8943d8c3f8582ed/docs/user-guide/nginx-configuration/annotations.md#whitelist-source-range) and [ambassador](https://www.getambassador.io/docs/edge-stack/latest/topics/running/ambassador#ip-allow-and-deny), and is a [common](https://github.com/projectcontour/contour/issues/62#issuecomment-698141472) [request](https://github.com/projectcontour/contour/issues/62#issuecomment-843193956) among the community, often listed as a [blocker](https://github.com/projectcontour/contour/issues/3693#issuecomment-1317049708) for adopting contour.

## Goals

- Support ipv4 and ipv6 allowlists and denylists via Contour APIs.
- Support filtering on request IP or on forwarded IP (i.e. via `X-Forwarded-For`)

## Non Goals

- Define Gateway API support for ip filtering. This is an obvious next step, but requires broader agreement. 
- Define a global IP filter that works across multiple Routes or HTTPProxy objects.
- Supporting envoy's network rbac filter (this design is for the http filter only).

## High-Level Design

The `virtualhost` and `route` sections of an `HTTPProxy` will each have two new fields added: `ipAllowPolicy` and `ipDenyPolicy`.

Example `HTTPProxy`:

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
      # route-level ip filters overrides the virtualhost-level filters
      ipAllowPolicy:
      # traffic is allowed if it came from localhost (i.e. co-located reverse proxy)
      - cidr: 127.0.0.1/32
        source: Peer
      # and the request originated from an IP in this range 
      - cidr: 99.99.0.0/16
        source: Remote
```

## Detailed Design


### API

`ipAllowPolicy`/`ipDenyPolicy` has the following structure:

```yaml
ipAllowPolicy:
  - cidr: 127.0.0.1/32
    source: Peer
  - cidr: 99.99.0.0/16
    source: Remote
```

```yaml
ipDenyPolicy:
  - cidr: 127.0.0.1/32
    source: Peer
  - cidr: 99.99.0.0/16
    source: Remote
```

Note that:
- Allow rules deny all requests that don't match a rule.
- Deny rules allow all requests that don't match a rule.
- Rules defined on the virtualhost can be overridden by rules defined on a route.
- Allow rules cannot be mixed with Deny rules: it won't be evident if a deny rule should be an exception to an allow rule, or if an allow rule should be an exception to a deny rule.
- Multiple allow/deny CIDR ranges can be specified by repeating a key.
- `Peer` refers to the request IP
- `Remote` refers to the originating IP, i.e. `X-Forwarded-For`. `numTrustedHops` may also need to be set to filter on the desired IP.

### Processing

`HTTPProxyProcessor` and `dag.Route` will store the ip filtering rules in a new field.

These settings will be converted convert to a per-route or per-virtualhost envoy filter, i.e.:

```yaml
routes:
  - match:
      prefix: /
    route:
      cluster: example-cluster 
    typed_per_filter_config:
      envoy.filters.http.rbac:
        '@type': type.googleapis.com/envoy.extensions.filters.http.rbac.v3.RBACPerRoute
        rbac:
          rules:
            # action is ALLOW if ipAllowPolicy is used, otherwise DENY is used
            action: ALLOW
            policies:
              ip-rules:
                permissions:
                  - any: true
                principals:
                  # direct_remote_ip is used if `source: Peer` is specified
                  - direct_remote_ip:
                      address_prefix: 127.0.0.1
                      prefix_len: 32
                  # remote_ip is used if `source: Remote` is specified
                  - remote_ip:
                      address_prefix: 99.99.0.0
                      prefix_len: 16
```

Please see [the Envoy docs](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/rbac/v3/rbac.proto) for more detailed explanations of the filter behavior.

## Alternatives

The ip filtering could be a top-level field on the `HTTPProxy`.

This would allow building "trees" of ip filters via `includes`:
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
  - peer: 127.0.0.1
  # and the request originated from an IP in this range 
  - remote: 99.99.0.0/16
  includes:
  - name: service2
    conditions:
      - prefix: /service2
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: service2
spec:
  ipDenyPolicy:
    # if the request made it through the first filter, it could still be denied here
    - remote: 99.99.0.1
  routes:
    - conditions:
        - prefix: /service2
      services:
        - name: s2
          port: 80
```

But no other request filtering options work this way today, so this approach seems like a mismatch with current Contour.
