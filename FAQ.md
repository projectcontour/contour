# Frequently Asked Questions

## Q: What's the relationship between Contour and Istio? 
**A**: Both projects use Envoy under the covers as a "data plane". They can both be thought of as ways to configure Envoy, but they approach configuration differently, and address different use cases.

Istio's service mesh model is intended to provide security, traffic direction, and insight within the cluster (east-west traffic) and between the cluster and the outside world (north-south traffic).

Contour focuses on north-south traffic only -- on making Envoy available to Kubernetes users as a simple, reliable ingress solution.

We continue to work with contributors to Istio and Envoy to find common ground; we're also part of Kubernetes SIG-networking. As Contour continues to develop -- who knows what the future holds?