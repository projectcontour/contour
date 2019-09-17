# Upgrading Contour

This document describes the changes needed to upgrade your Contour installation.

## Upgrading Contour 0.15 to 1.0.0-beta.1

Contour 1.0.0-beta.1 changes the namespace Contour is deployed too, promotes leader election to on by default, and introduces a new version of the IngressRoute CRD, now called HTTPProxy.

### Beta release

Contour 0.15.0 remains the current stable release.
The `:latest` tag will continue to point to 0.15.0 until Contour 1.0.0 is released.

### Deprecated deployments

The following deployment examples were deprecated in Contour 0.15 and have been removed:

- `deployment-grpc-v2`
- `ds-grpc-v2`
- `ds-hostnet-split`
- `ds-hostnet`

### IngressRoute v1beta1 deprecation

The IngressRoute v1beta1 CRD has been deprecated and will not receive further updates.
Contour will continue to recognize IngressRoute v1beta1 through Contour 1.0.0 final, however we anticipate it will be removed completely shortly after that.

The replacement for IngressRoute which we have called HTTPProxy is available in Contour 1.0.0-beta.1 and is anticipated to be declared final by Contour 1.0.0.

**TODO(dfc) link to HTTPProxy documentation**

## The easy way to upgrade

If the following are true for you:

 * Your previous installation is in the `heptio-contour` namespace.
 * You are using one of the [example](/example/) deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.0.0-beta.1 is to delete the `heptio-contour` namespace and reapply the `examples/contour` sample manifest.
From the root directory of the repository:
```
kubectl delete namespace heptio-contour

kubectl apply -f examples/contour
```
Note that `examples/contour` now deploys into the `projectcontour` namespace.

If you're using a `LoadBalancer` Service, deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address](./deploy-options.md#get_your_hostname_or_ip_address).

## The less easy way

This section contains information for administrators who wish to apply the Contour 0.15 to 1.0.0-beta.1 changes manually.

### Namespace change

As part of sunsetting the Heptio brand the `heptio-contour` namespace has been renamed to `projectcontour`.
Contour assumes it will be deployed into the `projectcontour` namespace.

If you deploy Contour into another namespace you will need to pass `contour bootstrap --namespace=<namespace>` and update the `contour.yaml` configuration file's leader election parameters as appropriate.

### Upgrade to Contour 1.0.0-beta.1

As part of sunsetting the Heptio brand Docker images have moved from `gcr.io/heptio-images` to `docker.io/projectcontour`.

Change the Contour image version to `docker.io/projectcontour/contour:v1.0.0-beta.1`.

### Recommended Envoy version

The recommended version of Envoy remains unchanged from Contour 0.15.
Ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.11.1`.

### Leader Election

Contour 1.0.0-beta.1 enables leader election by default.
No specific configuration is required 

Should you wish to disable leader election, pass `contour serve --disable-leader-election`.

### Envoy pod readiness checks

Update the readiness checks on your Envoy pod's spec to reflect Envoy 1.11.1's `/ready` endpoint
```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8002
```

### Root namespace restriction

The `contour serve --ingressroute-root-namespaces` flag has been renamed to `--root-namespaces`.
The previous flag's name will be supported until Contour 1.0.0-rc.1.
If you use this feature please update your deployments.

## Upgrading Contour 0.14 to 0.15

Contour 0.15 requires changes to your deployment manifests to explicitly opt in, or opt out of, secure communication between Contour and Envoy.

Contour 0.15 also adds experimental support for leader election which may be useful for installations which have split their Contour and Envoy containers into separate pods.
A configuration we call _split deployment_.

### Breaking change

Contour's `contour serve` now requires that either TLS certificates be available, or you supply the `--insecure` parameter.

**If you do not supply TLS details or `--insecure`, `contour serve` will not start.**

### Envoy 1.11.1 upgrade

Due to the recently announced HTTP/2 vulnerabilities Contour 0.15 requires Envoy 1.11.1.
As of August 2019, Envoy 1.11.1 is the only released version of Envoy that contains the fixes for those vulnerabilities.

Please see the [Envoy Release Notes](https://www.envoyproxy.io/docs/envoy/v1.11.1/intro/version_history) for information about issues fixed in Envoy 1.11.1.

## The easy way to upgrade

If the following are true for you:

 * Your installation is in the `heptio-contour` namespace.
 * You are using one of the [example](/example/) deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 0.15 is to delete the `heptio-contour` namespace and reapply one of the example configurations.
From the root directory of the repository:
```
kubectl delete namespace heptio-contour

