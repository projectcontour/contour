---
title: Compatibility Matrix
layout: page
---

This page documents the compatibility matrix of versions of Contour, Envoy, Kubernetes, and the Contour Operator.
These combinations of versions are specifically tested in CI and supported by the Contour maintainers.

## Compatibility Matrix

| Contour Version | Envoy Version        | Kubernetes Versions | Operator Version | Gateway API Version |
| --------------- | :------------------- | ------------------- | ---------------- | --------------------|
| main            | [1.25.4][32]         | 1.26, 1.25, 1.24    | N/A              | v1alpha2, v1beta1   |
| 1.24.3          | [1.25.4][32]         | 1.26, 1.25, 1.24    | N/A              | v1alpha2, v1beta1   |
| 1.24.2          | [1.25.2][31]         | 1.26, 1.25, 1.24    | N/A              | v1alpha2, v1beta1   |
| 1.24.1          | [1.25.1][28]         | 1.26, 1.25, 1.24    | N/A              | v1alpha2, v1beta1   |
| 1.24.0          | [1.25.0][25]         | 1.26, 1.25, 1.24    | [1.24.0][75]     | v1alpha2, v1beta1   |
| 1.23.5          | [1.24.5][33]         | 1.25, 1.24, 1.23    | N/A              | v1alpha2, v1beta1   |
| 1.23.4          | [1.24.3][30]         | 1.25, 1.24, 1.23    | N/A              | v1alpha2, v1beta1   |
| 1.23.3          | [1.24.2][27]         | 1.25, 1.24, 1.23    | N/A              | v1alpha2, v1beta1   |
| 1.23.2          | [1.24.1][24]         | 1.25, 1.24, 1.23    | N/A              | v1alpha2, v1beta1   |
| 1.23.1          | [1.24.1][24]         | 1.25, 1.24, 1.23    | N/A              | v1alpha2, v1beta1   |
| 1.23.0          | [1.24.0][21]         | 1.25, 1.24, 1.23    | [1.23.0][74]     | v1alpha2, v1beta1   |
| 1.22.6          | [1.23.7][34]         | 1.24, 1.23, 1.22    | N/A              | v1alpha2, v1beta1   |
| 1.22.5          | [1.23.5][29]         | 1.24, 1.23, 1.22    | N/A              | v1alpha2, v1beta1   |
| 1.22.4          | [1.23.4][26]         | 1.24, 1.23, 1.22    | N/A              | v1alpha2, v1beta1   |
| 1.22.3          | [1.23.3][23]         | 1.24, 1.23, 1.22    | N/A              | v1alpha2, v1beta1   |
| 1.22.2          | [1.23.3][23]         | 1.24, 1.23, 1.22    | N/A              | v1alpha2, v1beta1   |
| 1.22.1          | [1.23.1][20]         | 1.24, 1.23, 1.22    | [1.22.1][73]     | v1alpha2, v1beta1   |
| 1.22.0          | [1.23.0][19]         | 1.24, 1.23, 1.22    | [1.22.0][72]     | v1alpha2, v1beta1   |
| 1.21.3          | [1.22.6][22]         | 1.23, 1.22, 1.21    | N/A              | v1alpha2            |
| 1.21.2          | [1.22.6][22]         | 1.23, 1.22, 1.21    | N/A              | v1alpha2            |
| 1.21.1          | [1.22.2][17]         | 1.23, 1.22, 1.21    | [1.21.1][70]     | v1alpha2            |
| 1.21.0          | [1.22.0][16]         | 1.23, 1.22, 1.21    | [1.21.0][69]     | v1alpha2            |
| 1.20.2          | [1.21.3][18]         | 1.23, 1.22, 1.21    | [1.20.2][71]     | v1alpha2            |
| 1.20.1          | [1.21.1][15]         | 1.23, 1.22, 1.21    | [1.20.1][68]     | v1alpha2            |
| 1.20.0          | [1.21.0][14]         | 1.23, 1.22, 1.21    | [1.20.0][67]     | v1alpha2            |
| 1.19.1          | [1.19.1][13]         | 1.22, 1.21, 1.20    | [1.19.1][65]     | v1alpha1            |
| 1.19.0          | [1.19.1][13]         | 1.22, 1.21, 1.20    | [1.19.0][64]     | v1alpha1            |
| 1.18.3          | [1.19.1][13]         | 1.21, 1.20, 1.19    | [1.18.3][66]     | v1alpha1            |
| 1.18.2          | [1.19.1][13]         | 1.21, 1.20, 1.19    | [1.18.2][63]     | v1alpha1            |
| 1.18.1          | [1.19.1][13]         | 1.21, 1.20, 1.19    | [1.18.1][62]     | v1alpha1            |
| 1.18.0          | [1.19.0][10]         | 1.21, 1.20, 1.19    | [1.18.0][61]     | v1alpha1            |
| 1.17.2          | [1.18.4][12]         | 1.21, 1.20, 1.19    | N/A              | v1alpha1            |
| 1.17.1          | [1.18.3][9]          | 1.21, 1.20, 1.19    | N/A              | v1alpha1            |
| 1.17.0          | [1.18.3][9]          | 1.21, 1.20, 1.19    | [1.17.0][60]     | v1alpha1            |
| 1.16.1          | [1.18.4][12]         | 1.21, 1.20, 1.19    | N/A              | v1alpha1            |
| 1.16.0          | [1.18.3][9]          | 1.21, 1.20, 1.19    | [1.16.0][59]     | v1alpha1            |
| 1.15.2          | [1.18.4][12]         | 1.21, 1.20, 1.19    | N/A              | v1alpha1            |
| 1.15.1          | [1.18.3][9]          | 1.21, 1.20, 1.19    | [1.15.1][58]     | v1alpha1            |
| 1.15.0          | [1.18.2][8]          | 1.21, 1.20, 1.19    | [1.15.0][57]     | v1alpha1            |
| 1.14.2          | [1.17.4][11]         | 1.20, 1.19, 1.18    | N/A              | v1alpha1            |
| 1.14.1          | [1.17.2][7]          | 1.20, 1.19, 1.18    | [1.14.1][56]     | v1alpha1            |
| 1.14.0          | [1.17.1][6]          | 1.20, 1.19, 1.18    | [1.14.0][55]     | v1alpha1            |
| 1.13.1          | [1.17.1][6]          | 1.20, 1.19, 1.18    | [1.13.1][54]     | v1alpha1            |
| 1.13.0          | [1.17.0][5]          | 1.20, 1.19, 1.18    | [1.13.0][53]     | v1alpha1            |
| 1.12.0          | [1.17.0][5]          | 1.19, 1.18, 1.17    | [1.12.0][52]     | N/A                 |
| 1.11.0          | [1.16.2][4]          | 1.19, 1.18, 1.17    | [1.11.0][51]     | N/A                 |
| 1.10.1          | [1.16.2][4]          | 1.19, 1.18, 1.17    | N/A              | N/A                 |
| 1.10.0          | [1.16.0][3]          | 1.19, 1.18, 1.17    | N/A              | N/A                 |
| 1.9.0           | [1.15.1][2]          | 1.18, 1.17, 1.16    | N/A              | N/A                 |

