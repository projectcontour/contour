Two new listener settings expose Envoy path transformations that Contour previously hardcoded:

- `disableNormalizePath` turns off RFC 3986 path normalization. Use this only if the backend requires paths that Envoy would otherwise normalize to remain unchanged, for example, paths containing `.` or `..` segments. Do not rely on path prefixes for access control if the backend interprets those paths differently.
- `pathWithEscapedSlashesAction` controls how requests containing escaped slash sequences (`%2F`, `%2f`, `%5C`, `%5c`) are handled. Valid values are `keep_unchanged` (default), `reject_request`, `unescape_and_redirect` and `unescape_and_forward`. Setting this to `reject_request` or `unescape_and_redirect` closes an authorization bypass vector against prefix-matching routes and external authorization policies.

Both default to Contour's existing behavior for standard configurations. Explicitly setting the escaped-slash action means Envoy no longer uses the `http_connection_manager.path_with_escaped_slashes_action` runtime override. Installations using that override with a custom bootstrap should configure the equivalent action through `pathWithEscapedSlashesAction`.

```yaml
envoy:
  listener:
    disableNormalizePath: false
    pathWithEscapedSlashesAction: keep_unchanged
```

These settings are also available as `disableNormalizePath` and `pathWithEscapedSlashesAction` in the deprecated ConfigMap configuration.

Because these transformations run before route matching, route match conditions and external authorization policies always see the transformed path. See the [path transformation documentation](https://projectcontour.io/docs/main/configuration/#path-transformation) for how the settings interact and for the security implications of loosening them.
