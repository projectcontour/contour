Two new listener settings expose Envoy path transformations that Contour previously hardcoded:

- `disableNormalizePath` turns off RFC 3986 path normalization. Use this only if your backends treat request paths as opaque identifiers, such as object storage gateways, artifact registries, or APIs whose request signatures cover the raw path.
- `pathWithEscapedSlashesAction` controls how requests containing escaped slash sequences (`%2F`, `%2f`, `%5C`, `%5c`) are handled. Valid values are `keep_unchanged` (default), `reject_request`, `unescape_and_redirect` and `unescape_and_forward`. Setting this to `reject_request` or `unescape_and_redirect` closes an authorization bypass vector against prefix-matching routes and external authorization policies.

Both default to Contour's existing behavior, so upgrades are a no-op.

```yaml
envoy:
  listener:
    disableNormalizePath: false
    pathWithEscapedSlashesAction: keep_unchanged
```

These settings are also available as `disableNormalizePath` and `pathWithEscapedSlashesAction` in the deprecated ConfigMap configuration.

Because these transformations run before route matching, route match conditions and external authorization policies always see the transformed path. See the [path transformation documentation](https://projectcontour.io/docs/main/configuration/#path-transformation) for how the settings interact and for the security implications of loosening them.
