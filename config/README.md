# Contour Installation

This directory contains Contour configuration suitable for use by itself, or with [kustomize](https://kustomize.io).


## Components

The [components](./components) directory contains the collaborating components
of a Contour installation.

1. [types](./types) contains the CRD types for the Contour API. If you have
   Kuberenetes 1.6 or later, [types-v1](./types-v1) contains the same API types
2. [contour](./contour) contains a deployment of the Contour service. This
   service will be a xDS management server for an Envoy cluster.
3. [envoy](./envoy) deploys an Envoy cluster as a Daemonset.
4. [certgen](./certgen) deploys a Contour generation Job to generate TLS
   certificates that will be used for the xDS session between Contour and
   Envoy.

Installing these components creates the following moving parts:

- Contour is run as Deployment and Envoy as a Daemonset
- Envoy runs on host networking
- Envoy runs on ports 80 & 443
- In our example deployment, the following certificates must be present as Secrets in the `projectcontour` namespace for the example YAMLs to apply:
  - `cacert`: must contain a `cacert.pem` key that contains a CA certificate that signs the other certificates.
  - `contourcert`: be a Secret of type `kubernetes.io/tls` and must contain `tls.crt` and `tls.key` keys that contain a certificate and key for Contour. The certificate must be valid for the name `contour` either via CN or SAN.
  - `envoycert`: be a Secret of type `kubernetes.io/tls` and must contain `tls.crt` and `tls.key` keys that contain a certificate and key for Envoy.

For detailed instructions on how to configure the required certs manually, see the [step-by-step TLS HOWTO](https://projectcontour.io/docs/master/grpc-tls-howto).

## Deployments

The [deployments](./deployments) directory contains pre-configured
deployments for a number of Kubernetes targets. These are largely
similar. They all install all the Contour components into the
`projectcontour` namespace and use `contour certgen` to create the xDS
session certificates.

The [quickstart YAML](./quickstart.yaml) is the rendered result of the
[base deployment](./deployments/base).

## Deploy Contour

Either:

```bash
kubectl apply -f https://projectcontour.io/quickstart/contour.yaml
```

or:

Clone or fork the repository, and run:

```bash
kustomize build config/deployments/base | kubectl apply -f -
```

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