<br />

## Notes on Compatibility

**As of Contour version 1.16.0, Contour only subscribes to Ingress v1 resources (and no longer falls back to Ingress v1beta1). The minimum compatible Kubernetes version for Contour 1.16.0 and above is Kubernetes 1.19.**

Contour utilizes [client-go][98] to watch for resources in a Kubernetes cluster.
We depend on the latest version of the library and by extension only support the latest versions of Kubernetes.
While the `client-go` [compatibility matrix][99] may list older versions of Kubernetes as being compatible and supported by upstream, the Contour project only tests a given version of Contour against the versions listed in the table above.
Combinations not listed are not tested, guaranteed to work, or supported by the Contour maintainers.

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


[1]: {{< param github_url >}}/tree/{{< param latest_version >}}/examples/contour

[2]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.15.1
[3]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.16.0
[4]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.16.2
[5]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.0
[6]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.1
[7]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.17.2
[8]: https://www.envoyproxy.io/docs/envoy/latest/version_history/v1.18.2
[9]: https://www.envoyproxy.io/docs/envoy/v1.18.3/version_history/current
[10]: https://www.envoyproxy.io/docs/envoy/v1.19.0/version_history/current
[11]: https://www.envoyproxy.io/docs/envoy/v1.17.4/version_history/current
[12]: https://www.envoyproxy.io/docs/envoy/v1.18.4/version_history/current
[13]: https://www.envoyproxy.io/docs/envoy/v1.19.1/version_history/current
[14]: https://www.envoyproxy.io/docs/envoy/v1.21.0/version_history/current
[15]: https://www.envoyproxy.io/docs/envoy/v1.21.1/version_history/current
[16]: https://www.envoyproxy.io/docs/envoy/v1.22.0/version_history/current
[17]: https://www.envoyproxy.io/docs/envoy/v1.22.2/version_history/current
[18]: https://www.envoyproxy.io/docs/envoy/v1.21.3/version_history/current
[19]: https://www.envoyproxy.io/docs/envoy/v1.23.0/version_history/v1.23/v1.23.0
[20]: https://www.envoyproxy.io/docs/envoy/v1.23.1/version_history/v1.23/v1.23.1
[21]: https://www.envoyproxy.io/docs/envoy/v1.24.0/version_history/v1.24/v1.24.0
[22]: https://www.envoyproxy.io/docs/envoy/v1.22.6/version_history/current
[23]: https://www.envoyproxy.io/docs/envoy/v1.23.3/version_history/v1.23/v1.23.3
[24]: https://www.envoyproxy.io/docs/envoy/v1.24.1/version_history/v1.24/v1.24.1
[25]: https://www.envoyproxy.io/docs/envoy/v1.25.0/version_history/v1.25/v1.25.0
[26]: https://www.envoyproxy.io/docs/envoy/v1.23.4/version_history/v1.23/v1.23.4
[27]: https://www.envoyproxy.io/docs/envoy/v1.24.2/version_history/v1.24/v1.24.2
[28]: https://www.envoyproxy.io/docs/envoy/v1.25.1/version_history/v1.25/v1.25.1
[29]: https://www.envoyproxy.io/docs/envoy/v1.23.5/version_history/v1.23/v1.23.5
[30]: https://www.envoyproxy.io/docs/envoy/v1.24.3/version_history/v1.24/v1.24.3
[31]: https://www.envoyproxy.io/docs/envoy/v1.25.2/version_history/v1.25/v1.25.2
[32]: https://www.envoyproxy.io/docs/envoy/v1.25.4/version_history/v1.25/v1.25.4
[33]: https://www.envoyproxy.io/docs/envoy/v1.24.5/version_history/v1.24/v1.24.5
[34]: https://www.envoyproxy.io/docs/envoy/v1.23.7/version_history/v1.23/v1.23.7

