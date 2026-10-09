---
title: How to Configure PROXY v1/v2 Support
---

If you deploy Contour as a Deployment or Daemonset, you will likely use a `type: LoadBalancer` Service to request an [external load balancer][1] from your hosting provider.
If you use the Elastic Load Balancer (ELB) service from Amazon's EC2, you need to perform a couple of additional steps to enable the [PROXY][0] protocol. Here's why:

External load balancers typically operate in one of two modes: a layer 7 HTTP proxy, or a layer 4 TCP proxy.
The former cannot be used to load balance TLS traffic, because your cloud provider attempts HTTP negotiation on port 443.
So the latter must be used when Contour handles HTTP and HTTPS traffic.

However this leads to a situation where the remote IP address of the client is reported as the inside address of your cloud provider's load balancer.
To rectify the situation, you can add annotations to your service and flags to your Contour Deployment or DaemonSet to enable the [PROXY][0] protocol which forwards the original client IP details to Envoy. 

## Enable PROXY protocol on your service in GKE

In GKE clusters a `type: LoadBalancer` Service is provisioned as a Network Load Balancer and will forward traffic to your Envoy instances with their client addresses intact.
Your services should see the addresses in the `X-Forwarded-For` or `X-Envoy-External-Address` headers without having to enable a PROXY protocol.

## Enable PROXY protocol on your service in AWS

To instruct EC2 to place the ELB into `tcp`+`PROXY` mode, add the following annotations to the `contour` Service:

```
apiVersion: v1
kind: Service
metadata:
  annotations:
      service.beta.kubernetes.io/aws-load-balancer-backend-protocol: tcp
      service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: '*'
    name: contour
    namespace: projectcontour
spec:
  type: LoadBalancer
...
```

## Enable PROXY protocol support for all Envoy listening ports

```
...
spec:
  containers:
  - image: ghcr.io/projectcontour/contour:<version>
    imagePullPolicy: Always
    name: contour
    command: ["contour"]
    args: ["serve", "--incluster", "--use-proxy-protocol"]
...
```

## Set X-Forwarded-Proto when the load balancer terminates TLS

If the load balancer terminates TLS itself, for example an AWS NLB with a TLS listener and an ACM certificate, and forwards plaintext to Envoy with the PROXY protocol, every connection Envoy accepts is plaintext.
Envoy then sets `X-Forwarded-Proto: http` on every request, and an HTTPProxy with TLS configured redirects its own HTTPS traffic to HTTPS in a loop.

Envoy 1.38 and later can derive `X-Forwarded-Proto` from the destination port carried in the PROXY protocol header instead.
Enable it in the Contour configuration file, together with `--use-proxy-protocol`:

```yaml
listener:
  forward-proto-config:
    https-destination-ports: [443]
    http-destination-ports: [80]
```

or in the `ContourConfiguration` resource:

```yaml
spec:
  envoy:
    listener:
      useProxyProtocol: true
      forwardProtoConfig:
        httpsDestinationPorts: [443]
        httpDestinationPorts: [80]
```

Requests that reached the load balancer on a port listed in `https-destination-ports` get `X-Forwarded-Proto: https`, those on a port in `http-destination-ports` get `http`, and any other port keeps Envoy's default behavior.

[0]: http://www.haproxy.org/download/1.8/doc/proxy-protocol.txt
[1]: https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer