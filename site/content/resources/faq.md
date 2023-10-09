---
title: Frequently Asked Questions
layout: page
---

## Q: What's the relationship between Contour and Istio? 

Both projects use Envoy under the covers as a "data plane".
They can both be thought of as ways to configure Envoy, but they approach configuration differently, and address different use cases.

Istio's service mesh model is intended to provide security, traffic direction, and insight within the cluster (east-west traffic) and between the cluster and the outside world (north-south traffic).

Contour focuses on north-south traffic only -- on making Envoy available to Kubernetes users as a simple, reliable load balancing solution.

We continue to work with contributors to Istio and Envoy to find common ground; we're also part of Kubernetes SIG-networking.
As Contour continues to develop -- who knows what the future holds?

## Q: What are the differences between Ingress and HTTPProxy?

Contour supports both the Kubernetes Ingress API and the HTTPProxy API, a custom resource that enables a richer and more robust experience for configuring ingress into your Kubernetes cluster.

The Kubernetes Ingress API was introduced in version 1.2, and has not experienced significant progress since then.
This is evidenced by the explosion of annotations used to express configuration that is otherwise not captured in the Ingress resource.
Furthermore, as it stands today, the Ingress API is not suitable for clusters that are shared across multiple teams, as there is no way to enforce isolation to prevent teams from breaking each other's ingress configuration.

The HTTPProxy custom resource is an attempt to solve these issues with an API that focuses on providing first-class support for HTTP(S) routing configuration instead of using annotations.
More importantly, the HTTPProxy CRD is designed with inclusion in mind, a feature that enables administrators to configure top-level ingress settings (for example, which virtual hosts are available to each team), while delegating the lower-level configuration (for example, the mapping between paths and backend services) to each development team.
More information about the HTTPProxy API can be found [in the HTTPProxy documentation][1].

## Q: When I load my site in Safari, it shows me an empty page. Developer tools show that the HTTP response was 421. Why does this happen?

The HTTP/2 specification allows user agents (browsers) to reuse TLS sessions to different hostnames as long as they share an IP address and a TLS server certificate (see [RFC 7540](https://tools.ietf.org/html/rfc7540#section-9.1.1)).
Sharing a TLS certificate typically uses a wildcard certificate, or a certificate containing multiple alternate names.
If this kind of session reuse is not supported by the server, it sends a "421 Misdirected Request", and the user agent may retry the request with a new TLS session.
Although Chrome and Firefox correctly retry 421 responses, Safari does not, and simply displays the 421 response body.

Contour programs Envoy with a tight binding between TLS server names and HTTP virtual host routing tables.
This is done for security reasons, so that TLS protocol configuration guarantees can be made for virtual hosts, TLS client authentication can be correctly enforced, and security auditing can be applied consistently across protocol layers.

The best workaround for this Safari issue is to avoid the use of wildcard certificates.
[cert-manager](https://cert-manager.io) can automatically issue TLS certificates for Ingress and HTTPProxy resources (see the [configuration guide][2]).
If wildcard certificates cannot be avoided, the other workaround is to disable HTTP/2 support which will prevent inappropriate TLS session reuse.
HTTP/2 support can be disabled by setting the `default-http-versions` field in the Contour [configuration file][3].

## Q: Why is the Envoy container not accepting connections even though Contour is running?

Contour does not configure Envoy to listen on a port unless there is traffic to be served.
For example, if you have not configured any TLS Ingress objects then Contour does not command Envoy to open the secure listener (port 443 in the example deployment).
Because the HTTP and HTTPS listeners both use the same code, if you have no Ingress objects deployed in your cluster, or if no Ingress objects are permitted to talk on HTTP, then Envoy does not listen on the insecure port (port 80 in the example deployment).

To test whether Contour is correctly deployed you can deploy the kuard example service:

```sh
$ kubectl apply -f https://projectcontour.io/examples/kuard.yaml
```

[1]: /docs/{{< param latest_version >}}/config/fundamentals
[2]: /docs/{{< param latest_version >}}/guides/cert-manager
[3]: /docs/{{< param latest_version >}}/configuration
