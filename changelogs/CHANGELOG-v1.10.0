We are delighted to present version 1.10.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

There have been a bunch of great contributions from our community for this release, thanks to everyone!

# Major Changes

## Envoy xDS v3 Support
Contour now supports Envoy's xDS v3 protocol in addition to the deprecated v2 protocol. The example YAML has been updated to configure Envoy to use the v3 protocol by default.

When users have an existing Contour installation and wish to upgrade without dropping connections, users should first upgrade Contour  to v1.10.0 which will serve both v2 and v3 xDS versions from the same gRPC endpoint. Next, change the Envoy Daemonset or deployment to include `--xds-resource-version=v3` as an argument in the `envoy-initconfig` init container, which tells Envoy to upgrade to the `v3` resource version. The usual rollout process will handle draining connections allowing a fleet of Envoy instances to move from the v2 xDS Resource API version gradually to the v3 version.

See the xDS Migration guide for more information: https://projectcontour.io/guides/xds-migration/

Related issues and PRs: #1898, #2930, #3016, #3017, #3068, #3079, #3074, #3087, #3093

Thanks to @stevesloka and @jpeach for their hard work on this upgrade.

## Custom JSON fields for Envoy access logs
Contour now supports custom JSON fields in the Envoy access log. Custom fields can be specified in the `json-fields` config field, using the format `<custom-field-name>=<Envoy format string>`, where the Envoy format string can contain [any Envoy command operator](https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log/usage#command-operators) except `DYNAMIC_METADATA` and `FILTER_STATE`. 

You can read more about this feature in Contour's [updated guide to structured logging](https://projectcontour.io/guides/structured-logs/).

Related issues and PRs: #3059, #3032, #1507

Thanks to @Mike1808, @KauzClay, and @XanderStrike for designing and implementing this feature!

## Multi-arch Docker images
Contour's Docker images are now multi-architecture, with `linux/amd64` and `linux/arm64` currently supported. No change is needed by users; the correct architecture will be automatically be pulled for your host.

Related issues and PRs: #3031, #2868

Thanks to @skriss for implementing multi-arch support.

## Envoy 1.16

Contour now uses Envoy 1.16.0.

Related issues and PRs: #3029, #3013

Thanks to @yoitsro for this upgrade!

## Default minimum TLS version is now 1.2

TLS 1.2 is now the default minimum TLS version for `HTTPProxies` and `Ingresses`. It's still possible to use 1.1 if necessary by explicitly specifying it. See the [HTTPProxy documentation](https://projectcontour.io/docs/v1.10.0/config/tls-termination/) and [Ingress documentation](https://projectcontour.io/docs/v1.10.0/config/annotations/#contour-specific-ingress-annotations) for more information.

Related issues and PRs: #3007, #2777, #3012

Thanks to @skriss for making this change.

## RBAC v1

Contour's example YAML now uses `rbac.authorization.k8s.io/v1` instead of the deprecated `rbac.authorization.k8s.io/v1beta1` version for role-based access control (RBAC) resources. RBAC has been generally available in Kubernetes since v1.8, so this has no effect on the minimum supported Kubernetes version.

Related issues and PRs: #3015, #2991

Thanks to @narahari92 for this upgrade!

# Deprecation & Removal Notices
- The `request-timeout` field has been removed from the config file. This field was moved into the timeouts block, i.e. `timeouts.request-timeout`, in Contour 1.7.
- In Contour 1.11, TLS 1.1 will be disabled by default. Users who require TLS 1.1 will have to enable it via the config file's `tls.minimum-protocol-version` field, and by specifying it for each `HTTPProxy` or `Ingress` where it's needed. See the [HTTPProxy documentation](https://projectcontour.io/docs/v1.10.0/config/tls-termination/) and [Ingress documentation](https://projectcontour.io/docs/v1.10.0/config/annotations/#contour-specific-ingress-annotations) for more information.

# Upgrading
Please consult the upgrade [documentation](https://projectcontour.io/resources/upgrading/).

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
