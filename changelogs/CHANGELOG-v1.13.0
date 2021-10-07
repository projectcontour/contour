We are delighted to present version 1.13.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

# Major Changes

## Gateway API Support

Contour now provides initial support for [Gateway API](https://gateway-api.sigs.k8s.io/), an open source project to evolve service networking APIs within the Kubernetes ecosystem. Gateway API consists of multiple resources that provide user interfaces to expose Kubernetes applications- Services, Ingress, and more. See the [user guide](https://projectcontour.io/guides/gateway-api/) for additional details and to start using Gateway API with Contour.

Related issues and PRs: #3278 #3397 #2809 #3283

Thanks to @stevesloka and @youngnick for designing and implementing this feature.

## Global Rate Limiting

There are times when distributed circuit breaking is not very effective and global rate limiting is desired. With global rate limiting, Envoy communicates with an external Rate Limit Service (RLS) over gRPC to make rate limit decisions for requests. For additional details, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/v1.17.0/intro/arch_overview/other_features/global_rate_limiting.html).

Related issues and PRs: #3178 #3298 #3324

Thanks to @skriss for designing and implementing this feature!

**Known Issues**: #3409 (global rate limit policies at the virtual host level for TLS vhosts do not take effect).

## Configurable Global TLS Cipher Suites

TLS cipher suites used by Envoy listeners can now be configured. The configured cipher suites are validated against Envoy's allowed cipher list. Contour will exit on startup if any invalid cipher suites are present in the config file. If no cipher suites are provided, Contour will use the defaults that exist now.

Related issues and PRs: #2880 #3292 #3304

## Configurable Delay Close Timeout

There are situations where Envoy's "delayed_close_timeout" can close connections to a client when data remains to be written. This can happen when a client sets the "Connection: close" header and is slow to read the response. The 'delayed_close_timeout' can now be configured by users who encounter this situation.

Related issues and PRs: #3285 #3316

Thanks to @xtreme-jesse-malone for implementing this feature!

## Configurable XffNumTrustedHops

If a user has an external load balancer that terminates TLS, the X-Forwarded-Proto header gets overwritten unless the downstream connection is trusted. XffNumTrustedHops can now be configured to set the number of trusted hops which will allow the headers to be intact already set from downstream.

Related issues and PRs: #3294 #3293

Thanks to @stevesloka for implementing this feature!

## ExactBalance Connection Balancer
ExactBalance is a connection balancer implementation that does exact balancing. This means that a lock is held during balancing so that connection counts are nearly exactly balanced between worker threads. With long keep-alive connections, the Envoy listener will use the ExactBalance connection balancer. For additional details, see the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/v1.17.0/api-v2/api/v2/listener.proto#envoy-api-msg-listener-connection).

Related issues and PRs: #3314

Thanks to @iyacontrol for implementing this feature!

## Set SNI for Upstream externalName Clusters
SNI will be set on any TCPProxy.Service which references an externalName type service as well as having the upstream protocol of "tls".

Related issues and PRs: #3291

Thanks to @stevesloka for implementing this feature!

## Dynamic Service Headers
Adds support for %CONTOUR_NAMESPACE%, %CONTOUR_SERVICE_NAME% and %CONTOUR_SERVICE_PORT% dynamic variables. These variables will be expanded like the Envoy dynamic variables in #3234. __Note:__ The CONTOUR_ prefix is used to prevent the clashing with a future Envoy dynamic variable. Variables that can't be expanded are passed through literally.

Related issues and PRs: #3269

Thanks to @erwbgy for implementing this feature!

# Deprecation & Removal Notices
- The deprecated `FileAccessLog.json_format` access logging format field is replaced by `envoy.extensions.access_loggers.file.v3.FileAccessLog.log_format`. See #3210 for additional details.
- The deprecated cluster `Http2ProtocolOptions` field is replaced with `TypedExtensionProtocolOptions`. See #3308 for additional details.
- Insecure AES128/256 ciphers are disabled by default. See #3304 for additional details.
- The following Prometheus Gauges have been renamed to make the metric names follow promlint conventions. We encourage operators to have dashboard and alert queries refer to the new names. The old metrics will be removed completely in the next release:
   ```
   contour_httpproxy_total -> contour_httpproxy
   contour_httpproxy_invalid_total  -> contour_httpproxy_invalid
   contour_httpproxy_orphaned_total  -> contour_httpproxy_orphaned
   contour_httpproxy_valid_total  -> contour_httpproxy_valid
   contour_httpproxy_root_total  -> contour_httpproxy_root
   ```

# Upgrading
Please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

## Community Thanks!
We’re immensely grateful for all the community contributions that help make Contour even better! For version 1.13, special thanks go out to the following contributors:
- @xtreme-jesse-malone
- @abhide
- @seemiller

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