[50]: https://github.com/projectcontour/contour-operator
[51]: https://github.com/projectcontour/contour-operator/releases/tag/v1.11.0
[52]: https://github.com/projectcontour/contour-operator/releases/tag/v1.12.0
[53]: https://github.com/projectcontour/contour-operator/releases/tag/v1.13.0
[54]: https://github.com/projectcontour/contour-operator/releases/tag/v1.13.1
[55]: https://github.com/projectcontour/contour-operator/releases/tag/v1.14.0
[56]: https://github.com/projectcontour/contour-operator/releases/tag/v1.14.1
[57]: https://github.com/projectcontour/contour-operator/releases/tag/v1.15.0
[58]: https://github.com/projectcontour/contour-operator/releases/tag/v1.15.1
[59]: https://github.com/projectcontour/contour-operator/releases/tag/v1.16.0
[60]: https://github.com/projectcontour/contour-operator/releases/tag/v1.17.0
[61]: https://github.com/projectcontour/contour-operator/releases/tag/v1.18.0
[62]: https://github.com/projectcontour/contour-operator/releases/tag/v1.18.1
[63]: https://github.com/projectcontour/contour-operator/releases/tag/v1.18.2
[64]: https://github.com/projectcontour/contour-operator/releases/tag/v1.19.0
[65]: https://github.com/projectcontour/contour-operator/releases/tag/v1.19.1
[66]: https://github.com/projectcontour/contour-operator/releases/tag/v1.18.3
[67]: https://github.com/projectcontour/contour-operator/releases/tag/v1.20.0
[68]: https://github.com/projectcontour/contour-operator/releases/tag/v1.20.1
[69]: https://github.com/projectcontour/contour-operator/releases/tag/v1.21.0
[70]: https://github.com/projectcontour/contour-operator/releases/tag/v1.21.1
[71]: https://github.com/projectcontour/contour-operator/releases/tag/v1.20.2
[72]: https://github.com/projectcontour/contour-operator/releases/tag/v1.22.0
[73]: https://github.com/projectcontour/contour-operator/releases/tag/v1.22.1
[74]: https://github.com/projectcontour/contour-operator/releases/tag/v1.23.0
[75]: https://github.com/projectcontour/contour-operator/releases/tag/v1.24.0

[98]: https://github.com/kubernetes/client-go
[99]: https://github.com/kubernetes/client-go#compatibility-matrix
