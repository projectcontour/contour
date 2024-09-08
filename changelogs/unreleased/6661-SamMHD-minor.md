## Disable ExtAuth by default if GlobalExtAuth.AuthPolicy.Disabled is set

Global external authorization or vhost-level authorization is enabled by default unless an AuthPolicy explicitly disables it. By default, `disabled` is set to `GlobalExtAuth.AuthPolicy.Disabled`. This global setting can be overridden by vhost-level AuthPolicy, which can further be overridden by route-specific AuthPolicy. Therefore, the final authorization state is determined by the most specific policy applied at the route level.

## Use AuthPolicy in UpgradeHTTPS

From now on, if external authorization is disabled on any route it will affect redirect-to-HTTPS as well. For example if you disable authorization on some route, Contour will configure Envoy to handle HTTPS Redirection without authorization on that route. (previously if GlobalExtAuth was set, Envoy would check request with ext_auth before redirection which could result in 401 instead of redirection)
