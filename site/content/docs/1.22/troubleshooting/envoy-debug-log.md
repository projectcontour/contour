# Enabling Envoy Debug Logging

The `envoy` command has a `--log-level` [flag][1] that can be useful for debugging.
By default, it's set to `info`.
To change it to `debug`, edit the `envoy` DaemonSet in the `projectcontour` namespace and replace the `--log-level info` flag with `--log-level debug`.
Setting the Envoy log level to `debug` can be particilarly useful for debugging TLS connection failures.

[1]: https://www.envoyproxy.io/docs/envoy/latest/operations/cli
