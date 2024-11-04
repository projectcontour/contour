# Gateway API

## Introduction

[Gateway API][1] is an open source project managed by the SIG Network community.
It is a collection of resources that model service networking in Kubernetes.
These resources - GatewayClass, Gateway, HTTPRoute, TCPRoute, Service, etc - aim to evolve Kubernetes service networking through expressive, extensible, and role-oriented interfaces that are implemented by many vendors and have broad industry support.

Contour implements Gateway API in addition to supporting HTTPProxy and Ingress.
In particular, Contour aims to support all [core and extended features][2] in Gateway API.

Gateway API has a comprehensive [website and docs][1], so this document focuses primarily on unique aspects of Contour's Gateway API implementation, rather than attempting to reproduce all of the content available on the Gateway API website.
The reader is suggested to familiarize themselves with the basics of Gateway API before continuing with this doc.

In Contour's Gateway API implementation, a Gateway corresponds 1:1 with a single deployment of Contour + Envoy.
In other words, each Gateway has its own control plane (Contour) and data plane (Envoy).

The remainder of this document delves into more detail regarding configuration options when using Contour with Gateway API.
If you are looking for a way to get started with Gateway API and Contour, see the [Gateway API guide][12], a step-by-step tutorial on getting Contour installed with Gateway API and using it to route traffic to a service.

## Enabling Gateway API in Contour

There are two ways to deploy Contour with Gateway API support: **static** provisioning and **dynamic** provisioning.

In **static** provisioning, the platform operator defines a `Gateway` resource, and then manually deploys a Contour instance corresponding to that `Gateway` resource.
It is up to the platform operator to ensure that all configuration matches between the `Gateway` and the Contour/Envoy resources.
Contour will then process that `Gateway` and its routes.

In **dynamic** provisioning, the platform operator first deploys Contour's Gateway provisioner. Then, the platform operator defines a `Gateway` resource, and the provisioner automatically deploys a Contour instance that corresponds to the `Gateway's` configuration and will process that `Gateway` and its routes.

Static provisioning makes sense for users who:
- prefer the traditional model of deploying Contour
- have only a single Gateway
- want to use just the standard listener ports (80/443)
- have highly customized YAML for deploying Contour.

Dynamic provisioning makes sense for users who:
- have many Gateways
- want to use additional listener ports
- prefer a simple declarative API for provisioning Contour instances
- want a fully conformant Gateway API implementation

### Static Provisioning

To statically provision Contour with Gateway API enabled:

1. Install the [Gateway API experimental channel][3].
1. Create a GatewayClass, with a controller name of `projectcontour.io/gateway-controller`.
1. Create a Gateway using the above GatewayClass.
1. In the Contour config file, add a reference to the above Gateway via `gateway.gatewayRef` (see https://projectcontour.io/docs/1.25/configuration/#gateway-configuration)
1. Install Contour using the above config file.

Contour provides an example manifest for this at https://projectcontour.io/quickstart/contour-gateway.yaml.

### Dynamic Provisioning

To dynamically provision Contour with Gateway API enabled:

1. Install the [Contour Gateway Provisioner][9], which includes the Gateway API experimental channel.
1. Create a GatewayClass, with a controller name of `projectcontour.io/gateway-controller`.
1. Create a Gateway using the above GatewayClass.

The Contour Gateway Provisioner will deploy an instance of Contour in the Gateway's namespace implementing the Gateway spec.

**Note:** Gateway names must be 63 characters or shorter, to avoid issues when generating dependent resources. See [projectcontour/contour#5970][13] and [kubernetes-sigs/gateway-api#2592][14] for more information.

## Gateway Listeners

Each unique Gateway Listener port requires the Envoy service to expose that port, and to map it to an underlying port in the Envoy daemonset/deployment that Envoy is configured to listen on.
For example, the following Gateway Listener configuration (abridged) requires service ports of 80 and 443, mapped to underlying container ports 8080 and 8443:

```yaml
listeners:
- name: http
  protocol: HTTP
  port: 80
- name: https
  protocol: HTTPS
  port: 443
```

In dynamic provisioning, the Contour Gateway Provisioner will continuously ensure that the Envoy service and daemonset/deployment are kept in sync with the Gateway Listener configuration.
In static provisioning, it is up to the platform operator to keep the Envoy resources in sync with the Gateway Listeners.

To get from the Gateway Listener port to the port that Envoy will be configured to listen on, i.e. the container port:
- add 8000 to the Listener port number
- if the result is greater than 65535, subtract 65535
- if the result is less than or equal to 1023, add 1023.

Note that, in rare corner cases, it's possible to have port conflicts.
Check the Gateway status to ensure that Listeners have been properly provisioned.

## Routing

Gateway API defines multiple route types.
Each route type is appropriate for a different type of traffic being proxied to a backend service.
Contour implements `HTTPRoute`, `TLSRoute`, `GRPCRoute` and `TCPRoute`.
The details of each of these route types are covered in extensive detail on the Gateway API website; the [route resources overview][11] is a good place to start learning about them.

### Routing with HTTPProxy or Ingress

When Gateway API is enabled in Contour, it's still possible to use HTTPProxy or Ingress to define routes, with some limitations.
This is useful for users who:
- are in the process of migrating to Gateway API
- want to use the Contour Gateway Provisioner for dynamic provisioning, but need the advanced features of HTTPProxy

To use HTTPProxy or Ingress with Gateway API, define a Gateway with the following Listeners:

```yaml
listeners:
- name: http
  protocol: HTTP
  port: 80
  allowedRoutes:
    namespaces:
      from: All
