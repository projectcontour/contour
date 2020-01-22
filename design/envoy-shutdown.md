# Envoy Shutdown Probes

Status: _Accepted_

This proposal describes how to add better support for identifying the status of Envoy connections during a restart or redeploy of Envoy.

## Goals

- Provide a mechanism to provide feedback on open connections & load inside an Envoy process
- Allow for a Envoy fleet roll-out minimizing connection loss

## Non Goals

- Guarantee zero connection loss during a roll-out

## Background

The Envoy process, the data path component of Contour, at times needs to be re-deployed.
This could be due to an upgrade, a change in configuration, or a node-failure forcing a redeployment.

Contour currently implements a `preStop` hook in the container which signals to Envoy to begin draining connections.
Once Envoy begins draining, the readiness probe on the pod triggers to fail, which in turn causes that instance of Envoy to stop accepting new connections.

The main problem is that the `preStop` hook (which sends the `/healthcheck/fail` request) does not wait until Envoy has drained all connections, so when the container
is restarted, users can receive errors.

This design looks to add a new component to Contour which will allow for a way to understand if open connections exist in Envoy before sending a `SIGTERM`.

## High-Level Design

Implement a new sub-command to Contour named `envoy shutdown-manager` which will handle sending the healthcheck fail request 
to Envoy and then begin polling the http/https listeners for active connections from the `/stats` endpoint available on 
`localhost:9001`.

Additionally, an optional `min-open-connections` parameter will be added which will allow users to define the minimum number of
open connections that can be open when waiting for connections to drain.

A `preStop` hook (https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/) in Kubernetes allows time for a container to do any cleanup or extra processing before getting sent a SIGTERM.
This lifecycle hook blocks the process during this cleanup time and then returns when its ready to be shutdown.

## Detailed Design

- Implement new sub-command in Contour called `envoy shutdown-manager`
- This command will expose an http endpoint over a specific port (e.g. `8090`) and path `/shutdown`. 
- A new container will be added to the Envoy Daemonset pod which will run this new sub-command
- This new container as well as the Envoy container will be updated to use an http preStop hook (see example below)
- When the pod gets a request to shutdown, the `preStop` hook will send a request to `localhost:8090/shutdown` which will tell Envoy to begin draining connections as well as start polling for active connections and will block until that reaches zero or the `min-open-connections` is met
- The `terminationGracePeriodSeconds` in the pod will be extended to a larger value (from the default 30 seconds) to allow time to drain connections. If this time limit is met, Kubernetes will SIGTERM the pod and kill it
- A second endpoint, `/healthz`, will be made available to check for the health of this container which will be implemented in a Kubernetes liveness probe in the event the shutdown-manager exits

```yaml
apiVersion: extensions/v1beta1
kind: DaemonSet
metadata:
  annotations:
  labels:
    app: envoy
  name: envoy
  namespace: projectcontour
spec:
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: envoy
  template:
    metadata:
      annotations:
        prometheus.io/path: /stats/prometheus
        prometheus.io/port: "8002"
        prometheus.io/scrape: "true"
      creationTimestamp: null
      labels:
        app: envoy
    spec:
      automountServiceAccountToken: false
      containers:
      - command:                          # <----- New Pod
        - /bin/shutdown-manager          
        image: stevesloka/envoyshutdown
        imagePullPolicy: Always
        lifecycle:
          preStop:                       # <----- PreStop Hook
            httpGet:
              path: /shutdown
              port: 8090
              scheme: HTTP
         livenessProbe:                  # <------ Liveness probe  
           httpGet:
             path: /healthz
             port: 8090
           initialDelaySeconds: 3
           periodSeconds: 10
        name: shutdown-manager
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
      - args:
        - -c
        - /config/envoy.json
        - --service-cluster $(CONTOUR_NAMESPACE)
        - --service-node $(ENVOY_POD_NAME)
        - --log-level info
        command:
        - envoy
        env:
        - name: CONTOUR_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        - name: ENVOY_POD_NAME
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.name
        image: docker.io/envoyproxy/envoy:v1.13.0
        imagePullPolicy: IfNotPresent
        lifecycle:                                # <----- PreStop Hook
          preStop:
            httpGet:
              path: /shutdown
              port: 8090
              scheme: HTTP
        name: envoy
        ports:
        - containerPort: 80
          hostPort: 80
          name: http
          protocol: TCP
        - containerPort: 443
          hostPort: 443
          name: https
          protocol: TCP
        readinessProbe:
          failureThreshold: 4
          httpGet:
            path: /ready
            port: 8002
            scheme: HTTP
          initialDelaySeconds: 3
          periodSeconds: 3
          successThreshold: 1
          timeoutSeconds: 1
        volumeMounts:
        - mountPath: /config
          name: envoy-config
        - mountPath: /certs
          name: envoycert
        - mountPath: /ca
          name: cacert
      dnsPolicy: ClusterFirst
      initContainers:
      - args:
        - bootstrap
        - /config/envoy.json
        - --xds-address=contour
        - --xds-port=8001
        - --envoy-cafile=/ca/cacert.pem
        - --envoy-cert-file=/certs/tls.crt
        - --envoy-key-file=/certs/tls.key
        command:
        - contour
        env:
        - name: CONTOUR_NAMESPACE
          valueFrom:
            fieldRef:
              apiVersion: v1
              fieldPath: metadata.namespace
        image: docker.io/projectcontour/contour:master
        imagePullPolicy: Always
        name: envoy-initconfig
        resources: {}
        terminationMessagePath: /dev/termination-log
        terminationMessagePolicy: File
        volumeMounts:
        - mountPath: /config
          name: envoy-config
        - mountPath: /certs
          name: envoycert
          readOnly: true
        - mountPath: /ca
          name: cacert
          readOnly: true
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 300
      volumes:
      - emptyDir: {}
        name: envoy-config
      - name: envoycert
        secret:
          defaultMode: 420
          secretName: envoycert
      - name: cacert
        secret:
          defaultMode: 420
          secretName: cacert
  updateStrategy:
    rollingUpdate:
      maxUnavailable: 10%
    type: RollingUpdate
```

## Alternatives Considered

- Bash scripts could be used to do this, however it's more difficult to implement and hard to test. 
- Instead of using the http preStop hook, we could call a binary to do the checks, however, it's difficult to get this into the Envoy container reliably (we'd have to use some shared volume)

## Security Considerations

The only possible issue that could arise is something goes wrong in the shutdown logic and either the pod terminates before all active connections are drained, or it never quits which
would then rely on the pod termination grace period to terminate the pod.
