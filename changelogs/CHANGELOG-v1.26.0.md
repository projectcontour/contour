We are delighted to present version v1.26.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Major Changes](#major-changes)
- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Docs Changes](#docs-changes)
- [Deprecations/Removals](#deprecation-and-removal-notices)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)

# Major Changes

## Support for Gateway Listeners on more than two ports

Contour now supports Gateway Listeners with many different ports.
Previously, Contour only allowed a single port for HTTP, and a single port for HTTPS/TLS.

As an example, the following Gateway, with two HTTP ports and two HTTPS ports, is now fully supported:

```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
spec:
  gatewayClassName: contour
  listeners:
    - name: http-1
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: Same
    - name: http-2
      protocol: HTTP
      port: 81
      allowedRoutes:
        namespaces:
          from: Same
    - name: https-1
      protocol: HTTPS
      port: 443
      allowedRoutes:
        namespaces:
          from: Same
      tls:
        mode: Terminate
        certificateRefs:
        - name: tls-cert-1
    - name: https-2
      protocol: HTTPS
      port: 444
      allowedRoutes:
        namespaces:
          from: Same
      tls:
        mode: Terminate
        certificateRefs:
        - name: tls-cert-2
```

If you are using the Contour Gateway Provisioner, ports for all valid Listeners will automatically be exposed via the Envoy service, and will update when any Listener changes are made.
If you are using static provisioning, you must keep the Service definition in sync with the Gateway spec manually.

Note that if you are using the Contour Gateway Provisioner along with HTTPProxy or Ingress for routing, then your Gateway must have exactly one HTTP Listener and one HTTPS Listener.
For this case, Contour supports a custom HTTPS Listener protocol value, to avoid having to specify TLS details in the Listener (since they're specified in the HTTPProxy or Ingress instead):

```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour-with-httpproxy
spec:
  gatewayClassName: contour
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
    - name: https
      protocol: projectcontour.io/https
      port: 443
      allowedRoutes:
        namespaces:
          from: All
```

(#5160, @skriss)


# Minor Changes

## Contour now outputs metrics about status update load

Metrics on status update counts and duration are now output by the xDS server.
This should enable deployments at scale to diagnose delays in status updates and possibly tune the `--kubernetes-client-qps` and `--kubernetes-client-burst` flags.

(#5037, @sunjayBhatia)

## Watching specific namespaces

The `contour serve` command takes a new optional flag, `--watch-namespaces`, that can
be used to restrict the namespaces where the Contour instance watches for resources.
Consequently, resources in other namespaces will not be known to Contour and will not
be acted upon.

You can watch a single or multiple namespaces, and you can further restrict the root
namespaces with `--root-namespaces` just like before. Root namespaces must be a subset
of the namespaces being watched, for example:

`--watch-namespaces=my-admin-namespace,my-app-namespace --root-namespaces=my-admin-namespace`

If the `--watch-namespaces` flag is not used, then all namespaces will be watched by default.

(#5214, @nsimons)

## HTTPProxy: Implement Regex Path Matching and Regex Header Matching.

This Change Adds 2 features to HTTPProxy
1. Regex based path matching.
1. Regex based header matching.


### Path Matching

In addition to `prefix` and `exact`, HTTPProxy now also support `regex`.


#### Root Proxies
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root-regex-match
spec:
  fqdn: local.projectcontour.io
  routes:
    - conditions:
      # matches
      # - /list/1234
      # - /list/
      # - /list/foobar
      # and so on and so forth
      - regex: /list/.*
      services:
        - name: s1
          port: 80
    - conditions:
      # matches
      # - /admin/dev
      # - /admin/prod
      - regex: /admin/(prod|dev)
      services:
        - name: s2
          port: 80
```

#### Inclusion

##### Root

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root-regex-match
spec:
  fqdn: local.projectcontour.io
  includes:
  - name: child-regex-match
    conditions:
    - prefix: /child
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
```

##### Included

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: child-regex-match
spec:
  fqdn: local.projectcontour.io
  routes:
    - conditions:
      # matches
      # - /child/list/1234
      # - /child/list/
      # - /child/list/foobar
      # and so on and so forth
      - regex: /list/.*
      services:
        - name: s1
          port: 80
    - conditions:
      # matches
      # - /child/admin/dev
      # - /child/admin/prod
      - regex: /admin/(prod|dev)
      services:
        - name: s2
          port: 80
    - conditions:
      # matches
      # - /child/bar/stats
      # - /child/foo/stats
      # and so on and so forth
      - regex: /.*/stats
      services:
        - name: s3
          port: 80
```

### Header Regex Matching

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpproxy-header-matching
spec:
  fqdn: local.projectcontour.io
  routes:
    - conditions:
      - queryParam:
          # matches header `x-header` with value of `dev-value` or `prod-value`
           name: x-header
          regex: (dev|prod)-value
      services:
        - name: s4
          port: 80
```

(#5319, @clayton-gonsalves)

## Adds critical level for access logging

New critical access log level was introduced to reduce the volume of logs for busy installations. Critical level produces access logs for response status >= 500.

(#5360, @davinci26)

## Default Global RateLimit Policy

This Change adds the ability to define a default global rate limit policy in the Contour configuration 
to be used as a global rate limit policy by all HTTPProxy objects.
HTTPProxy object can decide to opt out and disable this feature using `disabled` config.

### Sample Configurations
#### contour.yaml
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    rateLimitService:
      extensionService: projectcontour/ratelimit
      domain: contour
      failOpen: false
      defaultGlobalRateLimitPolicy:
        descriptors:
          - entries:
              - remoteAddress: {}
          - entries:
              - genericKey:
                  value: foo
```

(#5363, @shadialtarsha)

## Contour supports setting the MaxRequestsPerConnection

Support setting of MaxRequestsPerConnection on listeners or clusters via the contour configuration.

(#5417, @clayton-gonsalves)

## Failures to automatically set GOMAXPROCS are no longer fatal

In some (particularly local development) environments the [automaxprocs](https://github.com/uber-go/automaxprocs) library fails due to the cgroup namespace setup.
This failure is no longer fatal for Contour.
Contour will now simply log the error and continue with the automatic GOMAXPROCS detection ignored.

(#5427, @sunjayBhatia)

## Routes with HTTP Method matching have higher precedence

For conformance with Gateway API v0.7.1+, routes that utilize HTTP method matching now have an explicit precedence over routes with header/query matches.
See the [Gateway API documentation](https://github.com/kubernetes-sigs/gateway-api/blob/v0.7.1/apis/v1beta1/httproute_types.go#L163-L167) for a description of this precedence order.

This change applies not only to HTTPRoute but also HTTPProxy method matches (implemented in configuration via a header match on the `:method` header).

(#5434, @sunjayBhatia)

## Host header including port is passed through unmodified to backend

Previously Contour would strip any port from the Host header in a downstream request for convenience in routing.
This resulted in backends not receiving the Host header with a port.
We no longer do this, for conformance with Gateway API (this change also applies to HTTPProxy and Ingress configuration).

(#5437, @sunjayBhatia)

## Gateway API: add TCPRoute support

Contour now supports Gateway API's [TCPRoute](https://gateway-api.sigs.k8s.io/guides/tcp/) resource.
This route type provides simple TCP forwarding for traffic received on a given Listener port.

This is a simple example of a Gateway and TCPRoute configuration:

```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
  namespace: projectcontour
spec:
  gatewayClassName: contour
  listeners:
    - name: tcp-listener
      protocol: TCP
      port: 10000
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: echo-1
  namespace: default
spec:
  parentRefs:
  - namespace: projectcontour
    name: contour
    sectionName: tcp-listener
  rules:
  - backendRefs:
    - name: s1
      port: 80
```

(#5471, @skriss)

## Gateway API: Support TLS termination with TLSRoute and TCPRoute

Contour now supports using TLSRoute and TCPRoute in combination with TLS termination.
To use this feature, create a Gateway with a Listener like the following:

```yaml
- name: tls-listener
  protocol: TLS
  port: 5000
  tls:
    mode: Terminate
    certificateRefs:
    - name: tls-cert-secret
  allowedRoutes:
    namespaces:
      from: All
---
```

It is then possible to attach either 1+ TLSRoutes, or a single TCPRoute, to this Listener.
If using TLSRoute, traffic can be routed to a different backend based on SNI.
If using TCPRoute, all traffic is forwarded to the backend referenced in the route.

(#5481, @skriss)

## Allow setting of `per_connection_buffer_limit_bytes` value for Clusters

Allow changing `per_connection_buffer_limit_bytes` for all Clusters. Default is not set to keep compatibility with existing configurations. Envoy [recommends](https://www.envoyproxy.io/docs/envoy/latest/configuration/best_practices/edge) setting to 32KiB for Edge proxies.

(#5493, @rajatvig)

## Allow setting of `per_connection_buffer_limit_bytes` value for Listeners

Allow changing `per_connection_buffer_limit_bytes` for Listeners. Default is not set to keep compatibility with existing configurations.
Envoy [recommends](https://www.envoyproxy.io/docs/envoy/latest/configuration/best_practices/edge) setting to 32KiB for Edge proxies.

(#5513, @rajatvig)

## Fix order of Global ExtAuth and Global Ratelimit

This ensures that the order of execution of extauth and global ratelimit is the same across HTTP and HTTPS virtualhosts, which is Auth goes first then Global ratelimit. (#5559, @clayton-gonsalves)

## Adds support for case-insensitive header matching

Adds support for `IgnoreCase` in route header matching condition. This brings parity to matching capabilities of query param.

(#5567, @davinci26)

## Adds support for treating missing headers as empty when they are not present as part of header matching

`TreatMissingAsEmpty` specifies if the header match rule specified header does not exist, this header value will be treated as empty. Defaults to false.
Unlike the underlying Envoy implementation this is **only** supported for negative matches (e.g. NotContains, NotExact).

(#5584, @davinci26)

## Adding support for multiple gateway-api RequestMirror filters within the same HTTP or GRPC rule 

Currently, Contour supports a single RequestMirror filter per rule in HTTPRoute or GRPCRoute.
Envoy however, supports more than one mirror backend using [request_mirror_policies](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/route/v3/route_components.proto#config-route-v3-routeaction)

This PR adds support for multiple gateway-api RequestMirror filters within the same HTTP or GRPC rule.

(#5652, @LiorLieberman)

## Support Gateway API v0.8.0

Contour now supports Gateway API v0.8.0, keeping up to date with conformance and API changes.
This release mainly contains refinements to status conditions, conformance test additions, and the addition of CEL validation for Gateway API CRDs.
The previous version of Contour supported Gateway API v0.6.2 and there have been multiple releases in the interim.
See [v0.7.0 release notes](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.7.0), [v0.7.1 release notes ](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.7.1), and [v0.8.0 release notes](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.8.0) for more detail on the content of these releases.

(#5726, @sunjayBhatia)


# Other Changes
- Additional metrics generated by controller-runtime are now included in Prometheus metrics output by Contour. These include metrics around API Server requests, leader election, and individual resource controllers. (#5006, @sunjayBhatia)
- Gateway provisioner: add a container port to the Envoy daemonset/deployment for the metrics port. (#5101, @izturn)
- Add Kubernetes annotations configurability to ContourDeployment resource. to enable customize pod annotations for pod/contour (#5346, @izturn)
- Add configuration for socket options to support DSCP marking for outbound IP packets, for both IPv4 (TOS field) and IPv6 (Traffic Class field). (#5352, @tsaarni)
- Gateway API: Add `ipFamilyPolicy` field to `ContourDeployment.spec.envoy.networkPublishing` to control dual-stack-ness of the generated service. (#5386, @Jean-Daniel)
- Gateway provisioner: set the default resource requirements for containers: shutdown-manager & envoy-initconfig (#5425, @izturn)
- DAG rebuild fixes: Update of CRL Secret did not trigger reload when it was not co-located in the same Secret with CA certificate, update of TLS Secret did not trigger reload when using `Ingress.spec.tls.secretName` with certificate delegation and `projectcontour.io/tls-cert-namespace` annotation. (#5463, @tsaarni)
- Certgen job name in example deployment manifest is now generated using pattern `contour-certgen-v1-26-0` instead of `contour-certgen-v1.26.0` to follow Kubernetes pod naming rules. (#5484, @tsaarni)
- Service change now triggers update for GRPCRoute. (#5485, @yanggangtony)
- HTTPProxy: support fractional mirroring to a service by specifying `mirror: true` and a `weight` of 1-100. See the [traffic mirroring docs](https://projectcontour.io/docs/1.26/config/request-routing/#traffic-mirroring) for more information. (#5516, @hshamji)
- Gateway API: enable websockets upgrade by default. (#5524, @skriss)
- HTTPProxy: HTTP health checks can now be configured to treat response codes other than 200 as healthy. See [HTTPProxy Health Checking](https://projectcontour.io/docs/1.26/config/health-checks/#http-proxy-health-checking) and [HTTPHealthCheckPolicy reference docs](https://projectcontour.io/docs/1.26/config/api/#projectcontour.io/v1.HTTPHealthCheckPolicy) for more information. (#5528, @skriss)
- Refactor the handling of cross-namespace references and improve documentation. (#5529, @tsaarni)
- The maximum TLS version on Envoy Listeners can be configured. Valid options are TLS versions `1.2` and `1.3` with a default of `1.3`. (#5533, @izturn)
- Contour can now include the kind, namespace and name of the relevant HTTPProxy/Ingress/HTTPRoute in Envoy's access logs. See the [access logging docs](https://projectcontour.io/docs/1.26/config/access-logging/#logging-the-route-source) for more information. (#5534, @skriss)
- Updates Envoy to v1.27.0. See the [release notes here](https://www.envoyproxy.io/docs/envoy/v1.27.0/version_history/v1.27/v1.27.0). (#5594, @skriss)
- Gateway provisioner: Expose all Envoy service IPs and Hostname in `Gateway.Status.Addresses`. Only the first IP was exposed before, even for dual-stack service. (#5651, @Jean-Daniel)
- Updates to Go 1.20.7. See the [Go release notes](https://go.dev/doc/devel/release#go1.20) for more information. (#5663, @sunjayBhatia)
- Add included HTTPProxy namespace and name to status condition error message when a root HTTPProxy includes another root. (#5670, @HeavenTonight)
- Gateway provisioner: add a base-id to the Envoy daemonset/deployment to solve the (potential) shared memory regions conflict (#5677, @izturn)
- Updates to Kubernetes 1.28. Supported/tested Kubernetes versions are now 1.26, 1.27 and 1.28. (#5700, @skriss)
- Updates external auth examples and manifests to use the new contour-authserver registry and version, `ghcr.io/projectcontour/contour-authserver:v4`. (#5725, @skriss)


# Docs Changes
- Update FIPS build documentation for changes to Envoy 1.26. Updates guide versions for 1.25 and main. (#5415, @sunjayBhatia)
- Collapses the documentation for 1.21 and 1.20 into a single set per minor version, and removes older docs from the website dropdown. (#5432, @skriss)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.26.0 is tested against Kubernetes 1.26 through 1.28.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @HeavenTonight
- @Jean-Daniel
- @LiorLieberman
- @clayton-gonsalves
- @davinci26
- @hshamji
- @izturn
- @nsimons
- @rajatvig
- @shadialtarsha
- @yanggangtony


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
