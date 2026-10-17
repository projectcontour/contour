---
title: Using an AWS Application Load Balancer with Contour
---

An AWS Application Load Balancer (ALB) can run in front of Contour when you
want AWS to handle layer 7 features at the edge. For example, the ALB can use
an AWS Certificate Manager (ACM) certificate to terminate TLS and can be
associated with an AWS WAF web ACL before requests reach the cluster.

In this setup, the [AWS Load Balancer Controller][1] manages one Kubernetes
Ingress that sends all requests to Envoy. Contour continues to manage the
HTTPProxy, Ingress, or Gateway API resources that route those requests to
application Services.

```text
client --HTTPS--> ALB --HTTP--> Envoy --HTTP/HTTPS--> application
                         |
                         +--HTTP :8002/ready health check
```

Unlike a Network Load Balancer, an ALB terminates the client connection and
creates a new HTTP connection to Envoy. Do not enable the PROXY protocol for
this configuration. See the existing [AWS NLB][2] and [NLB TLS
termination][8] guides when layer 4 load balancing is a better fit. The same
TLS and forwarded-header considerations apply to other layer 7 proxies, but
the manifests and annotations below are specific to AWS.

## Prerequisites

This guide assumes that:

- Contour is installed and its Envoy workload is selected by a Service named
  `envoy` in the `projectcontour` namespace. The Service has the default
  `http` port mapped to Envoy port `8080`, and Envoy serves health checks on
  port `8002`. Adjust the examples if your installation changes those names
  or ports.
- The AWS Load Balancer Controller is installed and provides an IngressClass
  named `alb`.
- The cluster networking makes pod IP addresses routable from the ALB. The
  AWS VPC CNI normally provides this on Amazon EKS.
- Eligible ALB subnets are configured explicitly or are discoverable by the
  AWS Load Balancer Controller.
- An [ACM certificate][6] for the application host names exists in the same
  AWS Region as the ALB.
- Application routes have already been created for those host names.

The example uses the controller's `ip` target type. The ALB registers Envoy
pod IP addresses directly, so the Envoy Service can remain internal and the
health check can use a port that the Service does not expose.

## Plan the Envoy Service type

Contour's example installation creates the Envoy Service with type
`LoadBalancer`. Leaving it that way would provision a second AWS load
balancer that bypasses the ALB described in this guide. Set the Service type
to `ClusterIP` in the installation manifests, Helm values, or other
deployment configuration that you manage for a new installation.

For an existing production installation, create and validate the ALB target
health before changing the Service. The controller's `ip` target type works
while the Service is still a `LoadBalancer`, which lets you plan the DNS
cutover first. Changing the Service to `ClusterIP` deprovisions its existing
cloud load balancer and can interrupt clients that still use that address.
Move DNS to the ALB, account for the old record's TTL, and then make the
Service internal.

When it is safe to retire the old load balancer, update an installation based
on Contour's example manifests with:

```sh
$ kubectl patch service envoy --namespace projectcontour \
    --type merge \
    --patch '{"spec":{"type":"ClusterIP","externalTrafficPolicy":null}}'
```

Persist the same setting in your deployment configuration so a later apply
does not restore the `LoadBalancer` type.

## Trust the ALB's forwarded headers

The ALB adds the original client address to `X-Forwarded-For` and records the
client protocol in `X-Forwarded-Proto`, as described in the [AWS header
documentation][7]. Configure Contour to trust the one ALB hop so Envoy and
upstream applications use those values:

```yaml
network:
  num-trusted-hops: 1
```

This value assumes one ALB using its default `append` mode for
`X-Forwarded-For`. Recalculate the trusted-hop count if another proxy is
added or the ALB's forwarded-header processing mode is changed.

Merge this block into the existing `contour.yaml` configuration and restart
the Contour pods. When using a ContourConfiguration or ContourDeployment
instead, set `spec.envoy.network.numTrustedHops` or
`spec.runtimeSettings.envoy.network.numTrustedHops`, respectively. See the
[Contour configuration reference][3] for the configuration method used by
your installation.

{{< notice warning >}}
`num-trusted-hops` applies to every request received by Envoy. Prevent clients
from reaching Envoy directly, or they could supply trusted forwarded headers
themselves. Restrict the Envoy listener and health-check ports so they are
reachable from the ALB, but not from untrusted networks.
For an existing deployment, enforce this boundary as part of the ALB cutover
before enabling trusted hops.
{{< /notice >}}

## Terminate TLS only at the ALB

This example sends HTTP from the ALB to Envoy. Configure the public host name
on the Contour route, but do not configure downstream TLS on that route. For
example, this HTTPProxy intentionally omits `spec.virtualhost.tls`:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
  namespace: default
spec:
  virtualhost:
    fqdn: app.example.com
  routes:
    - conditions:
        - prefix: /
      services:
        - name: example
          port: 80
