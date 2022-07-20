## Consolidate access logging and TLS cipher suite validation

Access log and TLS cipher suite configuration validation logic is now consolidated in the `apis/projectcontour/v1alpha1` package.
Existing exported elements of the `pkg/config` package are left untouched, though implementation logic now lives in `apis/projectcontour/v1alpha1`.

This should largely be a no-op for users however, as part of this cleanup, a few minor incompatible changes have been made:
- TLS cipher suite list elements will no longer be allowed to have leading or trailing whitespace
- The ContourConfiguration CRD field `spec.envoy.logging.jsonFields` has been renamed to `spec.envoy.logging.accessLogJSONFields`
