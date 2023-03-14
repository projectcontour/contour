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

**NOTE**: The service annotation `service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: '*'` used to toggle the PROXY protocol is found to have no effect on NLBs (Due to this open [issue][2]). Hence, follow the steps mentioned in this AWS [documentation][3] to manually toggle PROXY protocol on NLBs

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

[0]: http://www.haproxy.org/download/1.8/doc/proxy-protocol.txt
[1]: https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer
[2]: https://github.com/kubernetes/kubernetes/issues/57250
[3]: https://docs.aws.amazon.com/elasticloadbalancing/latest/network/load-balancer-target-groups.html#enable-proxy-protocol
