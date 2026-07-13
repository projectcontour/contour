# GeoIP Filtering

Contour supports filtering requests based on the geolocation of the client IP
address, using Envoy's [GeoIP HTTP filter][geoip] to enrich requests and Envoy's
[RBAC filter][rbac] to allow or deny them based on the populated geolocation
headers. This mirrors the [Envoy Gateway GeoIP authorization][eg-geoip] task.

GeoIP filtering has two parts:

1. **Enrichment** — configured globally via
   [`ContourConfiguration.spec.geoIP`](#configuring-the-geoip-filter). Envoy
   looks up the client IP in a [MaxMind][maxmind] GeoIP2 `.mmdb` database and
   populates request headers with geolocation data (country, region, city,
   ASN/ISP and anonymous-IP flags). Contour controls the names of these
   headers. The filter is added only to TLS virtual hosts that use geo rules.
2. **Allow/deny rules** — configured per virtual host and/or route on
   `HTTPProxy` via the `geoAllowPolicy` and `geoDenyPolicy` fields. These rules
   match the headers populated by the GeoIP filter and allow or deny the
   request. Geo rules require TLS (see below).

If a request matches an allow policy it is proxied upstream; if it matches a
deny policy (or fails an allow policy) an HTTP 403 (Forbidden) is returned.

## Configuring the GeoIP filter

The GeoIP filter is enabled globally on the `ContourConfiguration` resource:

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ContourConfiguration
metadata:
  name: contour
spec:
  geoIP:
    provider:
      maxMind:
        cityDbPath: /etc/geoip/GeoLite2-City.mmdb
        asnDbPath: /etc/geoip/GeoLite2-ASN.mmdb
        anonDbPath: /etc/geoip/GeoLite2-Anonymous-IP.mmdb
```

Each `*DbPath` field points to an `.mmdb` file that must be **mounted into the
Envoy pod separately** — Contour only references the in-pod path, it does not
manage the volume. See [Mounting the MaxMind database](#mounting-the-maxmind-database).

The client IP is taken from the `X-Forwarded-For` header using Contour's
[`numTrustedHops`](../config/api/#projectcontour.io/v1.NetworkParameters); set
that to the number of trusted upstream proxies in front of Envoy so the correct
client IP is used.

### Database paths and populated headers

Contour populates a geolocation header only when a database capable of
populating it is configured. The header names are fixed by Contour:

| Header | Populated by | Value |
| --- | --- | --- |
| `x-contour-geo-country` | city or country database | ISO 3166-1 alpha-2 country code (e.g. `US`) |
| `x-contour-geo-region` | city database | ISO 3166-2 subdivision code (e.g. `US-CA`) |
| `x-contour-geo-city` | city database | city name |
| `x-contour-geo-asn` | ASN or ISP database | autonomous system number (e.g. `7922`) |
| `x-contour-geo-isp` | ISP database | ISP name |
| `x-contour-geo-anon` | anonymous-IP database | `true` or `false` |
| `x-contour-geo-anon-vpn` | anonymous-IP database | `true` or `false` |
| `x-contour-geo-anon-tor` | anonymous-IP database | `true` or `false` |
| `x-contour-geo-anon-hosting` | anonymous-IP database | `true` or `false` |
| `x-contour-geo-anon-proxy` | anonymous-IP database | `true` or `false` |

These headers are added to the request and are visible to upstream services as
well as to GeoIP allow/deny rules.

## Specifying GeoIP allow/deny rules

Rules are specified with the `geoAllowPolicy` and `geoDenyPolicy` fields on
`virtualhost` and `route`. Each rule matches a single geolocation dimension:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: geo-example
spec:
  virtualhost:
    fqdn: geo.example.com
    tls:
      secretName: geo-tls
    # Deny requests from anonymous IPs and VPNs.
    geoDenyPolicy:
      - dimension: anon
        value: "true"
      - dimension: anonVpn
        value: "true"
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
```

GeoIP filtering requires TLS to be configured on the virtual host (like
authorization), since the GeoIP filter is configured per TLS virtual host.

A rule has two fields:

- `dimension`: the geolocation attribute to match. One of `country`, `region`,
  `city`, `asn`, `isp`, `anon`, `anonVpn`, `anonTor`, `anonHosting`, `anonProxy`.
- `value`: the value to match exactly against the populated header. For
  `country` this is an ISO 3166-1 alpha-2 code (e.g. `US`); for the `anon*`
  dimensions it is `true` or `false`.

Rules within a policy are combined with OR semantics: a request matches the
policy if it matches any rule. The GeoIP filter must be configured with a
database that populates the matched dimension, otherwise the header will be
absent and the rule will never match.

### Allow vs Deny

- `geoAllowPolicy` only allows requests that match the geo rules.
- `geoDenyPolicy` denies requests that match the geo rules.

Allow and deny policies cannot both be specified at the same time for a virtual
host or route. When both IP (`ipAllowPolicy`/`ipDenyPolicy`) and geo
(`geoAllowPolicy`/`geoDenyPolicy`) rules are present on the same scope, they
must both be allow or both be deny; the rules are then combined so a request
matches if it matches any IP or geo rule.

### Virtual host and route precedence

GeoIP rules on the virtual host apply to all routes in the virtual host, unless
a route specifies its own rules. Rules specified on a route override any rules
defined on the virtual host, they are not additive.

## Mounting the MaxMind database

Contour's xDS configuration only references the in-pod `.mmdb` path; it does
not mount the file. The database must be supplied out of band.

### With the Gateway Provisioner

Mount the database via `ContourDeployment.spec.envoy.extraVolumes` and
`extraVolumeMounts`, e.g. from a ConfigMap or Secret containing the `.mmdb`:

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ContourDeployment
metadata:
  name: contour
spec:
  envoy:
    extraVolumes:
      - name: geoip
        configMap:
          name: geoip-db
    extraVolumeMounts:
      - name: geoip
        mountPath: /etc/geoip
        readOnly: true
```

### With the static example manifests

Edit `examples/contour/03-envoy.yaml` (or the `examples/deployment/` variant)
to add a `volume` for the `.mmdb` and a matching `volumeMount` on the Envoy
container, then point the `ContourConfiguration.spec.geoIP` paths at the mounted
location.

[geoip]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/geoip_filter
[rbac]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/rbac_filter.html
[eg-geoip]: https://gateway.envoyproxy.io/docs/tasks/security/geoip-authorization/
[maxmind]: https://dev.maxmind.com/geoip/geolite2-free-geolocation-data
