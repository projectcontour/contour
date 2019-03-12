# Sticky session support

**Status**: _draft_

This document proposes adding support for sticky session in contour.

## Goals

- Enabling support for sticky-sessions in Contour using
    - Cookie
    - Source IP
    - Header
- Adding support for sticky-sessions in IngressRoute CRD. 

## Non-goals

- Making options in IngressRoute very specific to Envoy.

## Background

Sticky-sessions allow requests to be routed to the same host based on certain parameters of the request.
The parameters can be a cookie, the source IP, a custom header, etc.

#### Sticky sessions in Envoy 
Envoy supports sticky sessions by using a hash-based load balancing strategy (RingHash, Maglev). 
Specifying a **hash policy** for such load-balancers will enable sticky sessions. Hash policy 
is specified per route. **For sticky sessions, each route should only have one service
associated to the route**. This is because if multiple clusters are associated per route (weighted clusters),
requests will be forwarded to the cluster based on weights and stickiness will be 
enforced in that cluster from that point. This defeats the purpose of using sticky sessions since a session will not
be pinned to a pod. 

## High-Level Design
In contour, sticky sessions will be enabled per service. 
If a service has sticky sessions enabled, it should be the **only service**
under that route. Sticky sessions **do not** work with multiple services (weighted clusters).
This means that sticky sessions cannot be used along with blue/green deployments.

### Sticky-sessions configuration
To enable sticky-sessions in a service, the `strategy` needs to be set to *StickySession*.
By default, sticky-session will be based on **source-ip**. `stickySessionConfig` can 
be provided in the service to specify the parameter for stickiness.

#### Cookie-based stickiness
This allows a request to be routed based on a cookie in the http request.
The following parameters can be specified under the strategy options for cookie-based stickiness:
- **name**: The name of the cookie to look for to determine stickiness.
- **path**: The path for the cookie.
```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata: 
  name: basic
  namespace: default
spec: 
  virtualhost:
    fqdn: foo-basic.bar.com
  routes: 
    - match: /       
      services: 
        - name: s1
          port: 80  
          strategy: StickySession
          stickySessionConfig:
            cookie:
              name: name
              path: cookie-path
```                

#### Header-based stickiness
This allows a request to be routed based on a header in the http request.
The following parameters can be specified under the strategy options for header-based stickiness:
- **name**: The name of the header to look for to determine stickiness.
```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata: 
  name: basic
  namespace: default
spec: 
  virtualhost:
    fqdn: foo-basic.bar.com
  routes: 
    - match: /       
      services: 
        - name: s1
          port: 80  
          strategy: StickySession
          stickySessionConfig:
            header:
              name: header-name
```  

#### Connection-based stickiness
This allows a request to be routed based on a connection property of the incoming request. Currently supported
property is the source-IP of the incoming request.
The following parameters can be specified under the strategy options for connection-based stickiness:
- **property**: The property of the connection to be used to determine stickiness. Currently supported property:   
    - IP
```yaml
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata: 
  name: basic
  namespace: default
spec: 
  virtualhost:
    fqdn: foo-basic.bar.com
  routes: 
    - match: /       
      services: 
        - name: s1
          port: 80  
          strategy: StickySession
          stickySessionConfig:
            connection:
              property: IP
```  

## Detailed Design

Changes will be made to the ingressRoute CRD, the DAG, and envoy-specific internal code.

### IngressRoute

- deployment/common/common.yaml will be modified to include change in the ingressroute CRD.
```yaml
spec:
  group: contour.heptio.com
  version: v1beta1
  scope: Namespaced
  names:
    plural: ingressroutes
    kind: IngressRoute
  additionalPrinterColumns: ...
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            ...
            routes:
              type: array
              items:
                ...
                properties:
                  ...
                  services:
                    type: array
                    items:
                      type: object
                      ...
                      properties:
                        ...
                        strategy:
                          type: string
                          enum:
                            - RoundRobin
                            - WeightedLeastRequest
                            - Random
                            - RingHash
                            - Maglev
                            - StickySession
                        stickySessionConfig:
                          type: object
                          properties:
                            cookie:
                              type: object
                              properties:
                                name:
                                  type: string
                                path:
                                  type: string
                            header:
                              type: object
                              properties:
                                name:
                                  type: string
                            connection:
                              type: object
                              properties:
                                property:
                                  type: string
                                  enum:
                                  - IP

```
- The following changes will be made in **apis/contour/v1beta/ingressroute.go**:   
```go
type Service struct {
	...
	// LB Algorithm to apply (see https://github.com/heptio/contour/blob/master/design/ingressroute-design.md#load-balancing)
	Strategy string `json:"strategy,omitempty"`
	// Sticky session config
	Config *StickySessionConfig `json:"stickySessionConfig,omitempty"`
}

type StickySessionConfig struct {
	// Cookie config for sticky session
	CookieCfg *StickySessionCookieConfig `json:"cookie,omitempty"`
	// Header config for sticky session
	HeaderCfg *StickySessionHeaderConfig `json:"header,omitempty"`
	// Connection config for sticky session
	ConnectionCfg *StickySessionConnectionConfig `json:"connection,omitempty"`
}

type StickySessionCookieConfig struct {
	// The name of the cookie
	Name string `json:"name"`
	// The path for the cookie
	Path string `json:"path"`
}

type StickySessionHeaderConfig struct {
	// The name of the header
	Name string `json:"name"`
}

type StickySessionConnectionConfig struct {
	// The property of the connection to be used
	Property string `json:"property"`
}
```

