# Contour Installation

This is an installation guide to configure Contour in a Deployment separate from Envoy which allows for easier scaling of each component.

This configuration has several advantages:

1. Envoy runs as a daemonset which allows for distributed scaling across workers in the cluster
2. Communication between Contour and Envoy is secured by mutually-checked self-signed certificates.

## Moving parts

- Contour is run as Deployment and Envoy as a Daemonset
- Envoy runs on host networking
- Envoy runs on ports 80 & 443

The TLS secrets used to secure the gRPC session between Contour and Envoy are generated using a Job that runs `contour certgen`.
For detailed instructions on how to configure the required secrets manually, see the [step-by-step TLS HOWTO](https://projectcontour.io/docs/master/grpc-tls-howto).

## Deploy Contour

Either:

1. Run `kubectl apply -f https://projectcontour.io/quickstart/contour.yaml`

or:
Clone or fork the repository, then run:

```bash
kubectl apply -f examples/contour
```

This will:

- set up RBAC and Contour's CRDs (CRDs include HTTPProxy, TLSCertificateDelegation)
- run a Kubernetes Job that will generate one-year validity certs and put them into `projectcontour`
- Install Contour and Envoy in a Deployment and Daemonset respectively.

**NOTE**: The current configuration exposes the `/stats` path from the Envoy Admin UI so that Prometheus can scrape for metrics.

## Test

1. Install a workload (see the kuard example in the [main deployment guide](https://projectcontour.io/docs/master/deploy-options/#test-with-httpproxy)).

## Deploying with Host Networking enabled for Envoy

In order to deploy the Envoy Daemonset with host networking enabled, you need to make two changes.

In the Envoy daemonset definition, at the Pod spec level, change:

```yaml
dnsPolicy: ClusterFirst
```

to

```yaml
dnsPolicy: ClusterFirstWithHostNet
```

and add

```yaml
hostNetwork: true
```

Then, in the Envoy Service definition, change the annotation from:

```yaml
  # This annotation puts the AWS ELB into "TCP" mode so that it does not
  # do HTTP negotiation for HTTPS connections at the ELB edge.
  # The downside of this is the remote IP address of all connections will
  # appear to be the internal address of the ELB. See docs/proxy-proto.md
  # for information about enabling the PROXY protocol on the ELB to recover
  # the original remote IP address.
  service.beta.kubernetes.io/aws-load-balancer-backend-protocol: tcp
```

to

```yaml
   service.beta.kubernetes.io/aws-load-balancer-type: nlb
```

Then, apply the example as normal. This will still deploy a LoadBalancer Service, but it will be an NLB instead of an ELB.
