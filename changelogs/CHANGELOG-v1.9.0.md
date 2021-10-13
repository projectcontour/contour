We are delighted to present version 1.9.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

There's been a bunch of great contributions from our community for this release, thanks to everyone!

## External Authorization Support
Contour now supports integrating with external authorization services via the ExtensionService custom resource definition. This new Contour API exposes Envoy’s [external auth filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/network_filters/ext_authz_filter#config-network-filters-ext-authz), which allows incoming requests to be checked against the specified authorization service.

Thanks to @jpeach for leading design and implementation of this feature!

Related issues and PRs: #432, #2915, #2886, #2876, #2877, #2871

## Backend TLS Client Authentication
Contour now supports optionally specifying a Kubernetes secret that Envoy should present to upstream clusters as a client certificate for TLS, so the upstream services can validate that the connection is coming from Envoy.

Thanks to @tsaarni for leading design and implementation of this feature!

Related issues and PRs: #2338, #2910

## Cross-Origin Resource Sharing (CORS) Support
Contour’s HTTPProxy API now supports specifying a CORS policy, which configures Envoy’s [CORS filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/cors_filter) to allow web applications to request resources from different origins.

Thanks to @aberasarte and @glerchundi for driving the design and implementation of this new feature!  

Related issues and PRs: #437, #2890

## v1 Custom Resource Definitions
Contour now generates v1 custom resource definitions (CRDs) as part of its example YAML. This enables Contour to take full advantage of the v1 API’s capabilities around validation, defaulting, API documentation via `kubectl explain`, and more. CRDs became [generally available in Kubernetes 1.16](https://kubernetes.io/blog/2019/09/18/kubernetes-1-16-release-announcement/#custom-resources-reach-general-availability) over a year ago.

This change bumps Contour’s minimum supported Kubernetes version to 1.16.

Related issues and PRs: #2916, #2678, #1723, #1978, #2903, #2527

## HTTPProxy Conditions
Contour’s HTTPProxy and ExtensionService CRDs now expose [Conditions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties). Each custom resource, when processed by Contour, will have a single Condition, of type Valid, that will have a value of true or false to indicate whether or not the resource is valid. The Valid condition will further have a set of sub-conditions that provide more detail on the reason(s) for the resource’s validity/non-validity.

The existing HTTPProxy status fields `currentStatus` and `description` will be retained for backwards compatibility.

Thanks to @youngnick for designing and implementing this feature!

Related issues and PRs: #2962, #2931

## Experimental go-control-plane Support
Contour now has experimental support for [Envoy’s go-control-plane](https://github.com/envoyproxy/go-control-plane) xDS server implementation. When enabled, this replaces Contour’s custom xDS gRPC server implementation. This feature can be enabled by setting the server.xds-server-type to “envoy” in the Contour config file.

Thanks to @stevesloka for designing and implementing this feature!

Related issues and PRs: #2134, #2850, #2884, #2919

## Configurable DNS Lookup Family for ExternalName Services
We’ve added a config file field, cluster.dns-lookup-family, to customize DNS behavior for Kubernetes externalName services. Valid options are auto (default), v4, and v6. Previously, auto was always used, which first looks for an IPv6 address, and falls back to looking for an IPv4 address.

Thanks @stevesloka for debugging this issue and implementing the fix!

Related issues and PRs: #2894, #2873

## Timeout Field Validation
Contour now performs validation on all timeout fields/annotations on the HTTPProxy and Ingress APIs. Invalid values will be rejected at creation time where possible, and will otherwise be surfaced to the user as invalid HTTPProxies, or as errors in the Contour log. Previously, Contour would disable the timeout entirely if the configured value was not a valid duration string.

Related issues and PRs: #2728, #2913, #2905

## Deprecation Notices
⚠️ In Contour 1.10, we will be deprecating TLS 1.1 and lower. TLS 1.2 will become the default minimum TLS version. TLS 1.1 can still be enabled, but will require explicit configuration. If you need to use TLS 1.1 going forward, you will need to explicitly enable it via the Contour config file, and via the HTTPProxy API’s minimumProtocolVersion field.

⚠️ In Contour 1.10, we will be removing the request-timeout field from the config file. This field was moved into the timeouts block, i.e. timeouts.request-timeout, in Contour 1.7, and all support for the old field will be dropped.

## Upgrading
Please consult the upgrade [documentation](https://projectcontour.io/resources/upgrading/).

## Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).