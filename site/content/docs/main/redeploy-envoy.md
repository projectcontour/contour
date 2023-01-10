# Redeploying Envoy

The Envoy process, the data path component of Contour, at times needs to be re-deployed.
This could be due to an upgrade, a change in configuration, or a node-failure forcing a redeployment.

When implementing this roll out, the following steps should be taken:

1. Stop Envoy from accepting new connections
2. Start draining existing connections in Envoy by sending a `POST` request to `/healthcheck/fail` endpoint
3. Wait for connections to drain before allowing Kubernetes to `SIGTERM` the pod

## Overview

Contour implements an `envoy` sub-command named `shutdown-manager` whose job is to manage a single Envoy instances lifecycle for Kubernetes.
The `shutdown-manager` runs as a new container alongside the Envoy container in the same pod.
It uses a  Kubernetes `preStop` event hook to keep the Envoy container running while waiting for connections to drain. The `/shutdown` endpoint blocks until the connections are drained.

```yaml
 - name: shutdown-manager
   command:
   - /bin/contour
   args:
     - envoy
     - shutdown-manager
   image: ghcr.io/projectcontour/contour:main
   imagePullPolicy: Always
   lifecycle:
     preStop:
       exec:
         command:
           - /bin/contour
           - envoy
           - shutdown
```

The Envoy container also has some configuration to implement the shutdown manager.
First the `preStop` hook is configured to use the `/shutdown` endpoint which blocks the Envoy container from exiting.
Finally, the pod's `terminationGracePeriodSeconds` is customized to extend the time in which Kubernetes will allow the pod to be in the `Terminating` state.
The termination grace period defines an upper bound for long-lived sessions.
If during shutdown, the connections aren't drained to the configured amount, the `terminationGracePeriodSeconds` will send a `SIGTERM` to the pod killing it.

![shutdown-manager overview][1]

### Shutdown Manager Config Options

The `shutdown-manager` runs as another container in the Envoy pod.
When the pod is requested to terminate, the `preStop` hook on the `shutdown-manager` executes the `contour envoy shutdown` command initiating the shutdown sequence.

The shutdown manager has a single argument that can be passed to change how it behaves:

| Name | Type | Default | Description |
|------------|------|---------|-------------|
| <nobr>serve-port</nobr> | integer | 8090 | Port to serve the http server on |
| <nobr>ready-file</nobr> | string | /admin/ok | File to poll while waiting shutdown to be completed. |

### Shutdown Config Options

The `shutdown` command does the work of draining connections from Envoy and polling for open connections.

The shutdown command has a few arguments that can be passed to change how it behaves:

| Name | Type | Default | Description |
|------------|------|---------|-------------|
| <nobr>check-interval</nobr> | duration | 5s | Time interval to poll Envoy for open connections. |
| <nobr>check-delay</nobr> | duration | 0s | Time wait before polling Envoy for open connections. |
| <nobr>drain-delay</nobr> | duration | 0s | Time wait before draining Envoy connections. |
| <nobr>min-open-connections</nobr> | integer | 0 | Min number of open connections when polling Envoy. |
| <nobr>admin-port (Deprecated)</nobr> | integer | 9001 | Deprecated: No longer used, Envoy admin interface runs as a unix socket.  |
| <nobr>admin-address</nobr> | string | /admin/admin.sock | Path to Envoy admin unix domain socket. |
| <nobr>ready-file</nobr> | string | /admin/ok | File to write when shutdown is completed. |

  [1]: ../img/shutdownmanager.png
