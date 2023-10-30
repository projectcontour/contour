We are delighted to present version v1.27.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.


- [Major Changes](#major-changes)
- [Minor Changes](#minor-changes)
- [Other Changes](#other-changes)
- [Docs Changes](#docs-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)
- [Community Thanks!](#community-thanks)

# Major Changes

## Fix bug with algorithm used to sort Envoy regex/prefix path rules

Envoy greedy matches routes and as a result the order route matches are presented to Envoy is important. Contour attempts to produce consistent routing tables so that the most specific route matches are given preference. This is done to facilitate consistency when using HTTPProxy inclusion and provide a uniform user experience for route matching to be inline with Ingress and Gateway API Conformance.

This changes fixes the sorting algorithm used for `Prefix` and `Regex` based path matching. Previously the algorithm lexicographically sorted based on the path match string instead of sorting them based on the length of the `Prefix`|`Regex`. i.e. Longer prefix/regexes will be sorted first in order to give preference to more specific routes, then lexicographic sorting for things of the same length.

Note that for prefix matching, this change is _not_ expected to change the relative ordering of more specific prefixes vs. less specific ones when the more specific prefix match string has the less specific one as a prefix, e.g. `/foo/bar` will continue to sort before `/foo`. However, relative ordering of other combinations of prefix matches may change per the above description.

### How to update safely

Caution is advised if you update Contour and you are operating large routing tables. We advise you to:

1. Deploy a duplicate Contour installation that parses the same CRDs
2. Port-forward to the Envoy admin interface [docs](https://projectcontour.io/docs/v1.3.0/troubleshooting/)
3. Access `http://127.0.0.1:9001/config_dump` and compare the configuration of Envoy. In particular the routes and their order. The prefix routes might be changing in order, so if they are you need to verify that the route matches as expected.

(#5752, @davinci26)


# Minor Changes

## Specific routes can now opt out of the virtual host's global rate limit policy

Setting `rateLimitPolicy.global.disabled` flag to true on a specific route now disables the global rate limit policy inherited from the virtual host for that route.

### Sample Configurations
In the example below, `/foo` route is opted out from the global rate limit policy defined by the virtualhost.
#### httpproxy.yaml
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    rateLimitPolicy:
      global:
        descriptors:
          - entries:
            - remoteAddress: {}
            - genericKey:
                key: vhost
                value: local.projectcontour.io
  routes:
    - conditions:
        - prefix: /
      services:
        - name: ingress-conformance-echo
          port: 80
    - conditions:
        - prefix: /foo
      rateLimitPolicy:
        global:
          disabled: true
      services:
        - name: ingress-conformance-echo
          port: 80
```

(#5657, @shadialtarsha)

## Contour now waits for the cache sync before starting the DAG rebuild and XDS server

Before this, we only waited for informer caches to sync but didn't wait for delivering the events to subscribed handlers.
Now contour waits for the initial list of Kubernetes objects to be cached and processed by handlers (using the returned `HasSynced` methods)
and then starts building its DAG and serving XDS.

(#5672, @therealak12)

## HTTPProxy: Allow Host header rewrite with dynamic headers.

This Change allows the host header to be rewritten on requests using dynamic headers on the only route level.

#### Example
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: dynamic-host-header-rewrite
spec:
  fqdn: local.projectcontour.io
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
    - requestHeaderPolicy:
        set:
        - name: host
          value: "%REQ(x-rewrite-header)%"
```

(#5678, @clayton-gonsalves)

## Add Kubernetes Endpoint Slice support

This change optionally enables Contour to consume the kubernetes endpointslice API to determine the endpoints to configure Envoy with.
Note: This change is off by default and is gated by the feature flag `useEndpointSlices`.

This feature will be enabled by default in a future version on Contour once it has had sufficient bake time in production environments.

(#5745, @clayton-gonsalves)

## Max HTTP requests per IO cycle is configurable as an additional mitigation for HTTP/2 CVE-2023-44487

Envoy v1.27.1 mitigates CVE-2023-44487 with some default runtime settings, however the `http.max_requests_per_io_cycle` does not have a default value.
This change allows configuring this runtime setting via Contour configuration to allow administrators of Contour to prevent abusive connections from starving resources from other valid connections.
The default is left as the existing behavior (no limit) so as not to impact existing valid traffic.

The Contour ConfigMap can be modified similar to the following (and Contour restarted) to set this value:

```
listener:
  max-requests-per-io-cycle: 10
```

(Note this can be used in addition to the existing Listener configuration field `listener.max-requests-per-connection` which is used primarily for HTTP/1.1 connections and is an approximate limit for HTTP/2)

See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.27.1/version_history/v1.27/v1.27.1) for more details.

(#5827, @sunjayBhatia)

## HTTP/2 max concurrent streams is configurable

This field can be used to limit the number of concurrent streams Envoy will allow on a single connection from a downstream peer.
It can be used to tune resource usage and as a mitigation for DOS attacks arising from vulnerabilities like CVE-2023-44487.

The Contour ConfigMap can be modified similar to the following (and Contour restarted) to set this value:

```
listener:
  http2-max-concurrent-streams: 50
```

(#5850, @sunjayBhatia)


# Other Changes
- Add flags: `--incluster`, `--kubeconfig` for enable run the `gateway-provisioner` in or out of the cluster. (#5686, @izturn)
- Gateway provisioner: Add the `overloadMaxHeapSize` configuration option to contourDeployment to allow adding [overloadManager](https://projectcontour.io/docs/main/config/overload-manager/) configuration when generating envoy's initial configuration file. (#5699, @yangyy93)
- Drops the Gateway API webhook from example manifests and testing since validations are now implemented in Common Expression Language (CEL). (#5735, @skriss)
- Gateway API: set Listeners' `ResolvedRefs` condition to `true` by default. (#5804, @skriss)
- Updates to Go 1.21.3. See the [Go release notes](https://go.dev/doc/devel/release#go1.21.minor) for more information. (#5841, @sunjayBhatia)
- Updates Envoy to v1.28.0. See the release notes [here](https://www.envoyproxy.io/docs/envoy/v1.28.0/version_history/v1.28/v1.28.0). (#5870, @skriss)


# Docs Changes
- Switch to documenting the Gateway API release semantic version instead of API versions in versions.yaml and the [compatibility matrix](https://projectcontour.io/resources/compatibility-matrix/), to provide more information about features available with each release. (#5871, @skriss)


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.27.0 is tested against Kubernetes 1.26 through 1.28.

# Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:

- @clayton-gonsalves
- @davinci26
- @izturn
- @shadialtarsha
- @therealak12
- @yangyy93


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
