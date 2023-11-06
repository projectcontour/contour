## Backend Protocol Selection

Contour now enables end-users to specify backend protocols by configuring the `ServicePort.AppProtocol` parameter. The accepted values for it are `projectcontour.io/[h2|h2c|tls]` and `kubernetes.io/[h2c|ws]`. It's important to note that the `kubernetes.io/wss` is not supported. If `AppProtocol` is set, any other configurations, such as the annotation: `projectcontour.io/upstream-protocol.{protocol}` will be disregarded.
