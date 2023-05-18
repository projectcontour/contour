We are delighted to present version v1.25.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Major Changes](#major-changes)
- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Docs Changes](#docs-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)

# Major Changes

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

(#5008, @ecordell)

## Add Tracing Support

Contour now supports exporting tracing data to [OpenTelemetry][1]

The Contour configuration file and ContourConfiguration CRD will be extended with a new optional `tracing` section. This configuration block, if present, will enable tracing and will define the trace properties needed to generate and export trace data.

### Contour supports the following configurations
- Custom service name, the default is `contour`.
- Custom sampling rate, the default is `100`.
- Custom the maximum length of the request path, the default is `256`.
- Customize span tags from literal and request headers.
- Customize whether to include the pod's hostname and namespace.

[1]: https://opentelemetry.io/

(#5043, @yangyy93)


# Minor Changes

## Add support for Global External Authorization for HTTPProxy.

Contour now supports external authorization for all hosts by setting the config as part of the `contourConfig` like so: 

```yaml
globalExtAuth:
  extensionService: projectcontour-auth/htpasswd
  failOpen: false
  authPolicy:
    context:
      header1: value1
      header2: value2
  responseTimeout: 1s
```

Individual hosts can also override or opt out of this global configuration. 
You can read more about this feature in detail in the [guide](https://projectcontour.io/docs/1.25/guides/external-authorization/#global-external-authorization).

(#4994, @clayton-gonsalves)

## HTTPProxy: Add support for exact path match condition

HttpProxy conditions block now also supports exact path match condition.

(#5000, @arjunsalyan)

## HTTPProxy: Internal Redirect support

Contour now supports specifying an `internalRedirectPolicy` on a `Route` to handle 3xx redirects internally, that is capturing a configurable 3xx redirect response, synthesizing a new request,
sending it to the upstream specified by the new route match,
and returning the redirected response as the response to the original request.

(#5010, @Jean-Daniel)

## HTTPProxy: implement HTTP query parameter matching

Contour now implements HTTP query parameter matching for HTTPProxy
resource-based routes. It supports `Exact`, `Prefix`, `Suffix`, `Regex` and
`Contains` string matching conditions together with the `IgnoreCase` modifier
and also the `Present` matching condition.
For example, the following HTTPProxy will route requests based on the configured
condition examples for the given query parameter `search`:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpproxy-queryparam-matching
spec:
  routes:
    - conditions:
      - queryParam:
          # will match e.g. '?search=example' as is
          name: search
          exact: example
      services:
        - name: s1
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=prefixthis' or any string value prefixed by `prefix` (case insensitive)
          name: search
          prefix: PreFix
          ignoreCase: true
      services:
        - name: s2
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=thispostfix' or any string value suffixed by `postfix` (case sensitive)
          name: search
          suffix: postfix
      services:
        - name: s3
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=regularexp123' or any string value matching the given regular expression
          name: search
          regex: ^regular.*
      services:
        - name: s4
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=somethinginsideanother' or any string value containing the substring 'inside' (case sensitive)
          name: search
          contains: inside
      services:
        - name: s5
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=' or any string value given to the named parameter
          name: search
          present: true
      services:
        - name: s6
          port: 80
```

(#5036, @relu)

## Allow Disabling Features

The `contour serve` command takes a new optional flag, `--disable-feature`, that allows disabling
certain features.

The flag is used to disable the informer for a custom resource, effectively making the corresponding
CRD optional in the cluster. You can provide the flag multiple times.

Current options include `extensionservices` and the experimental Gateway API features `tlsroutes` and
`grpcroutes`.

For example, to disable ExtensionService CRD, use the flag as follows: `--disable-feature=extensionservices`.

(#5080, @nsimons)

## Gateway API: support GRPCRoute

Contour now implements GRPCRoute (https://gateway-api.sigs.k8s.io/api-types/grpcroute/).
The "core conformance" parts of the spec are implemented.
See https://projectcontour.io/docs/1.25/guides/grpc/#gateway-api-configuration
on how to use GRPCRoute.

(#5114, @fangfpeng)


# Other Changes
- Add `AllowPrivateNetwork` to `CORSPolicy` for support Access-Control-Allow-Private-Network. (#5034, @lvyanru8200)
- Optimize processing of HTTPProxy, Secret and other objects by avoiding full object comparison. This reduces CPU usage during object updates. (#5064, @tsaarni)
- Gateway API: support the `path` field on the HTTPRoute RequestRedirect filter. (#5068, @skriss)
- Optimized the memory usage when handling Secrets in Kubernetes client informer cache. (#5099, @tsaarni)
- Adds a new gauge metric, `contour_dag_cache_object`, to indicate the total number of items that are currently in the DAG cache. (#5109, @izturn)
- Gateway API: for routes, replace use of custom `NotImplemented` condition with the upstream `Accepted: false` condition with reason `UnsupportedValue` to match the spec. (#5125, @skriss)
- `%REQ()` operator in request/response header policies now properly supports HTTP/2 pseudo-headers, header fallbacks, and truncating header values. (#5130, @sunjayBhatia)
- Gateway API: for routes, always set ResolvedRefs condition, even if true to match the spec. (#5131, @izturn)
- Set 502 response if include references another root. (#5157, @liangyuanpeng)
- Update to support Gateway API v0.6.2, which includes updated conformance tests. See release notes [here](https://github.com/kubernetes-sigs/gateway-api/releases/tag/v0.6.2). (#5194, @sunjayBhatia)
- HTTPProxy: support Host header rewrites per-service. (#5195, @fangfpeng)
- Contour now sets `GOMAXPROCS` to match the number of CPUs available to the container which results in lower and more stable CPU usage under high loads and where the container and node CPU counts differ significantly.
This is the default behavior but can be overridden by specifying `GOMAXPROCS` to a fixed value as an environment variable. (#5211, @rajatvig)
- Gateway API: Contour now always sets the Accepted condition on Gateway Listeners. If there is a specific validation error of top-level fields (port, protocol, etc.) the status is set to False, otherwise it is set to True. (#5220, @sunjayBhatia)
- Gateway API: Envoy containers manifests should use value from `envoy.health.port` and `envoy.metrics.port` if they are defined in `ContourDeployment.spec.runtimeSettings`. (#5233, @Jean-Daniel)
- Gateway API: support regex path/header match for HTTPRoute and regex header match for GRPCRoute. (#5239, @fangfpeng)
- Fix HTTPProxy duplicate include detection. If we have multiple distinct includes on the same path but different headers or query parameters, duplicates of any include conditions after the first were not detected. (#5296, @sunjayBhatia)
- Gateway API: support regular expressions in HTTPRoute query param match type. (#5310, @padlar)
- Supported/tested Kubernetes versions are now 1.25, 1.26, 1.27. (#5318, @skriss)
- Updates Envoy to v1.26.1. See the [v1.26.0 and v1.26.1 changelogs](https://www.envoyproxy.io/docs/envoy/v1.26.1/version_history/v1.26/v1.26) for details. (#5320, @skriss)
- Updates to Go 1.20.4. See the [Go release notes](https://go.dev/doc/devel/release#go1.20) for more information. (#5347, @skriss)


# Docs Changes
- Upgrade algolia docsearch to v3 on the docs website (#5129, @pnbrown)
- Updates Steve Kriss as Tech Lead and moves Nick Young to Emeritus Maintainer (#5151, @pnbrown)
- Move to a single set of docs per minor release, e.g. 1.24, 1.23 and 1.22. (#5163, @skriss)

# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.25.0 is tested against Kubernetes 1.25 through 1.27.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, we would like to give a special shoutout to the folks who joined our ContribFest at KubeCon EU 2023:

- @padlar
- @IdanAtias
- @owayss
- @shayyxi
- @bhavaniMiro
- @umutkocasarac

We'd also like to thank the other contributors to this release:

- @Jean-Daniel
- @arjunsalyan
- @clayton-gonsalves
- @ecordell
- @fangfpeng
- @izturn
- @liangyuanpeng
- @lvyanru8200
- @nsimons
- @pnbrown
- @rajatvig
- @relu
- @yangyy93

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
