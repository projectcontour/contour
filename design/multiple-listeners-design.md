# Support for multiple Listeners

Status: Draft

## Abstract
This document proposes changes to Contour in order to support an arbitrary number of Listeners.

## Background
Historically, Contour has always supported configuring two Envoy Listeners: one for HTTP, and one for HTTPS.
Root HTTPProxies without TLS enabled have been configured on the HTTP Listener, and those with TLS enabled have been configured on the HTTPS Listener.
Now that Contour implements Gateway API, it's necessary to support any number of Listeners in order to be fully conformant with the Gateway API spec.

It's important to distinguish between **Envoy Listeners** and **Gateway Listeners**, since they are related but not the same.

An **Envoy Listener** can be thought of simply as Envoy listening on a given port.

A **Gateway Listener** encompasses listening on a certain "exposed" port (typically defined on an Envoy LoadBalancer service), which is mapped to an underlying Envoy Listener, for requests with a given hostname, and optionally with certain TLS details.
In Envoy terminology, this more closely corresponds to the combination of a Listener, a FilterChain, and a RouteConfiguration virtual host.

## Goals
- Support Gateways, and associated HTTPRoutes/TLSRoutes, with Listeners configured on >2 ports.
- Don't break existing functionality for HTTPProxy/Ingress.

## Non Goals
- Support for HTTPProxy/Ingress attaching to additional Listeners.
- Support for new route types, e.g. TCPRoute.

## High-Level Design
Broadly speaking, Contour can operate in one of two modes, which we will call **Gateway mode** and **non-Gateway mode**.
In Gateway mode, Contour is configured to process a Gateway API Gateway, and gets its Listener configuration from there.
Non-Gateway mode is the traditional mode of operation, where Listener configuration comes from CLI flags, a config file, and/or a ContourConfiguration resource, plus the Envoy service YAML, and HTTPProxy and/or Ingress is used for routing configuration.

For non-Gateway mode, this design should be fully backwards-compatible.
There should be no changes observed by a user of Contour in non-Gateway mode.

For Gateway mode, the following changes are needed:

- The Gateway provisioner must configure ports on the Envoy service and daemonset/deployment corresponding to all of the Gateway Listener ports. The mapping between service ports and container ports will be described in the Detailed Design section below.
- Contour's directed acyclic graph (DAG) processing changes, to first add Listeners, and then add virtual hosts directly to the appropriate Listener(s).
- Each DAG Listener results in an Envoy Listener being configured. The naming scheme will be described in the Detailed Design section below. 
- Envoy RouteConfigurations will be generated for each Envoy Listener. The naming scheme will be described in the Detailed Design section below. 

For Gateway mode, we must also consider what happens when HTTPProxy or Ingress is used for routing configuration.
The details of this use case are described in the Compatibility section below.

## Detailed Design

### internal/gatewayapi
The `ValidateListeners` function logic must change, to allow any number of unique ports, while still validating protocols and hostnames for each port.
Instead of returning a single insecure and secure port number, it will return a slice of ports.

### internal/provisioner
The Gateway controller will use the results of the `ValidateListeners` function (above) to add all relevant ports to `contourModel.Spec.NetworkPublishing.Envoy.Ports`.
These ports will be populated on the Envoy service and deployment/daemonset.

Currently, the single insecure port is always mapped to port 8080 inside the Envoy container, and the single secure port is always mapped to 8443.
Supporting any number of Listeners requires a more flexible port mapping scheme, since the requested Gateway Listener ports could be privileged ports (e.g. 1-1023), non-privileged ports (e.g. 8080, 8443), and/or node ports (e.g. 32080).
The main constraint is that, since Envoy runs as non-root, it cannot bind to privileged ports within the container.
As such, any requested privileged Gateway Listener port must be mapped to a non-privileged port inside the Envoy container.
The scheme proposed is as follows:
- any privileged Gateway Listener port (1-1023) is mapped to the range 64513-65535 within the container.
- any other Gateway Listener port (1024-65535) is mapped as-is within the container.
- conflicts between mapped privileged ports, and high non-privileged ports (in the range 64513-65535), will be detected and reported as status conditions on the Gateway/Listener, resulting in those Listeners not being programmed.

### internal/dag
The first step in DAG processing will now be to populate Listeners, based on either the Gateway spec (for Gateway mode), or the CLI flags/config file/ContourConfiguration (for non-Gateway mode).
The DAG will now store desired Listener names, addresses and "inside" ports.
Subsequently, in the Ingress/HTTPProxy/Gateway API processors, virtual hosts will be added directly to the appropriate DAG Listener(s), rather than to the root of the DAG.
The final step of DAG processing will still prune and sort Listeners and virtual hosts.

The Listener naming scheme will be as follows:
- for non-Gateway mode, the default Listeners will continue to be named `ingress_http` and `ingress_https` for compatibility.
- for Gateway mode, Listeners will be named `<http|https>_<port number>`, e.g. `http_80` or `https_8443`.

