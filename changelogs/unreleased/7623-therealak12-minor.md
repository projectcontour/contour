## Contour now supports per-route authorization overrides

With this change, Contour supports overriding the external authorization
configuration on an individual HTTPProxy route, allowing different routes
within the same virtual host to use different authorization service
backends. This includes a separate ExtensionService reference, HTTP or gRPC
service type, per-route timeout, header allow-lists, and request-body
buffering. The existing `route.authPolicy` field is deprecated in favor of
the new `authzOverride` field.
