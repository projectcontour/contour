# Contour Architecture

The Contour Ingress controller is a collaboration between:

* Envoy, which provides the high performance reverse proxy.
* Contour, which acts as a management server for Envoy and provides it with configuration.

These containers are deployed separately, Contour as a Deployment and Envoy as a Kubernetes Daemonset or Deployment, although other configurations are possible.

In the Envoy Pods, Contour runs as an initcontainer in `bootstrap` mode and writes an Envoy bootstrap configuration to a temporary volume.
This volume is passed to the Envoy container and directs Envoy to treat Contour as its [management server][1].

After initialization is complete, the Envoy container starts, retrieves the bootstrap configuration written by Contour's `bootstrap` mode, and establishes a GRPC session with Contour to receive configuration.

Envoy will gracefully retry if the management server is unavailable, which removes any container startup ordering issues.

Contour is a client of the Kubernetes API.
Contour watches Ingress, HTTPProxy, Gateway API, Secret, Service, and Endpoint objects, and acts as the management server for its Envoy sibling by translating its cache of objects into the relevant JSON stanzas: Service objects for CDS, Ingress for RDS, Endpoint objects for EDS, and so on).

The transfer of information from Kubernetes to Contour is by watching the Kubernetes API utilizing [controller-runtime][4] primitives.

Kubernetes readiness probes are configured to check whether Envoy is ready to accept connections.
The Envoy readiness probe sends GET requests to `/ready` in Envoy's administration endpoint.

For Contour, a liveness probe checks the `/healthz` running on the Pod's metrics port.
Readiness probe is a check that Contour can access the Kubernetes API. 

## Architectural Overview
Below are a couple of high level architectural diagrams of how Contour works inside a Kubernetes cluster as well as showing the data path of a request to a backend pod.

A request to `projectcontour.io/blog` gets routed via a load balancer to an instance of an Envoy proxy which then sends the request to a pod.

![architectural overview][2]

Following is a diagram of how Contour and Envoy are deployed in a Kubernetes cluster. 

### Kubernetes API Server

The following API objects are watched:
- Services
- Endpoints
- Secrets
- Ingress
- HTTPProxy
- Gateway API (Optional)

### Contour Deployment

Contour is deployed in the cluster using a Kubernetes Deployment.
It has built-in leader election so the total number of replicas does not matter.
All instances are able to serve xDS configuration to any Envoy instance, but only the leader can write status back to the API server.

### Envoy Deployment

Envoy can be deployed in two different models, as a Kubernetes Daemonset or as a Kubernetes Deployment. 

Daemonset is the standard deployment model where a single instance of Envoy is deployed per Kubernetes Node.
This allows for simple Envoy pod distribution across the cluster as well as being able to expose Envoy using `hostPorts` to improve network performance. 
One potential downside of this deployment model is when a node is removed from the cluster (e.g. on a cluster scale down, etc) then the configured `preStop` hooks are not available so connections can be dropped.
This is a limitation that applies to any Daemonset in Kubernetes.

An alternative Envoy deployment model is utilizing a Kubernetes Deployment with a configured `podAntiAffinity` which attempts to mirror the Daemonset deployment model.
A benefit of this model compared to the Daemonset version is when a node is removed from the cluster, the proper shutdown events are available so connections can be cleanly drained from Envoy before terminating.

![architectural overview 2][3]

[1]: https://www.envoyproxy.io/docs/envoy/v1.13.0/api-docs/xds_protocol
[2]: ../img/archoverview.png
[3]: ../img/contour_deployment_in_k8s.png
[4]: https://github.com/kubernetes-sigs/controller-runtime
