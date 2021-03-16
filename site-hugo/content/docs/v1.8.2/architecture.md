# Contour Architecture

The Contour Ingress controller is a collaboration between:

* Envoy, which provides the high performance reverse proxy.
* Contour, which acts as a management server for Envoy and provides it with configuration.

These containers are deployed separately, Contour as a Deployment and Envoy as a Daemonset, although other configurations are possible.

In the Envoy Pods, Contour runs as an initcontainer in `bootstrap` mode and writes a bootstrap configuration to a temporary volume.
This volume is passed to the Envoy container and directs Envoy to treat Contour as its [management server][1].

After initialisation is complete, the Envoy container starts, retrieves the bootstrap configuration written by Contour's `bootstrap` mode, and establishes a GRPC session with Contour to receive configuration.

Envoy will gracefully retry if the management server is unavailable, which removes any container startup ordering issues.

Contour is a client of the Kubernetes API.
Contour watches Ingress, HTTPProxy, Secret, Service, and Endpoint objects, and acts as the management server for its Envoy sibling by translating its cache of objects into the relevant JSON stanzas: Service objects for CDS, Ingress for RDS, Endpoint objects for SDS, and so on).

The transfer of information from Kubernetes to Contour is by watching the API with the SharedInformer framework.

Kubernetes readiness probes are configured to check whether Envoy is ready to accept connections.
The Envoy readiness probe sends GET requests to `/ready` in Envoy's administration endpoint.

For Contour, a liveness probe checks the `/healthz` running on the Pod's metrics port.
Readiness probe is a TCP check that the gRPC port is open.

## Diagram
Below are a couple of high level architectural diagrams of how Contour works inside a Kubernetes cluster as well as showing the data path of a request to a backend pod.

A request to `projectcontour.io/blog` gets routed via a load balancer to an instance of an Envoy proxy which then sends the request to a pod.

![architectural overview][2]{: .center-image }


![architectural overview 2][3]{: .center-image }

[1]: https://www.envoyproxy.io/docs/envoy/v1.13.0/api-docs/xds_protocol
[2]: ../img/archoverview.png
[3]: ../img/contour_deployment_in_k8s.png
