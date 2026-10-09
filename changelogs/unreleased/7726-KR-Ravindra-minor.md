## Contour log level can be changed at runtime via the debug server

The Contour debug HTTP server (`--debug-http-address`/`--debug-http-port`) now serves a `/debug/loglevel` endpoint. A `GET` returns the current level and a `PUT` or `POST` with a logrus level name (for example `debug` or `info`) changes it for the running process without a restart. The endpoint is only exposed on the debug server, which is bound to localhost by default. Kubernetes client (klog) verbosity is still controlled by `--kubernetes-debug`.