### DAG

- The `TCPService` structure in **internal/dag/dag.go** will be changed to add the sticky session option.
```go
type TCPService struct {
	...
	LoadBalancerStrategy string

	// Sticky Sessions config
	stickySessionCfg *stickySessionConfig
	...
}

type stickySessionParam int8

const (
	stickySessionCookie     stickySessionParam = 1
	stickySessionHeader     stickySessionParam = 2
	stickySessionConnection stickySessionParam = 3
)

type stickySessionConfig struct {
	// The name of the parameter to be used for sticky sessions: cookie/header/connection
	param stickySessionParam
	
	// The options
	options map[string]string
}
```

- The following functions will be changed in **internal/dag/builder.go** to have sticky session config as a parameter:
```go
func (b *builder) lookupHTTPService(m meta, port intstr.IntOrString, weight int, strategy string, stickySessionCfg *ingressroutev1.StickySessionConfig, hc *ingressroutev1.HealthCheck) *HTTPService {
	...
}

func (b *builder) lookupService(m meta, port intstr.IntOrString, weight int, strategy string, stickySessionCfg *ingressroutev1.StickySessionConfig, hc *ingressroutev1.HealthCheck) Service {
	...
}

func (b *builder) addHTTPService(svc *v1.Service, port *v1.ServicePort, weight int, strategy string, stickySessionCfg *ingressroutev1.StickySessionConfig, hc *ingressroutev1.HealthCheck) *HTTPService {
	...
	s := &HTTPService{
		TCPService: TCPService{
			Name:                 svc.Name,
			Namespace:            svc.Namespace,
			ServicePort:          port,
			Weight:               weight,
			LoadBalancerStrategy: strategy,
			LoadBalancerStrategyOptions: toStickySessionConfig(stickySessionCfg), // Converts ingressroutev1.StickySessionConfig to stickySessionConfig defined in dag.go

			MaxConnections:     parseAnnotation(svc.Annotations, annotationMaxConnections),
			MaxPendingRequests: parseAnnotation(svc.Annotations, annotationMaxPendingRequests),
			MaxRequests:        parseAnnotation(svc.Annotations, annotationMaxRequests),
			MaxRetries:         parseAnnotation(svc.Annotations, annotationMaxRetries),
			HealthCheck:        hc,
		},
		Protocol: protocol,
	}
	b.services[s.toMeta()] = s
	return s
}
```

### Envoy-specific code
When sticky session is used for a particular service, the route associated to the service will have a hash policy
defined according to the sticky session configuration.

- The following changes are required in *internal/envoy/route.go*:
```go
func RouteRoute(r *dag.Route, services []*dag.HTTPService) *route.Route_Route {
	ra := route.RouteAction{
		RetryPolicy:   retryPolicy(r),
		Timeout:       timeout(r),
		PrefixRewrite: r.PrefixRewrite,
	}

	...
	switch len(services) {
	case 1:
		if services[0].TCPService.LoadBalancerStrategy == "StickySession" {
			// Set Hash policy in ra
			...
		}
        ...
	}
	return &route.Route_Route{
		Route: &ra,
	}
}
``` 
- *internal/envoy/cluster.go*: lbPolicy will return RingHash when StickySession strategy is used. 
 ```go
func lbPolicy(strategy string) v2.Cluster_LbPolicy {
	switch strategy {
	case "WeightedLeastRequest":
		return v2.Cluster_LEAST_REQUEST
	case "RingHash":
		return v2.Cluster_RING_HASH
	case "StickySession":
		return v2.Cluster_RING_HASH
	case "Maglev":
		return v2.Cluster_MAGLEV
	case "Random":
		return v2.Cluster_RANDOM
	default:
		return v2.Cluster_ROUND_ROBIN
	}
}
```

## Alternatives Considered

- Setting sticky-session configuration at the route level instead of service level. Though this can be directly
translated to envoy configuration, it needs more configuration to be done at user's end. This is because even though
sticky session configuration is declared at the route level, we still need to specify an appropriate load-balancing strategy
that can work with a hash policy at the service level. This introduces room for configuration error on the user's end. Moreover,
sticky session with [multiple services](#sticky-sessions-in-envoy) under a route does not make sense. Declaring sticky session config 
at service level solves these issues.