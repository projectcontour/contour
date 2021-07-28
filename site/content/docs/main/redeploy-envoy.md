# Redeploying Envoy

The Envoy process, the data path component of Contour, at times needs to be re-deployed.
This could be due to an upgrade, a change in configuration, or a node-failure forcing a redeployment.

When implementing this roll out, the following steps should be taken: 

1. Stop Envoy from accepting new connections 
2. Start draining existing connections in Envoy by sending a `POST` request to `/healthcheck/fail` endpoint
3. Wait for connections to drain before allowing Kubernetes to `SIGTERM` the pod

## Overview

Contour implements a new `envoy` sub-command which has a `shutdown-manager` whose job is to manage a single Envoy instances lifecycle for Kubernetes.
The `shutdown-manager` runs as a new container alongside the Envoy container in the same pod.
It exposes two HTTP endpoints which are used for `livenessProbe` as well as to handle the Kubernetes `preStop` event hook.

- **livenessProbe**: This is used to validate the shutdown manager is still running properly. If requests to `/healthz` fail, the container will be restarted.
- **preStop**: This is used to keep the container running while waiting for Envoy to drain connections. The `/shutdown` endpoint blocks until the connections are drained.

```yaml
 - name: shutdown-manager
   command:
   - /bin/contour
   args:
     - envoy
     - shutdown-manager
   image: docker.io/projectcontour/contour:main
   imagePullPolicy: Always
   lifecycle:
     preStop:
       exec:
         command:
           - /bin/contour
           - envoy
           - shutdown
   livenessProbe:
     httpGet:
       path: /healthz
       port: 8090
     initialDelaySeconds: 3
     periodSeconds: 10  
```

The Envoy container also has some configuration to implement the shutdown manager.
First the `preStop` hook is configured to use the `/shutdown` endpoint which blocks the Envoy container from exiting.
Finally, the pod's `terminationGracePeriodSeconds` is customized to extend the time in which Kubernetes will allow the pod to be in the `Terminating` state.
The termination grace period defines an upper bound for long-lived sessions.
If during shutdown, the connections aren't drained to the configured amount, the `terminationGracePeriodSeconds` will send a `SIGTERM` to the pod killing it.

![shutdown-manager overview][1]

### Shutdown Manager Config Options

The shutdown manager has a set of arguments that can be passed to change how it behaves:

| Field Name | Type | Default | Description |
|------------|------|---------|-------------|
| --serve-port | integer | `8090` | Port to serve the http server on. |

### Shutdown Manager PreStop Options

The shutdown manager "shutdown" command has a set of arguments that can be passed to change how it behaves.
This command is used via the pod's lifecycle.preStop.exec.command when the actual shutdown sequence begins:

| Field Name | Type | Default | Description |
|------------|------|---------|-------------|
| --admin-port | integer | `9001` | Envoy admin interface port |
| --admin-address | ip address | `127.0.0.1` | Envoy admin interface address. |
| --check-interval | duration | `5s` | Interval of time to wait between polls for open connections. |
| --check-delay | duration | `60s` | Time wait before polling Envoy for open connections. |
| --drain-delay | duration | `0s` | Time wait before draining Envoy connections. |
| --min-open-connections | integer | `0` | Min number of open connections when polling Envoy. |

[1]: ../img/shutdownmanager.png