- name: https
  protocol: projectcontour.io/https
  port: 443
  allowedRoutes:
    namespaces:
      from: All
```

Note that for the second Listener, a Contour-specific protocol is used, and no TLS details are specified.
Instead, TLS details continue to be configured on the HTTPProxy or Ingress resource.

This is an area of active development and further work will be done in upcoming releases to better support migrations and mixed modes of operation.

## Contour Gateway Provisioner

### Customizing a GatewayClass

Gateway API [supports attaching parameters to a GatewayClass][5], which can customize the Gateways that are provisioned for that GatewayClass.

Contour defines a CRD called `ContourDeployment`, which can be used as `GatewayClass` parameters.

A simple example of a parameterized Contour GatewayClass that provisions Envoy as a Deployment instead of the default DaemonSet looks like:

```yaml
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: contour-with-envoy-deployment
spec:
  controllerName: projectcontour.io/gateway-controller
  parametersRef:
    kind: ContourDeployment
    group: projectcontour.io
    name: contour-with-envoy-deployment-params
    namespace: projectcontour
---
kind: ContourDeployment
apiVersion: projectcontour.io/v1alpha1
metadata:
  namespace: projectcontour
  name: contour-with-envoy-deployment-params
spec:
  envoy:
    workloadType: Deployment
```

All Gateways provisioned using the `contour-with-envoy-deployment` GatewayClass would get an Envoy Deployment.

See [the API documentation][6] for all `ContourDeployment` options.

It's important to note that, per the [GatewayClass spec][10]:

> It is recommended that [GatewayClass] be used as a template for Gateways.
> This means that a Gateway is based on the state of the GatewayClass at the time it was created and changes to the GatewayClass or associated parameters are not propagated down to existing Gateways.
> This recommendation is intended to limit the blast radius of changes to GatewayClass or associated parameters.
> If implementations choose to propagate GatewayClass changes to existing Gateways, that MUST be clearly documented by the implementation.

Contour follows the recommended behavior, meaning changes to a GatewayClass and its parameters are not propagated down to existing Gateways.

### Upgrades

When the Contour Gateway Provisioner is upgraded to a new version, it will upgrade all Gateways it controls (both the control plane and the data plane).

## Disabling Experimental Resources

Some users may want to use Contour with the [Gateway API standard channel][4] instead of the experimental channel, to avoid installing alpha resources into their clusters.
To do this, Contour must be told to disable informers for the experimental resources.
In the Contour (control plane) deployment, use the `--disable-feature` flag for `contour serve` to disable informers for the experimental resources:

```yaml
containers:
- name: contour
  image: ghcr.io/projectcontour/contour:<version>
  command: ["contour"]
  args:
  - serve
  - --incluster
  - --xds-address=0.0.0.0
  - --xds-port=8001
  - --contour-cafile=/certs/ca.crt
  - --contour-cert-file=/certs/tls.crt
  - --contour-key-file=/certs/tls.key
  - --config-path=/config/contour.yaml
  - --disable-feature=tlsroutes
  - --disable-feature=tcproutes
  ...
```

[1]: https://gateway-api.sigs.k8s.io/
[2]: https://gateway-api.sigs.k8s.io/concepts/conformance/#2-support-levels
[3]: https://gateway-api.sigs.k8s.io/guides/#install-experimental-channel
[4]: https://gateway-api.sigs.k8s.io/guides/#install-standard-channel
[5]: https://gateway-api.sigs.k8s.io/api-types/gatewayclass/#gatewayclass-parameters
[6]: https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1alpha1.ContourDeployment
[7]: https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1alpha1.GatewayConfig
[8]: https://gateway-api.sigs.k8s.io/api-types/gatewayclass/#gatewayclass-controller-selection
[9]: https://projectcontour.io/quickstart/contour-gateway-provisioner.yaml
[10]: https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1.GatewayClass
[11]: https://gateway-api.sigs.k8s.io/concepts/api-overview/#route-resources
[12]: /docs/{{< param version >}}/guides/gateway-api
[13]: https://github.com/projectcontour/contour/issues/5970
[14]: https://github.com/kubernetes-sigs/gateway-api/issues/2592