### internal/xdscache
All DAG Listeners will be converted to Envoy Listeners, using the DAG Listeners' name, address and port.
Each generated Envoy Listener will have one or more corresponding Envoy RouteConfigurations, as follows:
- HTTP Listeners will have a single corresponding RouteConfiguration. For non-Gateway mode, it will be named `ingress_http`, and for Gateway mode, it will be named the same as the Listener (e.g. `http_80`).
- HTTPS Listeners will have multiple corresponding RouteConfigurations:
    - each attached virtual host will have a RouteConfiguration. For non-Gateway mode, it will be named `https/<fqdn>`, and for Gateway mode, it will be named `<listener name>/<fqdn>`.
    - if any attached virtual host has the fallback certificate enabled, an additional RouteConfiguration will be created. For non-Gateway mode, it will be named `ingress_fallbackcert`, and for Gateway mode, it will be named `<listener name>/fallbackcert`.

For both non-Gateway and Gateway mode, the stats prefix for the HTTP connection manager will equal the Envoy Listener's name, i.e. `ingress_http` and `ingress_https` for non-Gateway mode, and `<http|https>_<port number>` for Gateway mode.

## Alternatives Considered
N/A

## Security Considerations
N/A

## Compatibility

### Static Gateway mode

It's also possible to configure Contour to correspond to a particular Gateway, while not using the Gateway provisioner to dynamically manage infrastructure.
This is described in detail [here](https://projectcontour.io/docs/v1.24.1/guides/gateway-api/#option-1-statically-provisioned).
In this case, the user is responsible for defining the Envoy Service YAML, including exposing any relevant ports.

For example, given the following Gateway:

```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
  namespace: projectcontour
spec:
  gatewayClassName: example
  listeners:
    - name: http-1
      protocol: HTTP
      port: 80
    - name: http-2
      protocol: HTTP
      port: 80
    - name: https-1
      protocol: HTTPS
      port: 443
      tls:
         ...
    - name: https-2
      protocol: HTTPS
      port: 444
      tls:
         ...
```

The user would need to define a corresponding Envoy Service:

```yaml
---
apiVersion: v1
kind: Service
metadata:
  name: envoy
  namespace: projectcontour
spec:
  ports:
  - port: 80
    name: http-1
    protocol: TCP
    targetPort: 64592 # matches Contour's mapping scheme described above
  - port: 81
    name: http-2
    protocol: TCP
    targetPort: 64593 # matches Contour's mapping scheme described above
  - port: 443
    name: https-1
    protocol: TCP
    targetPort: 64955 # matches Contour's mapping scheme described above
  - port: 444
    name: https-2
    protocol: TCP
    targetPort: 64956 # matches Contour's mapping scheme described above
  selector:
    app: envoy
  type: LoadBalancer
```

### Gateway mode with Ingress/HTTPProxy

When using Gateway mode, it is possible to use Ingress and/or HTTPProxy to define routing config.
In this scenario, Contour must be able to determine which Listener(s) to attach virtual hosts and routes to.
As an initial implementation, we are imposing a strict requirement for the Gateway spec in order to be compatible with Ingress and HTTPProxy:
- the Gateway _must_ have one or two Listeners.
- if the Gateway has one Listener, it _must_ be named `ingress_http` and will be used as the insecure Listener.
- if the Gateway has two Listeners, the second _must_ be named `ingress_https` and will be used as the secure Listener.

A valid Gateway spec for this use case looks like:
```yaml
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: httpproxy-compatible
  namespace: projectcontour
spec:
  gatewayClassName: contour
  listeners:
    - name: ingress_http
      protocol: HTTP
      port: 80
    - name: ingress_https
      protocol: HTTP # note, this can't be HTTPS because we aren't providing TLS details here.
      port: 443
```

If the Gateway does not conform to this spec, any HTTPProxy or Ingress will be considered invalid and will be marked as such.

In the future, we may consider enhancing support for this scenario.
For example, HTTPProxy could be extended with an optional `listeners` field that names the Gateway Listener(s) to attach to.
This could also enable scenarios where TLS details are provided on the Gateway Listener instead of the HTTPProxy.
However, this mixing and matching of APIs requires much more consideration and design and will not be tackled here.

### non-Gateway mode

As described above, one of the goals for this change is to have no impact on non-Gateway mode.
All Envoy configuration should remain exactly the same.
This will be verified through the existing suite of unit/feature/E2E tests.

### Insecure -> Secure Redirect

Contour supports automatically redirecting requests from HTTP to HTTPS, when a virtual host has been configured with TLS (read more [here](https://projectcontour.io/docs/v1.24.0/config/tls-termination/)).
The implementation of this redirect currently assumes that the "outside" ports are the standard HTTP/HTTPS ports 80 and 443.
As such, the redirect simply changes the scheme from `http` to `https`.
The details of this can be seen [here](https://github.com/projectcontour/contour/blob/v1.24.0/internal/envoy/v3/route.go#L423-L432).
This automatic redirect functionality is only supported for Ingress and HTTPProxy, while Gateway API can be explicitly configured by the user to do a similar redirect if desired.
For the purposes of this design, we propose not changing the implementation of this functionality.
The redirect will continue to be automatically enabled (for Ingress, only if `ingress.kubernetes.io/force-ssl-redirect` is set to `true`).
It will assume that the "outside" ports are the standard HTTP/HTTPS ports 80 and 443, and will not work properly otherwise.

## Implementation
An initial PR will refactor Contour internals to allow for any number of Envoy Listeners, while still only programming the two default ones.
A subsequent PR will make the necessary Gateway mode-related changes to actually program all Gateway Listeners.

## Open Issues
N/A
