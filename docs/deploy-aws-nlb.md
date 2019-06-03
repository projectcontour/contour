# Contour Deployment on AWS with NLB

This is an advanced deployment guide to configure Contour on AWS with the [Network Load Balancer (NLB)][1].
This configuration has several advantages:

1. NLBs are often cheaper. This is especially true for development. Idle LBs do not cost money.
2. There are no extra network hops. Traffic goes to the NLB, to the node hosting Contour, and then to the target pod.
3. Source IP addresses are retained. Envoy (running as part of Contour) sees the native source IP address and records this with an `X-Forwarded-For` header.

## Moving parts

- We run Contour as a DaemonSet across the cluster.
- The Contour pod runs with host networking and binds to port 8080 and 8443 on the node.
- Host networking means that traffic hits Envoy without transitioning through any other fancy networking hops.
- Contour also binds to 8001 for Envoy->Contour config traffic.

## Deploying Contour

1. [Clone the Contour repository][4] and cd into the repo.
2. Run `kubectl apply -f examples/ds-hostnet/`

This creates the `heptio-contour` Namespace along with a ServiceAccount, RBAC rules, and the DaemonSet itself.  It also creates the NLB based loadbalancer for you.

You can get the address of your NLB via:

```
kubectl get service contour --namespace=heptio-contour -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
```

## Test

You can now test your NLB.

1. Install a workload (see the kuard example in the [main deployment guide][2]).
2. Look up the address for your NLB in the AWS console and enter it in your browser.
  - Notice that Envoy fills out `X-Forwarded-For`, because it was the first to see the traffic directly from the browser.

[1]: https://aws.amazon.com/blogs/aws/new-network-load-balancer-effortless-scaling-to-millions-of-requests-per-second/
[2]: deploy-options.md#test
[3]: https://github.com/kubernetes/kubernetes/issues/52173
[4]: ../CONTRIBUTING.md
