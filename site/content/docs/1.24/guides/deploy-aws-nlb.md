---
title: Deploying Contour on AWS with NLB
---

This is an advanced deployment guide to configure Contour on AWS with the [Network Load Balancer (NLB)][1].
This configuration has several advantages:

1. NLBs are often cheaper. This is especially true for development. Idle LBs do not cost money.
2. There are no extra network hops. Traffic goes to the NLB, to the node hosting Contour, and then to the target pod.
3. Source IP addresses are retained. Envoy (running as part of Contour) sees the native source IP address and records this with an `X-Forwarded-For` header.

## Moving parts

- We run Envoy as a DaemonSet across the cluster and Contour as a deployment
- The Envoy pod runs on host ports 80 and 443 on the node
- Host networking means that traffic hits Envoy without transitioning through any other fancy networking hops
- Contour also binds to 8001 for Envoy->Contour config traffic.

## Deploying Contour

1. [Clone the Contour repository][4] and cd into the repo 
2. Edit the Envoy service (`02-service-envoy.yaml`) in the `examples/contour` directory:
    - Remove the existing annotation: `service.beta.kubernetes.io/aws-load-balancer-backend-protocol: tcp`
    - Add the following annotation: `service.beta.kubernetes.io/aws-load-balancer-type: nlb`
3. Run `kubectl apply -f examples/contour`

This creates the `projectcontour` Namespace along with a ServiceAccount, RBAC rules, Contour Deployment and an Envoy DaemonSet. 
It also creates the NLB based loadbalancer for you.

You can get the address of your NLB via:

```
$ kubectl get service envoy --namespace=projectcontour -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
```

## Test

You can now test your NLB.

1. Install a workload (see the kuard example in the [main deployment guide][2]).
2. Look up the address for your NLB in the AWS console and enter it in your browser.
  - Notice that Envoy fills out `X-Forwarded-For`, because it was the first to see the traffic directly from the browser.

[1]: https://aws.amazon.com/blogs/aws/new-network-load-balancer-effortless-scaling-to-millions-of-requests-per-second/
[2]: ../deploy-options/#testing-your-installation
[3]: https://github.com/kubernetes/kubernetes/issues/52173
[4]: {{< param github_url >}}/tree/{{< param branch >}}
