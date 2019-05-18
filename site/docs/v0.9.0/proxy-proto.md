# EC2 ELB PROXY protocol support

If you deploy Contour as a Deployment or Daemonset, you will likely use a `type: LoadBalancer` Service to request an [external load balancer][1] from your hosting provider.
If you use the Elastic Load Balancer (ELB) service from Amazon's EC2, you need to perform a couple of additional steps to enable the [PROXY][0] protocol. Here's why:

External load balancers typically operate in one of two modes: a layer 7 HTTP proxy, or a layer 3 TCP proxy.
The former cannot be used to load balance TLS traffic, because your cloud provider attempts HTTP negotiation on port 443. So the latter must be used when Contour handles HTTP and HTTPS traffic.

However this leads to a situation where the remote IP address of the client is reported as the inside address of your cloud provider's load balancer.
To rectify the situation, you can add annotations to your service and flags to your Contour Deployment or DaemonSet to enable the [PROXY][0] protocol which forwards the original client IP details to Envoy. 

## Enable PROXY protocol on your service

To instruct EC2 to place the ELB into `tcp`+`PROXY` mode, add the following annotations to the `contour` Service:

```
apiVersion: v1
kind: Service
metadata:
  annotations:
      service.beta.kubernetes.io/aws-load-balancer-backend-protocol: tcp
      service.beta.kubernetes.io/aws-load-balancer-proxy-protocol: '*'
    name: contour
    namespace: heptio-contour
spec:
  type: LoadBalancer
...
```

## Enable PROXY protocol support for all Envoy listening ports

```
...
spec:
  containers:
  - image: gcr.io/heptio-images/contour:master
    imagePullPolicy: Always
    name: contour
    command: ["contour"]
    args: ["serve", "--incluster", "--use-proxy-protocol"]
...
```

[0]: http://www.haproxy.org/download/1.8/doc/proxy-protocol.txt
[1]: https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer
