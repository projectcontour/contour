# Route CRD

## Executive Summary

The current Kubernetes ingress spec does not fully represent a real-world ingress object. There are many details which cannot be described without using annotations. For example, features such as automatic retries, active health checking, circuit breaking, global rate limiting, request shadowing, and zone-local load balancing are items that are missing clear representation in the current specification. 

A way to address these requirements are to utilize Custom Resource Definitions, allowing for clear representation and validation of those objects. A Route CRD will be created which will define the ingress specification and provide the details to route incoming traffic via a host, apply corresponding pathPrefixes and finally define the upstream endpoints who will serve the request. 

The Route CRD defines the Ingress route for a single vhosts routing to one or more upstream services. The Route CRD allows users also define a port to proxy as well as define a health check endpoint. 

## Goals

- Enable ingress specification via Kubernetes Custom Resource Definition
- Provide field level validation on CRD

## Terminology / Spec

Following is an layout of the spec which the CRD will be implemented:

- **apiVersion**: contour.heptio.com/v1alpha1
- **kind**: Route
- **metadata**:
  - **namespace**: Namespace deployed into
  - **name**: Name of object
- **status**: 
  - **currentStatus**: Current Status of the route (e.g. Invalid, Error, etc)
  - **lastProcessTime**: Time the object was processed by Contour and made available to cluster
  - **[]errors**: List of errors found in configuration
- **spec**:
  - **strategy (Optional)**: Load balancer algorithm type: Round Robin / Weighted Least Request / Random (Note: A strategy defined here will be the default for all routes below unless overridden in route spec)
  - **lbHealthCheck (Optional)**: (Note: A strategy defined here will be the default for all routes below unless overridden in route spec)
    - **path**: HTTP endpoint used to perform health checks on upstream service (e.g. /healthz). It expects a 200 response if the host is healthy. The upstream host can return 503 if it wants to immediately notify downstream hosts to no longer forward traffic to it.
    - **intervalSeconds**: The interval (seconds) between health checks. Defaults to 5 seconds if not set.
    - **timeoutSeconds**: The time to wait (seconds) for a health check response. If the timeout is reached the health check attempt will be considered a failure. Defaults to 2 seconds if not set.
    - **unhealthyThresholdCount**: The number of unhealthy health checks required before a host is marked unhealthy. Note that for http health checking if a host responds with 503 this threshold is ignored and the host is considered unhealthy immediately. Defaults to 3 if not defined.
  - **host**: Host name (e.g. heptio.com)
  - **[]routes**: List of route objects
    - **pathPrefix**: Allows for further route definition
    - **strategy (Optional)**: Load balancer algorithm type (Defaults to RoundRobin if not specified): RoundRobin / WeightedLeastRequest / Random (Note: A strategy defined here overrides anything specified above)
    - **[]upstreams**: List of upstreams to proxy traffic
      - **serviceName**: Name of Kubernetes service to proxy traffic. Names defined here will be used to look up corresponding endpoints which contain the ips to route. (Note: Must match service name in same namespace)
      - **servicePort**: Port (defined as Integer) to proxy traffic to since a service can have multiple defined
      - **weight (Optional)**: Percentage of traffic to balance traffic
      - **lbHealthCheck (Optional)**: Note: health checks defined here override any defined above
        - **path**: HTTP endpoint used to perform health checks on upstream service (e.g. /healthz). It expects a 200 response if the host is healthy. The upstream host can return 503 if it wants to immediately notify downstream hosts to no longer forward traffic to it.
        - **intervalSeconds**: The interval (seconds) between health checks. Defaults to 5 seconds if not set.
        - **timeoutSeconds**: The time to wait (seconds) for a health check response. If the timeout is reached the health check attempt will be considered a failure. Defaults to 2 seconds if not set.
        - **unhealthyThresholdCount**: The number of unhealthy health checks required before a host is marked unhealthy. Note that for http health checking if a host responds with 503 this threshold is ignored and the host is considered unhealthy immediately. Defaults to 3 if not defined.
        - **healthyThresholdCount**: The number of healthy health checks required before a host is marked healthy. Note that during startup, only a single successful health check is required to mark a host healthy.

References: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/health_checking.html  and https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/core/health_check.proto.html#envoy-api-msg-core-healthcheck 

## Example CRD

Following sample which defines two routes, one for `heptio.com/` as well as one for `heptio.com/ticker`. The first route targets two services (`www` & `canarywww`). The second route CRD targets the `ticker` service.

```
apiVersion: contour.heptio.com/v1alpha1
kind: Route
metadata: 
  name: finance-route
  namespace: finance
spec: 
  host: heptio.com
  routes: 
    - pathPrefix: /
      upstreams: 
        - lbHealthCheck: 
            intervalSeconds: 1
            path: /status
          serviceName: www
          servicePort: 8080
        - lbHealthCheck: 
            intervalSeconds: 1
            path: /status
          serviceName: canarywww
          servicePort: 8080
    - pathPrefix: /ticker
      upstreams: 
        - serviceName: tickersvc
          servicePort: 80
  strategy: RoundRobin
status: 
  currentStatus: Valid
  errors: []
  lastProcessTime: "date last processed"
```

