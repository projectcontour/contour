## Disable ExtAuth by default if GlobalExtAuth.AuthPolicy.Disabled is set

Global external authorization can now be disabled by default and enabled by overriding the vhost and route level auth policies.
This is achieved by setting the `globalExtAuth.authPolicy.disabled` in the configuration file or `ContourConfiguration` CRD to `true`, and setting the `authPolicy.disabled` to `false` in the vhost and route level auth policies.
The final authorization state is determined by the most specific policy applied at the route level.

## Disable External Authorization in HTTPS Upgrade

When external authorization is enabled, no authorization check will be performed for HTTP to HTTPS redirection.
Previously, external authorization was checked before redirection, which could result in a 401 Unauthorized error instead of a 301 Moved Permanently status code.
