---
title: Compatibility Matrix
layout: page
---

This page documents the compatibility matrix of versions of Contour, Envoy, Kubernetes, and the Contour Operator.
These combinations of versions are specifically tested and supported by the Contour maintainers.
Other combinations *may* work, but are not tested or supported.

## Compatibility Matrix

| Contour Version | Envoy Version        | Kubernetes Versions | Operator Version |
| --------------- | :------------------- | ------------------- | ---------------- |
| main            | [1.18.2][12]         | 1.21, 1.20, 1.19    | [main][50]       |
| 1.15.0          | [1.18.2][12]         | 1.21, 1.20, 1.19    | [1.15.0][57]     |
| 1.14.1          | [1.17.2][11]         | 1.20, 1.19, 1.18    | [1.14.1][56]     |
| 1.14.0          | [1.17.1][10]         | 1.20, 1.19, 1.18    | [1.14.0][55]     |
| 1.13.1          | [1.17.1][10]         | 1.20, 1.19, 1.18    | [1.13.1][54]     |
| 1.13.0          | [1.17.0][9]          | 1.20, 1.19, 1.18    | [1.13.0][53]     |
| 1.12.0          | [1.17.0][9]          | 1.19, 1.18, 1.17    | [1.12.0][52]     |
| 1.11.0          | [1.16.2][8]          | 1.19, 1.18, 1.17    | [1.11.0][51]     |
| 1.10.1          | [1.16.2][8]          | 1.19, 1.18, 1.17    | N/A              |
| 1.10.0          | [1.16.0][7]          | 1.19, 1.18, 1.17    | N/A              |
| 1.9.0           | [1.15.1][6]          | 1.18, 1.17, 1.16    | N/A              |
| 1.8.2           | [1.15.1][6]          | 1.18, 1.17, 1.16    | N/A              |
| 1.8.1           | [1.15.0][5]          | 1.18, 1.17, 1.16    | N/A              |
| 1.8.0           | [1.15.0][5]          | 1.18, 1.17, 1.16    | N/A              |
| 1.7.0           | [1.15.0][5]          | 1.18, 1.17, 1.16    | N/A              |
| 1.6.1           | [1.14.3][4]          | 1.18, 1.17, 1.16    | N/A              |
| 1.6.0           | [1.14.2][3]          | 1.18, 1.17, 1.16    | N/A              |
| 1.5.1           | [1.14.2][3]          | 1.18, 1.17, 1.16    | N/A              |
| 1.5.0           | [1.14.1][2]          | 1.18, 1.17, 1.16    | N/A              |
| 1.4.0           | [1.14.1][2]          | 1.17, 1.16, 1.15    | N/A              |

<br />

## Notes on Compatibility
Contour utilizes [client-go][98] to watch for resources in a Kubernetes cluster.
Since Kubernetes is backwards compatible with clients, older client-go versions will work with many different Kubernetes cluster versions.
Contour also only consumes a small number of quite stable Kubernetes APIs.
This means that Contour is *likely* compatible with more Kubernetes versions than those listed in the matrix.
However, combinations not listed are not tested or supported by the Contour maintainers.

The `client-go` package includes a [compatibility matrix][99] as to what Kubernetes API versions are supported with the version of client-go.

__Note:__ Since Contour now uses `apiextensions.k8s.io/v1` for Custom Resource Definitions (CRDs), the minimum compatible Kubernetes version is v1.16.

## Envoy Extensions
Contour requires the following Envoy extensions.
If you are using the image recommended in our [example deployment][1] no action is required.
If you are providing your own Envoy it must be compiled with the following extensions:

- Access Loggers: 
  - envoy.access_loggers.file
  - envoy.access_loggers.http_grpc
  - envoy.access_loggers.tcp_grpc
  
- Compression:
  - envoy.compression.gzip.compressor
    
- HTTP Filters:
  - envoy.filters.http.compressor
  - envoy.filters.http.cors
  - envoy.filters.http.ext_authz
  - envoy.filters.http.grpc_stats
  - envoy.filters.http.grpc_web
  - envoy.filters.http.health_check
  - envoy.filters.http.lua
  - envoy.filters.http.router
   
- Listener filters
  - envoy.filters.listener.http_inspector
  - envoy.filters.listener.original_dst
  - envoy.filters.listener.proxy_protocol
  - envoy.filters.listener.tls_inspector

- Network filters
  - envoy.filters.network.client_ssl_auth
  - envoy.filters.network.ext_authz
  - envoy.filters.network.http_connection_manager
  - envoy.filters.network.tcp_proxy
  
- Transport sockets
  - envoy.transport_sockets.upstream_proxy_protocol
  - envoy.transport_sockets.raw_buffer
  
- Http Upstreams
  - envoy.upstreams.http.http
  - envoy.upstreams.http.tcp

__Note:__ This list of extensions was last verified to be complete with Envoy v1.16.1.


[1]: {{site.github.repository_url}}/tree/{{site.github.latest_release.tag_name}}/examples/contour

[2]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.14.1
[3]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.14.2
[4]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.14.3
[5]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.15.0
[6]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.15.1
[7]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.16.0
[8]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.16.2
[9]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.0
[10]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.1
[11]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.2
[12]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.18.2


[50]: https://github.com/projectcontour/contour-operator
[51]: https://github.com/projectcontour/contour-operator/releases/tag/v1.11.0
[52]: https://github.com/projectcontour/contour-operator/releases/tag/v1.12.0
[53]: https://github.com/projectcontour/contour-operator/releases/tag/v1.13.0
[54]: https://github.com/projectcontour/contour-operator/releases/tag/v1.13.1
[55]: https://github.com/projectcontour/contour-operator/releases/tag/v1.14.0
[56]: https://github.com/projectcontour/contour-operator/releases/tag/v1.14.1
[57]: https://github.com/projectcontour/contour-operator/releases/tag/v1.15.0

[98]: https://github.com/kubernetes/client-go
[99]: https://github.com/kubernetes/client-go#compatibility-matrix
