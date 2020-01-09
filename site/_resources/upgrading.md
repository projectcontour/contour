---
title: Upgrading Contour
layout: page
# NOTE(jpeach): we have to use 'h1,h2' here, because the TOC is not generated at all if we just use 'h1'.
toc: h1,h2
---

<!-- NOTE: this document should be formatted with one sentence per line to made reviewing easier. -->

This document describes the changes needed to upgrade your Contour installation.

# Upgrading Contour 1.0.1 to 1.1.0

Contour 1.1.0 is the current stable release.

<div class="alert-deprecation">
<b>Deprecation Notice</b></br>
<p>
The <code>IngressRoute</code> CRD has been deprecated and will not receive further updates. Contour 1.1.0 continues to support the IngressRoute API, however we anticipate it will be removed in the future.
<p>
</p>
Please see the documentation for <a href="{% link docs/v1.1.0/httpproxy.md %}"><code>HTTPProxy</code></a>, which is the successor to <code>IngressRoute</code>.
You can also read the <a href="{% link _guides/ingressroute-to-httpproxy.md %}">IngressRoute to HTTPProxy upgrade</a> guide.
</div>

&nbsp;

## Recommended Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.12.2`.

Please see the [Envoy Release Notes][15] for information about issues fixed in Envoy 1.12.2.

## The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using one of the [example][11] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.1.0 is to delete the `projectcontour` namespace and reapply one of the example configurations.
From the root directory of the repository:

```
$ kubectl delete namespace projectcontour
$ kubectl apply -f examples/<your-desired-deployment>
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

**Note:** If you deployed Contour into a different namespace than `projectcontour` with a standard example, please delete that namespace.
Then in your editor of choice do a search and replace for `projectcontour` and replace it with your preferred name space and apply the updated manifest.

## The less easy way

This section contains information for administrators who wish to apply the Contour 1.0.1 to 1.1.0 changes manually.

### Upgrade to Contour 1.1.0

Change the Contour image version to `docker.io/projectcontour/contour:v1.1.0`.

### Upgrade to Envoy 1.12.2

Contour 1.1.0 requires Envoy 1.12.2. Change the Envoy image version to `docker.io/envoyproxy/envoy:v1.12.2`.

# Upgrading Contour 1.0.0 to 1.0.1

Contour 1.0.1 is the current stable release.

## The easy way to upgrade

If you are running Contour 1.0.0, the easy way to upgrade to Contour 1.0.1 is to reapply the [quickstart yaml][16].

```bash
$ kubectl apply -f {{ site.url }}/quickstart/contour.yaml
```

## The less easy way

This section contains information for administrators who wish to manually upgrade from Contour 1.0.0 to Contour 1.0.1.

### Contour version

Ensure the Contour image version is `docker.io/projectcontour/contour:v1.0.1`.

### Envoy version

Ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.12.2`.

Please see the [Envoy Release Notes][15] for information about issues fixed in Envoy 1.12.2.

# Upgrading Contour 0.15.3 to 1.0.0

## Recommended Envoy version

The recommended version of Envoy remains unchanged.
Ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.11.2`.

## The easy way to upgrade

If the following are true for you:

 * Your previous installation is in the `projectcontour` namespace.
 * You are using one of the [example][1] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade is to delete the `projectcontour` namespace and reapply the `examples/contour` sample manifest.
From the root directory of the repository:

```
$ kubectl delete namespace projectcontour
$ kubectl apply -f examples/contour
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

## The less easy way

This section contains information for administrators who wish to manually upgrade from Contour 0.15.3 to Contour 1.0.0.

### Upgrade to Contour 1.0.0

Change the Contour image version to `docker.io/projectcontour/contour:v1.0.0`.

Note that as part of sunsetting the Heptio brand, Contour Docker images have moved from `gcr.io/heptio-images` to `docker.io/projectcontour`.

### Reapply HTTPProxy and IngressRoute CRD definitions

Contour 1.0.0 ships with updated OpenAPIv3 validation schemas.

Contour 1.0.0 promotes the HTTPProxy CRD to v1.
HTTPProxy is now considered stable, and there will only be additive, compatible changes in the future.
See the [HTTPProxy documentation][3] for more information.

```
$ kubectl apply -f examples/contour/01-crds.yaml
```

### Update deprecated `contour.heptio.com` annotations

All the annotations with the prefix `contour.heptio.com` have been migrated to their respective `projectcontour.io` counterparts.
The deprecated `contour.heptio.com` annotations will be recognized through the Contour 1.0 release, but are scheduled to be removed after Contour 1.0.

See the [annotation documentation][4] for more information.

### Update old `projectcontour.io/v1alpha1` group versions

