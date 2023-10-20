We are delighted to present version v1.25.3 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

- [All Changes](#all-changes)
- [Installing/Upgrading](#installing-and-upgrading)
- [Compatible Kubernetes Versions](#compatible-kubernetes-versions)

# All Changes

This release includes various dependency bumps and fixes for [CVE-2023-44487](https://nvd.nist.gov/vuln/detail/CVE-2023-44487), including:

- Update to Envoy v1.26.6. See the release notes for v1.26.5 [here](https://www.envoyproxy.io/docs/envoy/v1.26.5/version_history/v1.26/v1.26.5) and v1.26.6 [here](https://www.envoyproxy.io/docs/envoy/v1.26.6/version_history/v1.26/v1.26.6).
- Update to Go v1.20.10. See the [Go release notes](https://go.dev/doc/devel/release#go1.20.minor) for more information.

Additional mitigations have been added for CVE-2023-44487 in the form of new configuration fields:

## Max HTTP requests per IO cycle is configurable as an additional mitigation for HTTP/2 CVE-2023-44487

Envoy mitigates CVE-2023-44487 with some default runtime settings, however the `http.max_requests_per_io_cycle` does not have a default value.
This change allows configuring this runtime setting via Contour configuration to allow administrators of Contour to prevent abusive connections from starving resources from other valid connections.
The default is left as the existing behavior (no limit) so as not to impact existing valid traffic.

The Contour ConfigMap can be modified similar to the following (and Contour restarted) to set this value:

```
listener:
  max-requests-per-io-cycle: 10
```

(Note this can be used in addition to the existing Listener configuration field `listener.max-requests-per-connection` which is used primarily for HTTP/1.1 connections and is an approximate limit for HTTP/2)

## HTTP/2 max concurrent streams is configurable

This field can be used to limit the number of concurrent streams Envoy will allow on a single connection from a downstream peer.
It can be used to tune resource usage and as a mitigation for DOS attacks arising from vulnerabilities like CVE-2023-44487.

The Contour ConfigMap can be modified similar to the following (and Contour restarted) to set this value:

```
listener:
  http2-max-concurrent-streams: 50
```


# Installing and Upgrading

For a fresh install of Contour, consult the [getting started documentation](https://projectcontour.io/getting-started/).

To upgrade an existing Contour installation, please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).


# Compatible Kubernetes Versions

Contour v1.25.3 is tested against Kubernetes 1.25 through 1.27.


# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
