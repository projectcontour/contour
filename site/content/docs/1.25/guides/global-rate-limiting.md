---
title: Global Rate Limiting
---

Starting in version 1.13, Contour supports [Envoy global rate limiting][1].
In global rate limiting, Envoy communicates with an external Rate Limit Service (RLS) over gRPC to make rate limit decisions for each request.
Envoy is configured to produce 1+ descriptors for incoming requests, containing things like the client IP, header values, and more.
Envoy sends descriptors to the RLS, and the RLS returns a rate limiting decision to Envoy based on the descriptors and the RLS's configured rate limits.

In this guide, we'll walk through deploying an RLS, configuring it in Contour, and configuring an `HTTPProxy` to use it for rate limiting.

**NOTE: you should not consider the RLS deployment in this guide to be production-ready.**
The instructions and example YAML below are intended to be a demonstration of functionality only.
Each user will have their own unique production requirements for their RLS deployment.

## Prerequisites

This guide assumes that you have:

- A local KinD cluster created using [the Contour guide][2].
- Contour installed and running in the cluster using the [quick start][3].

## Deploy an RLS

For this guide, we'll deploy the [Envoy rate limit service][4] as our RLS.
Per the project's README:

> The rate limit service is a Go/gRPC service designed to enable generic rate limit scenarios from different types of applications.
> Applications request a rate limit decision based on a domain and a set of descriptors.
> The service reads the configuration from disk via [runtime][10], composes a cache key, and talks to the Redis cache.
> A decision is then returned to the caller.

However, any service that implements the [RateLimitService gRPC interface][5] is supported by Contour/Envoy.

Create a config map with [the ratelimit service configuration][6]:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ratelimit-config
  namespace: projectcontour
data:
  ratelimit-config.yaml: |
    domain: contour
    descriptors:
      
      # requests with a descriptor of ["generic_key": "foo"]
      # are limited to one per minute.
      - key: generic_key
        value: foo
        rate_limit:
          unit: minute
          requests_per_unit: 1
      
      # each unique remote address (i.e. client IP)
      # is limited to three requests per minute.
      - key: remote_address
        rate_limit:
          unit: minute
          requests_per_unit: 3
```

Create a deployment for the RLS that mounts the config map as a volume.
**This configuration is for demonstration purposes only and is not a production-ready deployment.**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: ratelimit
  name: ratelimit
  namespace: projectcontour
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ratelimit
  template:
    metadata:
      labels:
        app: ratelimit
    spec:
      containers:
        - name: redis
          image: redis:alpine
          env:
            - name: REDIS_SOCKET_TYPE
              value: tcp
            - name: REDIS_URL
              value: redis:6379
        - name: ratelimit
          image: docker.io/envoyproxy/ratelimit:6f5de117
          ports:
            - containerPort: 8080
              name: http
              protocol: TCP
            - containerPort: 8081
              name: grpc
              protocol: TCP
          volumeMounts:
            - name: ratelimit-config
              mountPath: /data/ratelimit/config
              readOnly: true
          env:
            - name: USE_STATSD
              value: "false"
            - name: LOG_LEVEL
              value: debug
            - name: REDIS_SOCKET_TYPE
              value: tcp
            - name: REDIS_URL
              value: localhost:6379
            - name: RUNTIME_ROOT
              value: /data
            - name: RUNTIME_SUBDIRECTORY
              value: ratelimit
            - name: RUNTIME_WATCH_ROOT
              value: "false"
            # need to set RUNTIME_IGNOREDOTFILES to true to avoid issues with
            # how Kubernetes mounts configmaps into pods.
            - name: RUNTIME_IGNOREDOTFILES
              value: "true"
          command: ["/bin/ratelimit"]
          livenessProbe:
            httpGet:
              path: /healthcheck
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: ratelimit-config  
          configMap:
            name: ratelimit-config
```

Create a service:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: ratelimit
  namespace: projectcontour
spec:
  ports:
  - port: 8081
    name: grpc
    protocol: TCP
  selector:
    app: ratelimit
  type: ClusterIP
```

Check the progress of the deployment:

```bash
$ kubectl -n projectcontour get pods -l app=ratelimit 
NAME                         READY   STATUS    RESTARTS   AGE
ratelimit-658f4b8f6b-2hnrf   2/2     Running   0          12s
```

Once the pod is `Running` with `2/2` containers ready, move onto the next step.

## Configure the RLS with Contour

Create a Contour extension service for the RLS:

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ExtensionService
metadata:
  namespace: projectcontour
  name: ratelimit
spec:
  protocol: h2c
  # The service name and port correspond to
  # the service we created in the previous
  # step.
  services:
    - name: ratelimit
      port: 8081
  timeoutPolicy:
    response: 100ms  
```

Update the Contour config map to have the following RLS configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    rateLimitService:
      # extensionService is the <namespace>/<name>
      # of the ExtensionService we created in the
      # previous step.
      extensionService: projectcontour/ratelimit
      # domain corresponds to the domain in the
      # projectcontour/ratelimit-config config map.
      domain: contour
      # failOpen is whether to allow requests through
      # if there's an error connecting to the RLS.
      failOpen: false
