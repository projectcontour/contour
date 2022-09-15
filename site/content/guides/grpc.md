---
title: Configuring ingress to gRPC services with Contour
layout: page
---

## Example gRPC Service

The below examples use the [gRPC server](https://github.com/projectcontour/yages) used in Contour end to end tests.
The server implements a service `yages.Echo` with two methods `Ping` and `Reverse`.
It also implements the [gRPC health checking service](https://pkg.go.dev/google.golang.org/grpc/health/grpc_health_v1) (see [here](https://github.com/grpc/grpc/blob/master/doc/health-checking.md) for more details) and is bundled with the [gRPC health probe](https://github.com/grpc-ecosystem/grpc-health-probe).

An example base deployment and service for a gRPC server utilizing plaintext HTTP/2 are provided here:

```yaml
---
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/name: grpc-echo
  name: grpc-echo
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: grpc-echo
  template:
    metadata:
      labels:
        app.kubernetes.io/name: grpc-echo
    spec:
      containers:
      - name: grpc-echo
        image: ghcr.io/projectcontour/yages:v0.1.0
        ports:
        - name: grpc
          containerPort: 9000
        readinessProbe:
          exec:
            command: ["/grpc-health-probe", "-addr=localhost:9000"]
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.kubernetes.io/name: grpc-echo
  name: grpc-echo
spec:
  selector:
    app.kubernetes.io/name: grpc-echo
  ports:
  - port: 9000
    protocol: TCP
    targetPort: grpc
```

## HTTPProxy Configuration

Configuring proxying to a gRPC service with HTTPProxy is as simple as specifying the protocol Envoy uses with the upstream application via the `spec.routes[].services[].protocol` field.
For example, in the resource below, for proxying plaintext gRPC to the `yages` sample app, the protocol is set to `h2c` to denote HTTP/2 over cleartext.
For TLS secured gRPC, the protocol used would be `h2`.

Route path prefix matching can be used to match a specific gRPC message if required.

```yaml
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: my-grpc-service
spec:
  virtualhost:
    fqdn: my-grpc-service.foo.com 
  routes:
  - conditions:
    - prefix: /yages.Echo/Ping # Matches a specific gRPC method.
    services:
    - name: grpc-echo
      port: 9000
      protocol: h2c
  - conditions: 
    - prefix: / # Matches everything else.
    services:
    - name: grpc-echo
      port: 9000
      protocol: h2c
```

Using the sample deployment above along with this HTTPProxy example, you can test calling this plaintext gRPC server with the following [grpccurl](https://github.com/fullstorydev/grpcurl) command:

```
grpcurl -plaintext -authority=my-grpc-service.foo.com <load balancer IP and port if needed> yages.Echo/Ping
```

## Ingress v1 Configuration

To configure routing for gRPC requests with Ingress v1, you must add an annotation on the upstream Service resource as below.

```yaml
---
apiVersion: v1
kind: Service
metadata:
  labels:
    app.kubernetes.io/name: grpc-echo
  annotations:
    projectcontour.io/upstream-protocol.h2c: "9000"
  name: grpc-echo
spec:
  selector:
    app.kubernetes.io/name: grpc-echo
  ports:
  - port: 9000
    protocol: TCP
    targetPort: grpc
```

The annotation key must follow the form `projectcontour.io/upstream-protocol.{protocol}` where `{protocol}` is `h2c` for plaintext gRPC or `h2` for TLS encrypted gRPC to the upstream application.
The annotation value contains a comma-separated list of port names and/or numbers that must match with the ones defined in the Service definition.

Using the Service above with the Ingress resource below should achieve the same configuration as with an HTTPProxy.

```yaml
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-grpc-service
spec:
  rules:
  - host: my-grpc-service.foo.com
    http:
      paths:
      - path: /
        backend:
          service:
            name: grpc-echo
            port:
              number: 9000
        pathType: Prefix
```

## Gateway API Configuration

At the moment, configuring gRPC routes with Gateway API resources is achieved by the same method as with Ingress v1: annotation to select a protocol and port on a Service referenced by HTTPRoute `spec.rules[].backendRefs`.

Gateway API does include a specific resource [GRPCRoute](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.GRPCRoute) for routing gRPC requests.
This may be supported in future versions of Contour.

## gRPC-Web

Contour configures Envoy to automatically convert [gRPC-Web](https://github.com/grpc/grpc-web) HTTP/1 requests to gRPC over HTTP/2 RPC calls to an upstream service.
This is a convenience addition to make usage of gRPC web application client libraries and the like easier.

Note that you still must provide configuration of the upstream protocol to have gRPC-Web requests converted to gRPC to the upstream app.
If your upstream application does not in fact support gRPC, you may get a protocol error.
In that case, please see [this issue](https://github.com/projectcontour/contour/issues/4290). 

For example, with the example deployment and routing configuration provided above, an example HTTP/1.1 request and response via `curl` looks like:

```
curl \
  -s -v \
  <load balancer IP and port if needed>/yages.Echo/Ping \
  -XPOST \
  -H 'Host: my-grpc-service.foo.com' \
  -H 'Content-Type: application/grpc-web-text' \
  -H 'Accept: application/grpc-web-text' \
  -d'AAAAAAA='
```

This `curl` command sends and receives gRPC messages as base 64 encoded text over HTTP/1.1.
Piping the output to `base64 -d | od -c` we can see the raw gRPC response as text.
