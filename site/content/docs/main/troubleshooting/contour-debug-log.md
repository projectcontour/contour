# Enabling Contour Debug Logging

The `contour serve` subcommand has two command-line flags that can be helpful for debugging.
The `--debug` flag enables general Contour debug logging, which logs more information about how Contour is processing API resources.
The `--kubernetes-debug` flag enables verbose logging in the Kubernetes client API, which can help debug interactions between Contour and the Kubernetes API server.
This flag requires an integer log level argument, where higher number indicates more detailed logging.

## Changing the log level at runtime

Restarting Contour with `--debug` is not always an option, for example when chasing a rare issue in production.
Contour's debug HTTP server (`127.0.0.1:6060` by default, see the `--debug-http-address` and `--debug-http-port` flags) exposes a `/debug/loglevel` endpoint to read and change the Contour log level of a running instance.
A `GET` returns the current level; a `PUT` (or `POST`) with the desired level as the `level` query parameter or as the request body sets it.
Valid levels are `panic`, `fatal`, `error`, `warning`, `info`, `debug` and `trace`; an unknown level is rejected with `400 Bad Request` and leaves the level unchanged.

```bash
# Get one of the pods that matches the Contour deployment
$ CONTOUR_POD=$(kubectl -n projectcontour get pod -l app=contour -o name | head -1)
# Do the port forward to that pod
$ kubectl -n projectcontour port-forward $CONTOUR_POD 6060
# Show the current log level
$ curl localhost:6060/debug/loglevel
info
# Enable debug logging while investigating
$ curl -X PUT -d debug localhost:6060/debug/loglevel
debug
# Return to the default level afterwards so logs do not fill up
$ curl -X PUT -d info localhost:6060/debug/loglevel
info
```

The change only affects the Contour instance the request is sent to and is lost when the pod restarts.
The `--kubernetes-debug` level of the Kubernetes client is not affected.