```

Restart Contour to pick up the new config map:

```bash
$ kubectl -n projectcontour rollout restart deploy/contour
deployment.apps/contour restarted
```

## Deploy a sample app

To demonstrate how to use global rate limiting in a `HTTPProxy` resource, we first need to deploy a simple echo application:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ingress-conformance-echo
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: ingress-conformance-echo
  template:
    metadata:
      labels:
        app.kubernetes.io/name: ingress-conformance-echo
    spec:
      containers:
      - name: conformance-echo
        image: agervais/ingress-conformance-echo:latest
        ports:
        - name: http-api
          containerPort: 3000
        readinessProbe:
          httpGet:
            path: /health
            port: 3000
---
apiVersion: v1
kind: Service
metadata:
  name: ingress-conformance-echo
spec:
  ports:
  - name: http
    port: 80
    targetPort: http-api
  selector:
    app.kubernetes.io/name: ingress-conformance-echo
```

This echo server will respond with a JSON object that reports information about the HTTP request it received, including the request headers.

Once the application is running, we can expose it to Contour with a `HTTPProxy` resource:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
  - conditions:
    - prefix: /
    services:
    - name: ingress-conformance-echo
      port: 80
  - conditions:
    - prefix: /foo
    services:
    - name: ingress-conformance-echo
      port: 80
```

We can verify that the application is working by requesting any path:

```bash
$ curl -k http://local.projectcontour.io/test/$((RANDOM))
{"TestId":"","Path":"/test/22808","Host":"local.projectcontour.io","Method":"GET","Proto":"HTTP/1.1","Headers":{"Accept":["*/*"],"Content-Length":["0"],"User-Agent":["curl/7.75.0"],"X-Envoy-Expected-Rq-Timeout-Ms":["15000"],"X-Envoy-Internal":["true"],"X-Forwarded-For":["172.18.0.1"],"X-Forwarded-Proto":["http"],"X-Request-Id":["8ecb85e1-271b-44b4-9cf0-4859cbaed7a7"],"X-Request-Start":["t=1612903866.309"]}}
```

## Add global rate limit policies

Now that we have a working application exposed by a `HTTPProxy` resource, we can add add global rate limiting to it.

Edit the `HTTPProxy` that we created in the previous step to add rate limit policies to both routes:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
  - conditions:
    - prefix: /
    services:
    - name: ingress-conformance-echo
      port: 80
    rateLimitPolicy:
      global:
        descriptors:
          - entries:
              - remoteAddress: {}
  - conditions:
    - prefix: /foo
    services:
    - name: ingress-conformance-echo
      port: 80
    rateLimitPolicy:
      global:
        descriptors:
          - entries:
              - remoteAddress: {}
          - entries:
              - genericKey:
                  value: foo
```

## Make requests

Before making requests to our `HTTPProxy`, let's quickly revisit the `ratelimit-config` config map.
Here's what we defined:

```yaml
...
descriptors:
  # requests with a descriptor of ["generic_key": "foo"]
  # are limited to one per minute.
  - key: generic_key
    value: foo
    rate_limit:
      unit: minute
      requests_per_unit: 1
  
  # each unique remote address (i.e. client IP)
  # is limited to three total requests per minute.
  - key: remote_address
    rate_limit:
      unit: minute
      requests_per_unit: 3
```

The first entry says that requests with a descriptor of `["generic_key": "foo"]` should be limited to one per minute.
The second entry says that each unique remote address (client IP) should be allowed three total requests per minute.
All relevant rate limits are applied for each request, and requests that result in a `429 (Too Many Requests)` count against limits.

So, we should be able to make:
- a first request to `local.projectcontour.io/foo` that get a `200 (OK)` response
- a second request to `local.projectcontour.io/foo` that gets a `429 (Too Many Requests)` response (due to the first rate limit)
- a third request to `local.projectcontour.io/bar`that gets a `200 (OK)` response
- a fourth request to `local.projectcontour.io/bar`that gets a `429 (Too Many Requests)` response (due to the second rate limit)

Let's try it out (remember, you'll need to make all of these requests within 60 seconds since the rate limits are per minute):

Request #1:
```
$ curl -I local.projectcontour.io/foo

HTTP/1.1 200 OK
content-type: application/json
date: Mon, 08 Feb 2021 22:25:06 GMT
content-length: 403
x-envoy-upstream-service-time: 4
vary: Accept-Encoding
server: envoy
```

Request #2:

```
$ curl -I local.projectcontour.io/foo

HTTP/1.1 429 Too Many Requests
x-envoy-ratelimited: true
date: Mon, 08 Feb 2021 22:59:10 GMT
server: envoy
transfer-encoding: chunked
```

Request #3:

```
$ curl -I local.projectcontour.io/bar

HTTP/1.1 200 OK
content-type: application/json
date: Mon, 08 Feb 2021 22:59:54 GMT
content-length: 404
x-envoy-upstream-service-time: 2
vary: Accept-Encoding
server: envoy
```

Request #4:

```
$ curl -I local.projectcontour.io/bar

HTTP/1.1 429 Too Many Requests
x-envoy-ratelimited: true
date: Mon, 08 Feb 2021 23:00:28 GMT
server: envoy
transfer-encoding: chunked
```

## Wrapping up

For more information, see the [Contour rate limiting documentation][7] and the [API reference documentation][8].

The YAML used in this guide is available [in the Contour repository][9].

[1]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/other_features/global_rate_limiting
[2]: ../deploy-options/#kind
[3]: https://projectcontour.io/getting-started/#option-1-quickstart
[4]: https://github.com/envoyproxy/ratelimit
[5]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/ratelimit/v3/rls.proto
[6]: https://github.com/envoyproxy/ratelimit#configuration
[7]: ../config/rate-limiting/
[8]: ../config/api/
[9]: {{< param github_url>}}/tree/main/examples/ratelimit
[10]: https://github.com/lyft/goruntime
