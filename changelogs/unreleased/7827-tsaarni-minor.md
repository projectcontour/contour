## Align input validation with Envoy limits

Contour now rejects values that Envoy does not accept. Previously, Contour sent these values to Envoy, and Envoy rejected the configuration. Envoy kept using its previous configuration and ignored all later updates until the value was fixed.

After upgrading, resources with such values are rejected. These values did not work before the upgrade either.

The checks cover `HTTPProxy`, `ExtensionService`, `HTTPRoute`, `GRPCRoute`, `Ingress`, `ContourConfiguration`, `ContourDeployment` and the Contour configuration file, and include:

- NUL, CR or LF in header names, path rewrites, redirects and subject names
- control characters in health check path and host
- header values longer than 16384 bytes
- out of range weights, timeouts and durations, and negative numbers
- regular expressions using \x{...}, \p, \P or octal escapes above \377; use \xHH instead

See [#7827](https://github.com/projectcontour/contour/pull/7827) for the full list of fields.
