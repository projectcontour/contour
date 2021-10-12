We are delighted to present version 1.12.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

There have been a bunch of great contributions from our community for this release, thanks to everyone!

# Major Changes

## Local Rate Limiting Support

Contour now supports Envoy's local rate limiting, which allows users to define basic rate limits for virtual hosts or routes that are enforced on a per-Envoy basis. See [the rate limiting documentation](https://projectcontour.io/docs/v1.12.0/config/rate-limiting/) for more information.

Related issues and PRs: #3255 #3251

Thanks to @skriss for implementing this feature!

## Header Hash Load Balancing

Contour 1.12 now supports the `RequestHash` load balancing strategy, which enables load balancing based on request headers. An upstream Endpoint is selected based on the hash of an HTTP request header. Requests that contain a consistent value in a request header will be routed to the same upstream Endpoint.

For more information, including an example `HTTPProxy` definition, see the [Contour documentation](https://projectcontour.io/docs/v1.12.0/config/request-routing/#load-balancing-strategy).

Related issues and PRs: #3099 #3044 #3282 

Thanks to @sunjayBhatia for designing and implementing this feature!

## Dynamic Request Headers

Contour 1.12 adds support for including dynamic values in configured request and response headers. Almost all [variables supported by Envoy](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#custom-request-response-headers) are allowed. This feature can be used to set headers containing information such as the host name of where the Envoy pod is running, the TLS version, etc.

For more information, including a full list of supported variables, see the [Contour documentation](https://projectcontour.io/docs/v1.12.0/config/request-rewriting/#dynamic-header-values).

Related issues and PRs: #3234 #3236 #3269

Thanks to @erwbgy for adding this feature!

## Reverting some TLS Cipher changes

Contour has been making an effort to remove ciphers marked as "less secure" from the default cipher list given to Envoy. This work has been driven by @tsaarni and @ryanelian and @bascht. However, after the release of 1.11, we had a report of a production outage caused by these changes (#3299, thanks @moderation).

We've decided to revert PRs #3154 and #3237 for version 1.12, until we can fully implement #2880, so that if the default cipher suite breaks something for a user, they can put it back after upgrading until they have a chance to migrate away from the less-secure ciphers.

We're aiming to have #2880 completed in the 1.13 timeframe (the next month).

## Envoy 1.17.0

Contour 1.12.0 is compatible with Envoy 1.17.0.

Related issues and PRs: #3245

Thanks to @sunjayBhatia for performing this upgrade!

## Configurable allow_chunked_length

Envoy's `allow_chunked_length` setting is now enabled by default, with a Contour config file toggle to disable it. See the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/core/v3/protocol.proto#config-core-v3-http1protocoloptions) for more information on the setting, and the [Contour config file documentation](https://projectcontour.io/docs/main/configuration/#configuration-file) for information on how to disable it.

Related issues and PRs: #3221 #3248 

Thanks to @sunjayBhatia for making this change!

## Case-Insensitive Duplicate FQDN Check

Contour's check for duplicate fully-qualified domain names (FQDNs) is now case-insensitive.

Related issues and PRs: #3230 #3231

Thanks to @erwbgy for this fix!

## Fix for Rewriting Host Header

We fixed a regression related to rewriting the `Host` header for `externalName` services.

Related issues and PRs: #3252 

Thanks to @stevesloka for finding and fixing this regression!

# Deprecation & Removal Notices
- Contour no longer supports TLS 1.1.

# Upgrading
Please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

# Are you a Contour user? We would love to know!
If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