If you are upgrading a cluster that you previously installed a 1.0.0 release candidate, note that Contour 1.0.0 moves the HTTPProxy CRD from `projectcontour.io/v1alpha1` to `projectcontour.io/v1` and will no longer recognize the former group version.

Please edit your HTTPProxy documents to update their group version to `projectcontour.io/v1`.

### Check for HTTPProxy v1 schema changes

As part of finalizing the HTTPProxy v1 schema, three breaking changes have been introduced.
If you are upgrading a cluster that you previously installed a Contour 1.0.0 release candidate, you may need to edit HTTPProxy object to conform to the upgraded schema.

* The per-route prefix rewrite key, `prefixRewrite` has been removed.
  See [#899][5] for the status of its replacement.

* The per-service health check key, `healthcheck` has moved to per-route and has been renamed `healthCheckPolicy`.

<table class="table table-borderless" style="border: none;">
<tr><th>Before:</th><th>After:</th></tr>

<tr>
<td><pre><code class="language-yaml" data-lang="yaml">
spec:
  routes:
  - conditions:
    - prefix: /
    services:
    - name: www
      port: 80
      healthcheck:
      - path: /healthy
        intervalSeconds: 5
        timeoutSeconds: 2
        unhealthyThresholdCount: 3
        healthyThresholdCount: 5
</code></pre></td>

<td>
<pre><code class="language-yaml" data-lang="yaml">
spec:
  routes:
  - conditions:
    - prefix: /
    healthCheckPolicy:
    - path: /healthy
      intervalSeconds: 5
      timeoutSeconds: 2
      unhealthyThresholdCount: 3
      healthyThresholdCount: 5
    services:
    - name: www
      port: 80
</code></pre></td>

</tr>
</table>

* The per-service load balancer strategy key, `strategy` has moved to per-route and has been renamed `loadBalancerPolicy`.

<table class="table table-borderless" style="border: none;">
<tr><th>Before:</th><th>After:</th></tr>

<tr>
<td><pre><code class="language-yaml" data-lang="yaml">
spec:
  routes:
  - conditions:
    - prefix: /
    services:
    - name: www
      port: 80
      stategy: WeightedLeastRequest
</code></pre></td>

<td><pre><code class="language-yaml" data-lang="yaml">
spec:
  routes:
  - conditions:
    - prefix: /
    loadBalancerPolicy:
      strategy: WeightedLeastRequest
    services:
    - name: www
      port: 80
</code></pre></td>

</tr>
</table>

#### Check for Contour namespace change

As part of sunsetting the Heptio brand the `heptio-contour` namespace has been renamed to `projectcontour`.
Contour assumes it will be deployed into the `projectcontour` namespace.

If you deploy Contour into a different namespace you will need to pass `contour bootstrap --namespace=<namespace>` and update the leader election parameters in the [`contour.yaml` configuration][6]
as appropriate.

### Split deployment/daemonset now the default

We have changed the example installation to use a separate pod installation, where Contour is in a Deployment and Envoy is in a Daemonset.
Separated pod installations separate the lifecycle of Contour and Envoy, increasing operability.
Because of this, we are marking the single pod install type as officially deprecated.
If you are still running a single pod install type, please review the [`contour` example][7] and either adapt it or use it directly.

### Verify leader election

Contour 1.0.0 enables leader election by default.
No specific configuration is required if you are using the [example deployment][7].

Leader election requires that Contour have write access to a ConfigMap
called `leader-elect` in the project-contour namespace.
This is done with the [contour-leaderelection Role][8] in the [example RBAC][9].
The namespace and name of the configmap are configurable via the configuration file.

The leader election mechanism no longer blocks serving of gRPC until an instance becomes the leader.
Leader election controls writing status back to Contour CRDs (like HTTPProxy and IngressRoute) so that only one Contour pod writes status at a time.

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
If you use this feature please update your deployments.

# Upgrading Contour 0.14.x to 0.15.3

Contour 0.15.3 requires changes to your deployment manifests to explicitly opt in, or opt out of, secure communication between Contour and Envoy.

Contour 0.15.3 also adds experimental support for leader election which may be useful for installations which have split their Contour and Envoy containers into separate pods.
A configuration we call _split deployment_.

## Breaking change

Contour's `contour serve` now requires that either TLS certificates be available, or you supply the `--insecure` parameter.

**If you do not supply TLS details or `--insecure`, `contour serve` will not start.**

## Recommended Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.11.2`.

Please see the [Envoy Release Notes][10] for information about issues fixed in Envoy 1.11.2.

## The easy way to upgrade

If the following are true for you:

 * Your installation is in the `heptio-contour` namespace.
 * You are using one of the [example][11] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 0.15.3 is to delete the `heptio-contour` namespace and reapply one of the example configurations.
From the root directory of the repository:

```
$ kubectl delete namespace heptio-contour
$ kubectl apply -f examples/<your-desired-deployment>
```

If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

**Note:** If you deployed Contour into a different namespace than heptio-contour with a standard example, please delete that namespace.
Then in your editor of choice do a search and replace for `heptio-contour` and replace it with your preferred name space and apply the updated manifest.

## The less easy way

This section contains information for administrators who wish to apply the Contour 0.14.x to 0.15.3 changes manually.

### Upgrade to Contour 0.15.3

Due to the sun setting on the Heptio brand, from v0.15.0 onwards our images are now served from the docker hub repository [`docker.io/projectcontour/contour`][13]

Change the Contour image version to `docker.io/projectcontour/contour:v0.15.3`.

### Enabling TLS for gRPC

You *must* either enable TLS for gRPC serving, or put `--insecure` into your `contour serve` startup line.
If you are running with both Contour and Envoy in a single pod, the existing deployment examples have already been updated with this change.

If you are running using the `ds-hostnet-split` example or a derivative, we strongly recommend that you generate new certificates for securing your gRPC communication between Contour and Envoy.

There is a Job in the `ds-hostnet-split` directory that will use the new `contour certgen` command to generate a CA and then sign Contour and Envoy keypairs, which can also then be saved directly to Kubernetes as Secrets, ready to be mounted into your Contour and Envoy Deployments and Daemonsets.

If you would like more detail, see [grpc-tls-howto.md][14], which explains your options.

### Upgrade to Envoy 1.11.2

Contour 0.15.3 requires Envoy 1.11.2. Change the Envoy image version to `docker.io/envoyproxy/envoy:v1.11.2`.

### Enabling Leader Election

Contour 0.15.3 adds experimental support for leader election.
Enabling leader election will mean that only one of the Contour pods will actually serve gRPC traffic.
This will ensure that all Envoy's take their configuration from the same Contour.
You can enable leader election with the `--enable-leader-election` flag to `contour serve`.

If you have deployed Contour and Envoy in their own pods--we call this split deployment--you should enable leader election so all envoy pods take their configuration from the lead contour.

To enable leader election, the following must be true for you:

- You are running in a split Contour and Envoy setup.
  That is, there are separate Contour and Envoy pod(s).

In order for leader election to work, you must make the following changes to your setup:

- The Contour Deployment must have its readiness probe changed too TCP readiness probe configured to check port 8001 (the gRPC port), as non-leaders will not serve gRPC, and Envoys may not be properly configured if they attempt to connect to a non-leader Contour.
  That is, you will need to change:

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
- The update strategy for the Contour deployment must be changed to `Recreate` instead of `RollingUpdate`, as pods will never become Ready (since they won't pass the readiness probe).
  Add

```
  strategy:
    type: Recreate
```
to the top level of the Pod spec.
- Leader election is currently hard-coded to use a ConfigMap named `contour` in this namespace for the leader election lock.
If you are using a newer installation of Contour, this may be present already, if not, the leader election library will create an empty ConfigMap for you.

Once these changes are made, add `--enable-leader-election` to your `contour serve` command.
The leader will perform and log its operations as normal, and the non-leaders will block waiting to become leader.
You can inspect the state of the leadership using

```
$ kubectl describe configmap -n heptio-contour contour
```

and checking the annotations that store exact details using

```
$ kubectl get configmap -n heptio-contour -o yaml contour
```

[1]: {{site.github.repository_url}}/blob/{{site.latest}}/examples
[3]: /docs/{{site.latest}}/httpproxy
[4]: /docs/{{site.latest}}/annotations
[5]: {{site.github.repository_url}}/issues/899
[6]: /docs/{{site.latest}}/configuration
[7]: {{site.github.repository_url}}/blob/{{site.latest}}/examples/contour/README.md
[8]: {{site.github.repository_url}}/blob/{{site.latest}}/examples/contour/02-rbac.yaml#L71
[9]: {{site.github.repository_url}}/blob/{{site.latest}}/examples/contour/02-rbac.yaml
[10]: https://www.envoyproxy.io/docs/envoy/v1.11.2/intro/version_history
[11]: {{site.github.repository_url}}/blob/v0.15.3/examples/
[12]: /docs/{{site.latest}}/deploy-options
[13]: https://hub.docker.com/r/projectcontour/contour
[14]: /docs/{{site.latest}}/grpc-tls-howto
[15]: https://www.envoyproxy.io/docs/envoy/v1.12.2/intro/version_history
[16]: {% link getting-started.md %}