```

Similarly, a Kubernetes Ingress used as an application route should omit
`spec.tls` and should not set
`ingress.kubernetes.io/force-ssl-redirect: "true"`. A Gateway API deployment
must provide an HTTP listener for the ALB-to-Envoy connection rather than
only an HTTPS listener. The ACM certificate on the ALB protects the
client-facing connection.

For an existing TLS-enabled deployment, coordinate these route changes with
the ALB and DNS cutover. Removing Contour-side TLS while clients still use
the old load balancer interrupts that path.

{{< notice warning >}}
When TLS is configured on an HTTPProxy virtual host, Contour redirects HTTP
requests to HTTPS by default. Because the ALB-to-Envoy connection in this
example is HTTP, that configuration can cause a redirect loop. Remove
Contour-side TLS from routes that rely on ALB TLS termination. Re-encryption
from the ALB to Envoy requires a different configuration and is outside the
scope of this guide.
{{< /notice >}}

## Create the ALB

Create the following Ingress in the same namespace as the Envoy Service.
Replace the certificate ARN with the ARN of your ACM certificate:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: contour-alb
  namespace: projectcontour
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/target-type: ip
    alb.ingress.kubernetes.io/listen-ports: '[{"HTTP": 80}, {"HTTPS": 443}]'
    alb.ingress.kubernetes.io/ssl-redirect: '443'
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-west-2:123456789012:certificate/00000000-0000-0000-0000-000000000000
    alb.ingress.kubernetes.io/backend-protocol: HTTP
    alb.ingress.kubernetes.io/healthcheck-protocol: HTTP
    alb.ingress.kubernetes.io/healthcheck-port: '8002'
    alb.ingress.kubernetes.io/healthcheck-path: /ready
    alb.ingress.kubernetes.io/success-codes: '200'
spec:
  ingressClassName: alb
  defaultBackend:
    service:
      name: envoy
      port:
        name: http
```

The example creates an internet-facing ALB. Use `internal` for the `scheme`
annotation instead when only private clients should reach it.

The `alb` ingress class ensures the AWS Load Balancer Controller, rather than
Contour, reconciles this front-door Ingress. The default backend sends every
host to the Envoy Service. Envoy then uses the application routes managed by
Contour to select the final Service.

With `target-type: ip`, the Service's `http` port resolves to Envoy's pod port
`8080`. The separate numeric health-check port makes the ALB request
`http://<envoy-pod-ip>:8002/ready`. Envoy returns `200` only when it is ready
to accept traffic.

Apply the Ingress and wait for its address to be populated:

```sh
$ kubectl apply -f contour-alb.yaml
$ kubectl get ingress contour-alb --namespace projectcontour --watch
```

Create a DNS alias for each application host name that points to the ALB
address. For the standard port `80` and `443` listeners shown here, the ALB
forwards the requested host name, which lets Envoy match the request to the
corresponding Contour route.

## Publish the ALB address in route status

After changing the Envoy Service to `ClusterIP`, it no longer has an external
address for Contour to copy into the status of managed HTTPProxy, Ingress,
and Gateway resources. Get the ALB hostname from the front-door Ingress:

```sh
$ kubectl get ingress contour-alb --namespace projectcontour \
    --output jsonpath='{.status.loadBalancer.ingress[0].hostname}{"\n"}'
```

Add that hostname to `contour.yaml` so Contour publishes the useful address
on its managed routes:

```yaml
ingress-status-address: k8s-projectc-contoura-0000000000.us-west-2.elb.amazonaws.com
```

Restart the Contour pods after changing the ConfigMap. When using a
ContourConfiguration or ContourDeployment instead, set
`spec.ingress.statusAddress` or
`spec.runtimeSettings.ingress.statusAddress`, respectively. This status
setting aids discovery; it does not change request routing.

## Optional: associate an AWS WAF web ACL

After creating an AWS WAFv2 regional web ACL, add its ARN to the Ingress:

```yaml
metadata:
  annotations:
    alb.ingress.kubernetes.io/wafv2-acl-arn: arn:aws:wafv2:us-west-2:123456789012:regional/webacl/example/00000000-0000-0000-0000-000000000000
```

The AWS Load Balancer Controller's IAM role must have permission to associate
the web ACL. See the controller's [Ingress annotation reference][4] for the
current WAF and certificate annotations.

## Secure the target ports

The ALB must be able to reach each Envoy pod on TCP ports `8080` and `8002`.
Restrict security groups and network policies so that:

- public clients can reach only the ALB listeners;
- the ALB can reach Envoy on `8080` for application traffic;
- the ALB can reach Envoy on `8002` for health checks; and
- untrusted sources cannot reach Envoy directly through pod IPs, node
  `hostPort` mappings, or another Service.

Port `8002` is Contour's default Envoy metrics and health listener. It is not
part of the ALB's public listener rules and should not be exposed publicly.
AWS documents the required listener and health-check rules in its [security
group guidance][5].

## Verify the deployment

Check the following before sending production traffic:

1. `kubectl get service envoy --namespace projectcontour` reports
   `TYPE` as `ClusterIP`.
2. The AWS target group contains the Envoy pod IPs on port `8080` and reports
   them healthy using port `8002` and path `/ready`.
3. An HTTPS request to an application host returns the ACM certificate and
   reaches the expected backend without repeated redirects.
4. The backend observes the client address in `X-Forwarded-For` and `https`
   in `X-Forwarded-Proto`.
5. Direct requests to the Envoy listener and health-check ports are blocked
   from untrusted networks.

If targets remain unhealthy, verify pod-IP routing and the security rules for
port `8002`. If requests repeatedly redirect to the same HTTPS URL, check the
application route for Contour-side TLS configuration. If forwarded client
information is incorrect, verify `network.num-trusted-hops` and ensure there
is exactly one trusted HTTP proxy between the client and Envoy.

[1]: https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/deploy/installation/
[2]: deploy-aws-nlb
[3]: ../configuration/#network-configuration
[4]: https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/ingress/annotations/
[5]: https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-update-security-groups.html
[6]: https://docs.aws.amazon.com/acm/latest/userguide/acm-overview.html
[7]: https://docs.aws.amazon.com/elasticloadbalancing/latest/application/x-forwarded-headers.html
[8]: deploy-aws-tls-nlb
