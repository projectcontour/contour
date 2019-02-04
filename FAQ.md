# Frequently Asked Questions

## Q: What's the relationship between Contour and Istio? 

Both projects use Envoy under the covers as a "data plane". They can both be thought of as ways to configure Envoy, but they approach configuration differently, and address different use cases.

Istio's service mesh model is intended to provide security, traffic direction, and insight within the cluster (east-west traffic) and between the cluster and the outside world (north-south traffic).

Contour focuses on north-south traffic only -- on making Envoy available to Kubernetes users as a simple, reliable ingress solution.

We continue to work with contributors to Istio and Envoy to find common ground; we're also part of Kubernetes SIG-networking. As Contour continues to develop -- who knows what the future holds?

## Q: What are the differences between Ingress and IngressRoute?

Contour supports both the Kubernetes Ingress API and the IngressRoute API, a custom resource that enables a richer and more robust experience for configuring ingress into your Kubernetes cluster.

The Kubernetes Ingress API was introduced in version 1.2, and has not experienced significant progress since then. This is evidenced by the explosion of annotations used to express configuration that is otherwise not captured in the Ingress resource. Furthermore, as it stands today, the Ingress API is not suitable for clusters that are shared across multiple teams, as there is no way to enforce isolation to prevent teams from breaking each other's ingress configuration.

The IngressRoute custom resource is an attempt to solve these issues with an API that focuses on providing first-class support for HTTP(S) routing configuration instead of using annotations. More importantly, the IngressRoute API is designed with delegation in mind, a feature that enables administrators to configure top-level ingress settings (for example, which virtual hosts are available to each team), while delegating the lower-level configuration (for example, the mapping between paths and backend services) to each development team. More information about the IngressRoute API can be found [in the IngressRoute documentation](docs/ingressroute.md).
