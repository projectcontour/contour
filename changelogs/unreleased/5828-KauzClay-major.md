## Upstream TLS now supports TLS 1.3 and TLS parameters can be configured

The default maximum TLS version for upstream connections is now 1.3, instead of the Envoy default of 1.2.

In a similar way to how Contour users can configure Min/Max TLS version and
Cipher Suites for Envoy's listeners, users can now specify the
same information for upstream connections. In the ContourConfiguration, this is
available under `spec.envoy.cluster.upstreamTLS`. The equivalent config file
parameter is `cluster.upstream-tls`.
