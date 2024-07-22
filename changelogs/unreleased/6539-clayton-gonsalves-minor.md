## Add Circuit Breaker support for Extension Services

This change enabled the user to configure the Circuit breakers for extension services either via the global Contour config or on the Extension Service CRD itself on a per Extension Service itself.

**NOTE**: The `PerHostMaxConnections` is now also configurable via the global settings.

