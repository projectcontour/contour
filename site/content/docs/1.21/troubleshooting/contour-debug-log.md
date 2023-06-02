# Enabling Contour Debug Logging

The `contour serve` subcommand has two command-line flags that can be helpful for debugging.
The `--debug` flag enables general Contour debug logging, which logs more information about how Contour is processing API resources.
The `--kubernetes-debug` flag enables verbose logging in the Kubernetes client API, which can help debug interactions between Contour and the Kubernetes API server.
This flag requires an integer log level argument, where higher number indicates more detailed logging.