kubectl apply -f examples/<your-desired-deployment>
```
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address](./deploy-options.md#get_your_hostname_or_ip_address).

### Note

If you deployed Contour into a different namespace than heptio-contour with a standard example, please delete that namespace. Then in your editor of choice do a search and replace for `heptio-contour` and replace it with your preferred name space and apply the updated manifest.

## The less easy way

This section contains information for administrators who wish to apply the Contour 0.14 to 0.15 changes manually.

### Upgrade to Contour 0.15.0

Due to the sun setting on the Heptio brand, from v0.15.0 onwards our images are now served from the docker hub repository [`docker.io/projectcontour/contour`](https://hub.docker.com/r/projectcontour/contour)

Change the Contour image version to `docker.io/projectcontour/contour:v0.15.0`.

### Enabling TLS for gRPC

You *must* either enable TLS for gRPC serving, or put `--insecure` into your `contour serve` startup line.
If you are running with both Contour and Envoy in a single pod, the existing deployment examples have already been updated with this change.

If you are running using the `ds-hostnet-split` example or a derivative, we strongly recommend that you generate new certificates for securing your gRPC communication between Contour and Envoy.

There is a Job in the `ds-hostnet-split` directory that will use the new `contour certgen` command
to generate a CA and then sign Contour and Envoy keypairs, which can also then be saved directly
to Kubernetes as Secrets, ready to be mounted into your Contour and Envoy Deployments and Daemonsets.

If you would like more detail, see (grpc-tls-howto.md)[./grpc-tls-howto.md], which explains your options.

### Upgrade to Envoy 1.11.1

Contour 0.15 requires Envoy 1.11.1.
Change the Envoy image version to `docker.io/envoyproxy/envoy:v1.11.1`.

### Enabling Leader Election

Contour 0.15 adds experimental support for leader election.
Enabling leader election will mean that only one of the Contour pods will actually serve gRPC traffic.
This will ensure that all Envoy's take their configuration from the same Contour.
You can enable leader election with the `--enable-leader-election` flag to `contour serve`.

If you have deployed Contour and Envoy in their own pods--we call this split deployment--you should enable leader election so all envoy pods take their configuration from the lead contour. 

To enable leader election, the following must be true for you:

- You are running in a split Contour and Envoy setup.
That is, there are separate Contour and Envoy pod(s).  

In order for leader election to work, you must make the following changes to your setup:

- The Contour Deployment must have its readiness probe changed too TCP readiness probe
configured to check port 8001 (the gRPC port), as non-leaders will not serve gRPC, and
Envoys may not be properly configured if they attempt to connect to a non-leader Contour.
That is, you will need to change
```
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8000
```
to
```
        readinessProbe:
          tcpSocket:
            port: 8001
          initialDelaySeconds: 15
          periodSeconds: 10
```
inside the Pod spec.
- The update strategy for the Contour deployment must be changed to `Recreate` instead of
`RollingUpdate`, as pods will never become Ready (since they won't pass the readiness probe).
Add
```
  strategy:
    type: Recreate
```
to the top level of the Pod spec.
- Leader election is currently hard-coded to use a ConfigMap named `contour` in this namespace
for the leader election lock. If you are using a newer installation of Contour, this may be
present already, if not, the leader election library will create an empty ConfigMap for you.

Once these changes are made, add `--enable-leader-election` to your `contour serve` command. The
leader will perform and log its operations as normal, and the non-leaders will block waiting to
become leader. You can inspect the state of the leadership using

```
kubectl describe configmap -n heptio-contour contour
```

and checking the annotations that store exact details using

```
kubectl get configmap -n heptio-contour -o yaml contour
```
