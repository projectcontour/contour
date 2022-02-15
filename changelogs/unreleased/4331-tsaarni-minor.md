## Configurable access log level

The verbosity of HTTP and HTTPS access logs can now be configured to one of: `info` (default), `error`, `disabled`.
The verbosity level is set with `accesslog-level` field in the [configuration file](https://projectcontour.io/docs/main/configuration/#configuration-file) or `spec.envoy.logging.accessLogLevel` field in [`ContourConfiguration`](https://projectcontour.io/docs/main/config/api/).
