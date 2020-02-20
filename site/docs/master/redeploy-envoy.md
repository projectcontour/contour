# Redeploying Envoy

The Envoy process, the data path component of Contour, at times needs to be re-deployed.
This could be due to an upgrade, a change in configuration, or a node-failure forcing a redeployment.

When implementing this roll out, the following steps should be taken: 

1. Stop Envoy from accepting new connections 
2. Start draining existing connections in Envoy by sending a `POST` request to `/healthcheck/fail` endpoint
3. Wait for connections to drain before allowing Kubernetes to `SIGTERM` the pod

## Overview

Contour implements a new `envoy` sub-command which has a `shutdown-manager` whose job is to manage a single Envoy instances lifecycle for Kubernetes.
The `shutdown-maanger` runs as a new container alongside the Envoy container in the same pod.
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
   image: docker.io/projectcontour/contour:master
   imagePullPolicy: Always
   lifecycle:
     preStop:
       httpGet:
         path: /shutdown
         port: 8090
         scheme: HTTP
   livenessProbe:
     httpGet:
       path: /healthz
       port: 8090
     initialDelaySeconds: 3
     periodSeconds: 10  
```

The Envoy container also has some configuration to implement the shutdown manager.
First the `preStop` hook is configured to use the `/shutdown` endpoint which blocks the container from exiting.
Finally, the pod's `terminationGracePeriodSeconds` is customized to extend the time in which Kubernetes will allow the pod to be in the `Terminating` state.
The termination grace period defines an upper bound for long-lived sessions.
If during shutdown, the connections aren't drained to the configured amount, the `terminationGracePeriodSeconds` will send a `SIGTERM` to the pod killing it.

![shutdown-manager overview][1]{: .center-image }

### Shutdown Manager Config Options

The shutdown manager has a set of arguments that can be passed to change how it behaves:

- **check-interval:** Time to poll Envoy for open connections.
  - Type: duration (Default 5s)
- **check-delay:** Time wait before polling Envoy for open connections.
  - Type: duration (Default 60s)
- **min-open-connections:** Min number of open connections when polling Envoy.
  - Type: integer (Default 0)
- **serve-port:** Port to serve the http server on.
  - Type: integer (Default 8090)

  [1]: ../img/shutdownmanager.png