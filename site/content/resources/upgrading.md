---
title: Upgrading Contour
layout: page
---

<!-- NOTE: this document should be formatted with one sentence per line to made reviewing easier. -->

This document describes the changes needed to upgrade your Contour installation.

<div id="toc" class="navigation"></div>

# Before you start

Contour currently only tests sequential upgrades, i.e. without skipping any minor or patch versions.
This approach is recommended for users in order to minimize downtime and avoid any potential issues.
If you choose to skip versions while upgrading, please note that this may lead to additional downtime.

# Known issues

1. Envoy pod stuck in pending state

    If Envoy is deployed with a Deployment and the number of envoy instances is not less than number of kubernetes nodes in the clusters, during rolling upgrade, new envoy pod will be stuck in pending stage because old envoy pod is occupying host port.

    Workaround: Delete the envoy instance of older version manually. This will cause a little bit of downtime but it's pretty short.

# The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ export CONTOUR_VERSION=<desired version, e.g. v1.24.0>
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/$CONTOUR_VERSION/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

# The less easy way

This section contains information for administrators who wish to upgrade the Contour resources one-by-one.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the tag for the target version.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

# Legacy per-version upgrade instructions

Contour previously published per-version upgrade instructions which are retained below for posterity.
These will no longer be updated going forward, as the instructions were largely redundant between versions.

## Upgrading Contour 1.23.2 to 1.24.0

Contour 1.24.0 is the current stable release.

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.25.0`.

Please see the [Envoy Release Notes][46] for information about the changes included in Envoy 1.25.0.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.24.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.24.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.23.2 to 1.24.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.24.0` tag.

If your version of Contour is older than v1.23.2, please upgrade to v1.23.2 first, then upgrade to v1.24.0.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.23.1 to 1.23.2

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.24.1`.

Please see the [Envoy Release Notes][45] for information about the changes included in Envoy 1.24.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.23.2 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.23.2/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.23.1 to 1.23.2 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.23.2` tag.

If your version of Contour is older than v1.23.1, please upgrade to v1.23.1 first, then upgrade to v1.23.2.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.23.0 to 1.23.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.24.1`.

Please see the [Envoy Release Notes][45] for information about the changes included in Envoy 1.24.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.23.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.23.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.23.0 to 1.23.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.23.1` tag.

If your version of Contour is older than v1.23.0, please upgrade to v1.23.0 first, then upgrade to v1.23.1.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.22.3 to 1.23.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.24.0`.

Please see the [Envoy Release Notes][42] for information about the changes included in Envoy 1.24.0.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.23.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.23.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.22.3 to 1.23.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.23.0` tag.

If your version of Contour is older than v1.22.3, please upgrade to v1.22.3 first, then upgrade to v1.23.0.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.22.2 to 1.22.3

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.23.3`.

Please see the [Envoy Release Notes][44] for information about the changes included in Envoy 1.23.3.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.22.3 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.22.3/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.22.2 to 1.22.3 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.22.3` tag.

If your version of Contour is older than v1.22.2, please upgrade to v1.22.2 first, then upgrade to v1.22.3.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.22.1 to 1.22.2

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.23.3`.

Please see the [Envoy Release Notes][44] for information about the changes included in Envoy 1.23.3.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.22.2 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.22.2/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.22.1 to 1.22.2 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.22.2` tag.

If your version of Contour is older than v1.22.1, please upgrade to v1.22.1 first, then upgrade to v1.22.2.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.22.0 to 1.22.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.23.1`.

Please see the [Envoy Release Notes][41] for information about the changes included in Envoy 1.23.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.22.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.22.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.22.0 to 1.22.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.22.1` tag.

If your version of Contour is older than v1.22.0, please upgrade to v1.22.0 first, then upgrade to v1.22.1.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.21.3 to 1.22.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.23.0`.

Please see the [Envoy Release Notes][40] for information about the changes included in Envoy 1.23.0.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.22.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.22.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.21.3 to 1.22.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.22.0` tag.

If your version of Contour is older than v1.21.3, please upgrade to v1.21.3 first, then upgrade to v1.22.0.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.21.2 to 1.21.3

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.22.6`.

Please see the [Envoy Release Notes][43] for information about the changes included in Envoy 1.22.6.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.21.3 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.21.3/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.21.2 to 1.21.3 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.21.3` tag.

If your version of Contour is older than v1.21.2, please upgrade to v1.21.2 first, then upgrade to v1.21.3.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.21.1 to 1.21.2

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.22.6`.

Please see the [Envoy Release Notes][43] for information about the changes included in Envoy 1.22.6.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.21.2 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.21.2/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.21.1 to 1.21.2 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.21.2` tag.

If your version of Contour is older than v1.21.1, please upgrade to v1.21.1 first, then upgrade to v1.21.2.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```


## Upgrading Contour 1.21.0 to 1.21.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.22.2`.

Please see the [Envoy Release Notes][38] for information about the changes included in Envoy 1.22.2.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.21.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.21.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.21.0 to 1.21.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.21.1` tag.

If your version of Contour is older than v1.21.0, please upgrade to v1.21.0 first, then upgrade to v1.21.1.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.20.2 to 1.21.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.22.0`.

Please see the [Envoy Release Notes][37] for information about the changes included in Envoy 1.22.0.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.21.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.21.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.20.2 to 1.21.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.21.0` tag.

If your version of Contour is older than v1.20.2, please upgrade to v1.20.2 first, then upgrade to v1.21.0.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour RBAC resources:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.20.1 to 1.20.2

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.21.3`.

Please see the [Envoy Release Notes][39] for information about issues fixed in Envoy 1.21.3.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.20.2 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.20.2/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.20.1 to 1.20.2 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.20.2` tag.

If your version of Contour is older than v1.20.1, please upgrade to v1.20.1 first, then upgrade to v1.20.2.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour cluster role:

    ```bash
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```


## Upgrading Contour 1.20.0 to 1.20.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.21.1`.

Please see the [Envoy Release Notes][36] for information about issues fixed in Envoy 1.21.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.20.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.20.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.20.0 to 1.20.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.20.1` tag.

If your version of Contour is older than v1.20.0, please upgrade to v1.20.0 first, then upgrade to v1.20.1.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour cluster role:

    ```bash
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.19.1 to 1.20.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.21.0`.

Please see the [Envoy Release Notes][35] for information about issues fixed in Envoy 1.21.0.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.20.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.20.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.19.1 to 1.20.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.20.0` tag.

If your version of Contour is older than v1.19.1, please upgrade to v1.19.1 first, then upgrade to v1.20.0.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour cluster role:

    ```bash
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.19.0 to 1.19.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.19.1`.

Please see the [Envoy Release Notes][34] for information about issues fixed in Envoy 1.19.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.19.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.19.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.19.0 to 1.19.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.19.1` tag.

If your version of Contour is older than v1.19.0, please upgrade to v1.19.0 first, then upgrade to v1.19.1.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour cluster role:

    ```bash
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.18.3 to 1.19.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.19.1`.

Please see the [Envoy Release Notes][34] for information about issues fixed in Envoy 1.19.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.19.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.19.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.18.3 to 1.19.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.19.0` tag.

If your version of Contour is older than v1.18.3, please upgrade to v1.18.3 first, then upgrade to v1.19.0.

1. Update the Contour CRDs:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update the Contour cluster role:

    ```bash
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. Upgrade the Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.18.2 to 1.18.3

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.19.1`.

Please see the [Envoy Release Notes][34] for information about issues fixed in Envoy 1.19.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.18.3 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.18.3/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.18.2 to 1.18.3 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.18.3` tag.

If your version of Contour is older than v1.18.2, please upgrade to v1.18.2 first, then upgrade to v1.18.3.

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update your RBAC definitions:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    ```

1. Upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```


## Upgrading Contour 1.18.1 to 1.18.2

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.19.1`.

Please see the [Envoy Release Notes][34] for information about issues fixed in Envoy 1.19.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.18.2 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.18.2/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.18.1 to 1.18.2 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.18.2` tag.

If your version of Contour is older than v1.18.1, please upgrade to v1.18.1 first, then upgrade to v1.18.2.

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update your RBAC definitions:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    ```

1. Upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.18.0 to 1.18.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.19.1`.

Please see the [Envoy Release Notes][34] for information about issues fixed in Envoy 1.19.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.18.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.18.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.18.0 to 1.18.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.18.1` tag.

If your version of Contour is older than v1.18.0, please upgrade to v1.18.0 first, then upgrade to v1.18.1.

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update your RBAC definitions:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    ```

1. Upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```


## Upgrading Contour 1.17.1 to 1.18.0

**If you utilize ExternalName services in your cluster, please note that this release disables Contour processing such services by default.**
**Please see [this CVE](https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc) for context and the [1.18.0 release notes](https://github.com/projectcontour/contour/releases/tag/v1.18.0).**

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.19.0`.

Please see the [Envoy Release Notes][33] for information about issues fixed in Envoy 1.19.0.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.18.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.18.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.17.1 to 1.18.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.18.0` tag.

If your version of Contour is older than v1.17.1, please upgrade to v1.17.1 first, then upgrade to v1.18.0.

1. The Contour CRD definitions must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update your RBAC definitions:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    ```

1. Upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.17.0 to 1.17.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.18.3`.

Please see the [Envoy Release Notes][32] for information about issues fixed in Envoy 1.18.3.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.17.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.17.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.17.0 to 1.17.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.17.1` tag.

If your version of Contour is older than v1.17.0, please upgrade to v1.17.0 first, then upgrade to v1.17.1.

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update your RBAC definitions:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    ```

1. Upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.16.0 to 1.17.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.18.3`.

Please see the [Envoy Release Notes][32] for information about issues fixed in Envoy 1.18.3.

### The easiest way to upgrade (alpha)
For existing Contour Operator users, complete the following steps to upgrade Contour:

- Verify the operator is running v1.16.0, and it's deployment status is "Available=True".
- Verify the status of all Contour custom resources are "Available=True".
- Update the operator's image to v1.17.0:
   ```bash
   $ kubectl patch deploy/contour-operator -n contour-operator -p '{"spec":{"template":{"spec":{"containers":[{"name":"contour-operator","image":"docker.io/projectcontour/contour-operator:v1.17.0"}]}}}}'
   ```
- The above command will upgrade the operator. After the operator runs the new version, it will upgrade Contour.
- Verify the operator and Contour are running the new version.
- Verify all Contour custom resources are "Available=True".

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.17.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.17.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.16.0 to 1.17.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.17.0` tag.

If your version of Contour is older than v1.16.0, please upgrade to v1.16.0 first, then upgrade to v1.17.0.

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in a format compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Update your RBAC definitions:

    ```bash
    $ kubectl apply -f examples/contour/02-rbac.yaml
    ```

1. Upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```


## Upgrading Contour 1.15.1 to 1.16.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.18.3`.

Please see the [Envoy Release Notes][32] for information about issues fixed in Envoy 1.18.3.

### The easiest way to upgrade (alpha)
For existing Contour Operator users, complete the following steps to upgrade Contour:

- Verify the operator is running v1.15.1, and it's deployment status is "Available=True".
- Verify the status of all Contour custom resources are "Available=True".
- Update the operator's image to v1.16.0:
   ```bash
   $ kubectl patch deploy/contour-operator -n contour-operator -p '{"spec":{"template":{"spec":{"containers":[{"name":"contour-operator","image":"docker.io/projectcontour/contour-operator:v1.16.0"}]}}}}'
   ```
- The above command will upgrade the operator. After the operator runs the new version, it will upgrade Contour.
- Verify the operator and Contour are running the new version.
- Verify all Contour custom resources are "Available=True".

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.16.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.16.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.15.1 to 1.16.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.16.0` tag.

If your version of Contour is older than v1.15.1, please upgrade to v1.15.1 first, then upgrade to v1.16.0.

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
   This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.15.0 to 1.15.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.18.3`.

Please see the [Envoy Release Notes][32] for information about issues fixed in Envoy 1.18.3.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.15.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.15.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.15.0 to 1.15.01 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.15.1` tag.

If your version of Contour is older than v1.15.0, please upgrade to v1.15.0 first, then upgrade to v1.15.1.

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. Upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

1. Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

## Upgrading Contour 1.14.1 to 1.15.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.18.2`.

Please see the [Envoy Release Notes][31] for information about issues fixed in Envoy 1.18.2.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.15.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.15.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.14.1 to 1.15.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.15.0` tag.

If your version of Contour is older than v1.14, please upgrade to v1.14 first, then upgrade to v1.15.0.

1. The Contour CRD definitions must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

    ```bash
    $ kubectl apply -f examples/contour/01-crds.yaml
    ```

1. Users of the example deployment should reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

    ```bash
    $ kubectl apply -f examples/contour/02-job-certgen.yaml
    ```

1. This release includes an update to RBAC rules. Update the Contour ClusterRole with the following:

    ```bash
    $ kubectl apply -f examples/contour/02-role-contour.yaml
    ```

1. This release includes changes to support Ingress wildcard hosts that require Envoy to be upgraded *before* Contour. Update the Envoy DaemonSet:

    ```bash
    $ kubectl apply -f examples/contour/03-envoy.yaml
    ```

1. Once the Envoy DaemonSet has finished updating, upgrade your Contour deployment:

    ```bash
    $ kubectl apply -f examples/contour/03-contour.yaml
    ```

## Upgrading Contour 1.14.0 to 1.14.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.17.2`.

Please see the [Envoy Release Notes][30] for information about issues fixed in Envoy 1.17.2.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.14.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.14.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.14.0 to 1.14.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.14.1` tag.

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

If your version of Contour is older than v1.14, please upgrade to v1.14 first, then upgrade to v1.14.1.

```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

Upgrade your Contour deployment:

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
```

Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

```bash
$ kubectl apply -f examples/contour/03-envoy.yaml
```

## Upgrading Contour 1.13.1 to 1.14.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.17.1`.

Please see the [Envoy Release Notes][29] for information about issues fixed in Envoy 1.17.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.14.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.14.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.13.1 to 1.14.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.14.0` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

If your version of Contour is older than v1.13, please upgrade to v1.13 first, then upgrade to v1.14.0.

This release includes an update to the Envoy service ports. Upgrade your Envoy service with the following:

```bash
$ kubectl apply -f examples/contour/02-service-envoy.yaml
```

Upgrade your Contour deployment:

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
```

Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

```bash
$ kubectl apply -f examples/contour/03-envoy.yaml
```

## Upgrading Contour 1.12.0 to 1.13.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.17.1`.

Please see the [Envoy Release Notes][29] for information about issues fixed in Envoy 1.17.1.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.13.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.13.1/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.12.0 to 1.13.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.13.1` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

If your version of Contour is older than v1.12, please upgrade to v1.12 first, then upgrade to v1.13.1.

Upgrade your Contour deployment:

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
```

Once the Contour deployment has finished upgrading, update the Envoy DaemonSet:

```bash
$ kubectl apply -f examples/contour/03-envoy.yaml
```

## Upgrading Contour 1.11.0 to 1.12.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.17.0`.

Please see the [Envoy Release Notes][28] for information about issues fixed in Envoy 1.17.0.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.12.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.12.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.11.0 to 1.12.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.12.0` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

If your version of Contour is older than v1.11, please upgrade to v1.11 first, then upgrade to v1.12.

Upgrade your Contour deployment:

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
```

Note that the Contour deployment needs to be updated before the Envoy daemon set since it contains backwards-compatible changes that are required in order to work with Envoy 1.17.0.
Once the Contour deployment has finished upgrading, update the Envoy daemon set:

```bash
$ kubectl apply -f examples/contour/03-envoy.yaml
```

## Upgrading Contour 1.10.0 to 1.11.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.16.2`.

Please see the [Envoy Release Notes][27] for information about issues fixed in Envoy 1.16.2.

### The easy way to upgrade

If the following are true for you:

* Your installation is in the `projectcontour` namespace.
* You are using our [quickstart example][18] deployments.
* Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.11.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.11.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.10.0 to 1.11.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.11.0` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

If your version of Contour is older than v1.10, please upgrade to v1.10 first, then upgrade to v1.11.
For more information, see the [xDS Migration Guide][26].

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
$ kubectl apply -f examples/contour/03-envoy.yaml
```

## Upgrading Contour 1.9.0 to 1.10.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.16.0`.

Please see the [Envoy Release Notes][25] for information about issues fixed in Envoy 1.16.0.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.10.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.10.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.9.0 to 1.10.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.10.0` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

If your cluster cannot take downtime, it's important to first upgrade Contour to v1.10.0 then upgrade your Envoy pods.
This is due to an Envoy xDS Resource API upgrade to `v3`.
See the [xDS Migration Guide][26] for more information.

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
$ kubectl apply -f examples/contour/03-envoy.yaml
```

## Upgrading Contour 1.8.2 to 1.9.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.15.1`.

Please see the [Envoy Release Notes][23] for information about issues fixed in Envoy 1.15.1.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.9.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.9.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.8.2 to 1.9.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.9.0` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.
This will rotate the TLS certificates used for gRPC security.

```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

### Removing the IngressRoute CRDs

As a reminder, support for `IngressRoute` was officially dropped in v1.6.
If you haven't already migrated to `HTTPProxy`, see [the IngressRoute to HTTPProxy migration guide][24] for instructions on how to do so.
Once you have migrated, delete the `IngressRoute` and related CRDs:

```bash
$ kubectl delete crd ingressroutes.contour.heptio.com
$ kubectl delete crd tlscertificatedelegations.contour.heptio.com
```

## Upgrading Contour 1.7.0 to 1.8.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.15.0`.

Please see the [Envoy Release Notes][23] for information about issues fixed in Envoy 1.15.0.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take a few minutes of downtime.

Then the simplest way to upgrade to 1.8.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.8.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.7.0 to 1.8.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.8.0` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets. This will rotate the TLS certificates used for gRPC security.


```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

### Removing the IngressRoute CRDs

As a reminder, support for `IngressRoute` was officially dropped in v1.6.
If you haven't already migrated to `HTTPProxy`, see [the IngressRoute to HTTPProxy migration guide][24] for instructions on how to do so.
Once you have migrated, delete the `IngressRoute` and related CRDs:

```bash
$ kubectl delete crd ingressroutes.contour.heptio.com
$ kubectl delete crd tlscertificatedelegations.contour.heptio.com
```

## Upgrading Contour 1.6.1 to 1.7.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.15.0`.

Please see the [Envoy Release Notes][23] for information about issues fixed in Envoy 1.15.0.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.7.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.7.0/contour.yaml
```

This will remove the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.6.1 to 1.7.0 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.7.0` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets. This will rotate the TLS certs used for gRPC security.


```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

To consume the new Secrets, reapply the Envoy Daemonset and the Contour Deployment YAML.
All the Pods will gracefully restart and reconnect using the new TLS Secrets.
After this, the gRPC session between Contour and Envoy can be re-keyed by regenerating the Secrets.

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
$ kubectl apply -f examples/contour/03-envoy.yaml
```

### Removing the IngressRoute CRDs

As a reminder, support for `IngressRoute` was officially dropped in v1.6.
If you haven't already migrated to `HTTPProxy`, see [the IngressRoute to HTTPProxy migration guide][24] for instructions on how to do so.
Once you have migrated, delete the `IngressRoute` and related CRDs:

```bash
$ kubectl delete crd ingressroutes.contour.heptio.com
$ kubectl delete crd tlscertificatedelegations.contour.heptio.com
```

## Upgrading Contour 1.5.1 to 1.6.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.14.3`.

Please see the [Envoy Release Notes][22] for information about issues fixed in Envoy 1.14.3.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.6.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete crd ingressroutes.contour.heptio.com
$ kubectl delete crd tlscertificatedelegations.contour.heptio.com
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.6.1/contour.yaml
```

This will remove the IngressRoute CRD, and both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.5.1 to 1.6.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.6.1` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

Administrators should also remove the IngressRoute CRDs:
```bash
$ kubectl delete crd ingressroutes.contour.heptio.com
$ kubectl delete crd tlscertificatedelegations.contour.heptio.com
```

Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets. This will rotate the TLS certs used for gRPC security.


```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

To consume the new Secrets, reapply the Envoy Daemonset and the Contour Deployment YAML.
All the Pods will gracefully restart and reconnect using the new TLS Secrets.
After this, the gRPC session between Contour and Envoy can be re-keyed by regenerating the Secrets.

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
$ kubectl apply -f examples/contour/03-envoy.yaml
```

If you are upgrading from Contour 1.6.0, the only required change is to upgrade the version of the Envoy image version from `v1.14.2` to `v1.14.3`.
The Contour image can optionally be upgraded to `v1.6.1`.


## Upgrading Contour 1.4.0 to 1.5.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.14.2`.

Please see the [Envoy Release Notes][21] for information about issues fixed in Envoy 1.14.2.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.5.1 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.5.1/contour.yaml
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.4.0 to 1.5.1 changes manually.
The YAML files referenced in this section can be found by cloning the Contour repository and checking out the `v1.5.1` tag.

The Contour CRD definition must be re-applied to the cluster, since a number of compatible changes and additions have been made to the Contour API:

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

In this release, the format of the TLS Secrets that are used to secure the gRPC session between Envoy and Contour has changed.
This means that the Envoy Daemonset and the Contour Deployment have been changed to mount the TLS secrets volume differently.
Users of the example deployment should first reapply the certgen Job YAML which will re-generate the relevant Secrets in the new format, which is compatible with [cert-manager](https://cert-manager.io) TLS secrets.


```bash
$ kubectl apply -f examples/contour/02-job-certgen.yaml
```

To consume the new Secrets, reapply the Envoy Daemonset and the Contour Deployment YAML.
All the Pods will gracefully restart and reconnect using the new TLS Secrets.
After this, the gRPC session between Contour and Envoy can be re-keyed by regenerating the Secrets.

```bash
$ kubectl apply -f examples/contour/03-contour.yaml
$ kubectl apply -f examples/contour/03-envoy.yaml
```

Users who secure the gRPC session with their own certificate may need to modify the Envoy Daemonset and the Contour Deployment to ensure that their Secrets are correctly mounted within the corresponding Pod containers.
When making these changes, be sure to retain the `--resources-dir` flag to the `contour bootstrap` command so that Envoy will be configured with reloadable TLS certificate support.

## Upgrading Contour 1.3.0 to 1.4.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.14.1`.

Please see the [Envoy Release Notes][20] for information about issues fixed in Envoy 1.14.1.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.4.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.4.0/contour.yaml
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

**Note:** If you deployed Contour into a different namespace than `projectcontour` with a standard example, please delete that namespace.
Then in your editor of choice do a search and replace for `projectcontour` and replace it with your preferred name space and apply the updated manifest.

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.3.0 to 1.4.0 changes manually.

#### Upgrade to Contour 1.4.0

Change the Contour image version to `docker.io/projectcontour/contour:v1.4.0`

Because there has been a change to Envoy to add a serviceaccount, you need to reapply the Contour CRDs and RBAC.

From within a clone of the repo, checkout `release-1.4`, then you can:

```bash
kubectl apply -f examples/contour/00-common.yaml
kubectl apply -f examples/contour/01-crds.yaml
kubectl apply -f examples/contour/02-rbac.yaml
```

If you are using our Envoy daemonset:

```bash
kubectl apply -f examples/contour/03-envoy.yaml
```

Otherwise, you should add the new `envoy` `serviceAccount` to your Envoy deployment.
This will be used in the future to add further container-level security via PodSecurityPolicies.

## Upgrading Contour 1.2.1 to 1.3.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.13.1`.

Please see the [Envoy Release Notes][17] for information about issues fixed in Envoy 1.13.1.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.3.0 is to delete the `projectcontour` namespace and reapply one of the example configurations:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.3.0/contour.yaml
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

**Note:** If you deployed Contour into a different namespace than `projectcontour` with a standard example, please delete that namespace.
Then in your editor of choice do a search and replace for `projectcontour` and replace it with your preferred name space and apply the updated manifest.

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.2.1 to 1.3.0 changes manually.

#### Upgrade to Contour 1.3.0

Change the Contour image version to `docker.io/projectcontour/contour:v1.3.0`

## Upgrading Contour 1.2.0 to 1.2.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.13.1`.

Please see the [Envoy Release Notes][17] for information about issues fixed in Envoy 1.13.1.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.2.1 is to delete the `projectcontour` namespace and reapply one of the example configurations.
From the root directory of the repository:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.2.1/contour.yaml
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

**Note:** If you deployed Contour into a different namespace than `projectcontour` with a standard example, please delete that namespace.
Then in your editor of choice do a search and replace for `projectcontour` and replace it with your preferred name space and apply the updated manifest.

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.2.0 to 1.2.1 changes manually.

#### Upgrade to Contour 1.2.1

Change the Contour image version to `docker.io/projectcontour/contour:v1.2.1`.

#### Upgrade to Envoy 1.13.1

Contour 1.2.1 requires Envoy 1.13.1.
Change the Envoy image version to `docker.io/envoyproxy/envoy:v1.13.1`.

_Note: Envoy 1.13.1 includes fixes to a number of [CVEs][19]_

## Upgrading Contour 1.1.0 to 1.2.1

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.13.1`.

Please see the [Envoy Release Notes][17] for information about issues fixed in Envoy 1.13.1.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using our [quickstart example][18] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.2.1 is to delete the `projectcontour` namespace and reapply one of the example configurations.
From the root directory of the repository:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.2.1/contour.yaml
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

**Note:** If you deployed Contour into a different namespace than `projectcontour` with a standard example, please delete that namespace.
Then in your editor of choice do a search and replace for `projectcontour` and replace it with your preferred name space and apply the updated manifest.

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.1.0 to 1.2.1 changes manually.

#### Upgrade to Contour 1.2.1

Change the Contour image version to `docker.io/projectcontour/contour:v1.2.1`.

#### Upgrade to Envoy 1.13.1

Contour 1.2.1 requires Envoy 1.13.1.
Change the Envoy image version to `docker.io/envoyproxy/envoy:v1.13.0`.

#### Envoy shutdown manager

Contour 1.2.1 introduces a new sidecar to aid graceful shutdown of the Envoy pod.
Consult [shutdown manager]({% link docs/v1.2.1/redeploy-envoy.md %}) documentation for installation instructions.

## Upgrading Contour 1.0.1 to 1.1.0

### Required Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.12.2`.

Please see the [Envoy Release Notes][15] for information about issues fixed in Envoy 1.12.2.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `projectcontour` namespace.
 * You are using one of the [example][1] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 1.1.0 is to delete the `projectcontour` namespace and reapply one of the example configurations.
From the root directory of the repository:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f examples/<your-desired-deployment>
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

**Note:** If you deployed Contour into a different namespace than `projectcontour` with a standard example, please delete that namespace.
Then in your editor of choice do a search and replace for `projectcontour` and replace it with your preferred name space and apply the updated manifest.

**Note:** If you are deploying to a cluster where you have previously installed alpha versions of the Contour API, applying the Contour CRDs in `examples/contour` may fail with a message similar to `Invalid value: "v1alpha1": must appear in spec.versions`. In this case, you need to delete the old CRDs and apply the new ones.

```bash
$ kubectl delete namespace projectcontour
$ kubectl get crd  | awk '/projectcontour.io|contour.heptio.com/{print $1}' | xargs kubectl delete crd
$ kubectl apply -f examples/<your-desired-deployment>
```

### The less easy way

This section contains information for administrators who wish to apply the Contour 1.0.1 to 1.1.0 changes manually.

#### Upgrade to Contour 1.1.0

Change the Contour image version to `docker.io/projectcontour/contour:v1.1.0`.

#### Upgrade to Envoy 1.12.2

Contour 1.1.0 requires Envoy 1.12.2. Change the Envoy image version to `docker.io/envoyproxy/envoy:v1.12.2`.

## Upgrading Contour 1.0.0 to 1.0.1

### The easy way to upgrade

If you are running Contour 1.0.0, the easy way to upgrade to Contour 1.0.1 is to reapply the [quickstart yaml][16].

```bash
$ kubectl apply -f {{< param base_url >}}/quickstart/v1.0.1/contour.yaml
```

### The less easy way

This section contains information for administrators who wish to manually upgrade from Contour 1.0.0 to Contour 1.0.1.

#### Contour version

Ensure the Contour image version is `docker.io/projectcontour/contour:v1.0.1`.

#### Envoy version

Ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.12.2`.

Please see the [Envoy Release Notes][15] for information about issues fixed in Envoy 1.12.2.

## Upgrading Contour 0.15.3 to 1.0.0

### Required Envoy version

The required version of Envoy remains unchanged.
Ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.11.2`.

### The easy way to upgrade

If the following are true for you:

 * Your previous installation is in the `projectcontour` namespace.
 * You are using one of the [example][2] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade is to delete the `projectcontour` namespace and reapply the `examples/contour` sample manifest.
From the root directory of the repository:

```bash
$ kubectl delete namespace projectcontour
$ kubectl apply -f examples/contour
```

This will remove both the Envoy and Contour pods from your cluster and recreate them with the updated configuration.
If you're using a `LoadBalancer` Service, deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

### The less easy way

This section contains information for administrators who wish to manually upgrade from Contour 0.15.3 to Contour 1.0.0.

#### Upgrade to Contour 1.0.0

Change the Contour image version to `docker.io/projectcontour/contour:v1.0.0`.

Note that as part of sunsetting the Heptio brand, Contour Docker images have moved from `gcr.io/heptio-images` to `docker.io/projectcontour`.

#### Reapply HTTPProxy and IngressRoute CRD definitions

Contour 1.0.0 ships with updated OpenAPIv3 validation schemas.

Contour 1.0.0 promotes the HTTPProxy CRD to v1.
HTTPProxy is now considered stable, and there will only be additive, compatible changes in the future.
See the [HTTPProxy documentation][3] for more information.

```bash
$ kubectl apply -f examples/contour/01-crds.yaml
```

#### Update deprecated `contour.heptio.com` annotations

All the annotations with the prefix `contour.heptio.com` have been migrated to their respective `projectcontour.io` counterparts.
The deprecated `contour.heptio.com` annotations will be recognized through the Contour 1.0 release, but are scheduled to be removed after Contour 1.0.

See the [annotation documentation][4] for more information.

#### Update old `projectcontour.io/v1alpha1` group versions

If you are upgrading a cluster that you previously installed a 1.0.0 release candidate, note that Contour 1.0.0 moves the HTTPProxy CRD from `projectcontour.io/v1alpha1` to `projectcontour.io/v1` and will no longer recognize the former group version.

Please edit your HTTPProxy documents to update their group version to `projectcontour.io/v1`.

#### Check for HTTPProxy v1 schema changes

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
      strategy: WeightedLeastRequest
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

##### Check for Contour namespace change

As part of sunsetting the Heptio brand the `heptio-contour` namespace has been renamed to `projectcontour`.
Contour assumes it will be deployed into the `projectcontour` namespace.

If you deploy Contour into a different namespace you will need to pass `contour bootstrap --namespace=<namespace>` and update the leader election parameters in the [`contour.yaml` configuration][6]
as appropriate.

#### Split deployment/daemonset now the default

We have changed the example installation to use a separate pod installation, where Contour is in a Deployment and Envoy is in a Daemonset.
Separated pod installations separate the lifecycle of Contour and Envoy, increasing operability.
Because of this, we are marking the single pod install type as officially deprecated.
If you are still running a single pod install type, please review the [`contour` example][7] and either adapt it or use it directly.

#### Verify leader election

Contour 1.0.0 enables leader election by default.
No specific configuration is required if you are using the [example deployment][7].

Leader election requires that Contour have write access to a ConfigMap
called `leader-elect` in the project-contour namespace.
This is done with the [contour-leaderelection Role][8] in the [example RBAC][9].
The namespace and name of the configmap are configurable via the configuration file.

The leader election mechanism no longer blocks serving of gRPC until an instance becomes the leader.
Leader election controls writing status back to Contour CRDs (like HTTPProxy and IngressRoute) so that only one Contour pod writes status at a time.

Should you wish to disable leader election, pass `contour serve --disable-leader-election`.

#### Envoy pod readiness checks

Update the readiness checks on your Envoy pod's spec to reflect Envoy 1.11.1's `/ready` endpoint
```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8002
```

#### Root namespace restriction

The `contour serve --ingressroute-root-namespaces` flag has been renamed to `--root-namespaces`.
If you use this feature please update your deployments.

## Upgrading Contour 0.14.x to 0.15.3

Contour 0.15.3 requires changes to your deployment manifests to explicitly opt in, or opt out of, secure communication between Contour and Envoy.

Contour 0.15.3 also adds experimental support for leader election which may be useful for installations which have split their Contour and Envoy containers into separate pods.
A configuration we call _split deployment_.

### Breaking change

Contour's `contour serve` now requires that either TLS certificates be available, or you supply the `--insecure` parameter.

**If you do not supply TLS details or `--insecure`, `contour serve` will not start.**

### Recommended Envoy version

All users should ensure the Envoy image version is `docker.io/envoyproxy/envoy:v1.11.2`.

Please see the [Envoy Release Notes][10] for information about issues fixed in Envoy 1.11.2.

### The easy way to upgrade

If the following are true for you:

 * Your installation is in the `heptio-contour` namespace.
 * You are using one of the [example][11] deployments.
 * Your cluster can take few minutes of downtime.

Then the simplest way to upgrade to 0.15.3 is to delete the `heptio-contour` namespace and reapply one of the example configurations.
From the root directory of the repository:

```bash
$ kubectl delete namespace heptio-contour
$ kubectl apply -f examples/<your-desired-deployment>
```

If you're using a `LoadBalancer` Service, (which most of the examples do) deleting and recreating may change the public IP assigned by your cloud provider.
You'll need to re-check where your DNS names are pointing as well, using [Get your hostname or IP address][12].

**Note:** If you deployed Contour into a different namespace than heptio-contour with a standard example, please delete that namespace.
Then in your editor of choice do a search and replace for `heptio-contour` and replace it with your preferred name space and apply the updated manifest.

### The less easy way

This section contains information for administrators who wish to apply the Contour 0.14.x to 0.15.3 changes manually.

#### Upgrade to Contour 0.15.3

Due to the sun setting on the Heptio brand, from v0.15.0 onwards our images are now served from the docker hub repository [`docker.io/projectcontour/contour`][13]

Change the Contour image version to `docker.io/projectcontour/contour:v0.15.3`.

#### Enabling TLS for gRPC

You *must* either enable TLS for gRPC serving, or put `--insecure` into your `contour serve` startup line.
If you are running with both Contour and Envoy in a single pod, the existing deployment examples have already been updated with this change.

If you are running using the `ds-hostnet-split` example or a derivative, we strongly recommend that you generate new certificates for securing your gRPC communication between Contour and Envoy.

There is a Job in the `ds-hostnet-split` directory that will use the new `contour certgen` command to generate a CA and then sign Contour and Envoy keypairs, which can also then be saved directly to Kubernetes as Secrets, ready to be mounted into your Contour and Envoy Deployments and Daemonsets.

If you would like more detail, see [grpc-tls-howto.md][14], which explains your options.

#### Upgrade to Envoy 1.11.2

Contour 0.15.3 requires Envoy 1.11.2. Change the Envoy image version to `docker.io/envoyproxy/envoy:v1.11.2`.

#### Enabling Leader Election

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

```yaml
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8000
```
to

```yaml
        readinessProbe:
          tcpSocket:
            port: 8001
          initialDelaySeconds: 15
          periodSeconds: 10
```
inside the Pod spec.
- The update strategy for the Contour deployment must be changed to `Recreate` instead of `RollingUpdate`, as pods will never become Ready (since they won't pass the readiness probe).
  Add

```yaml
  strategy:
    type: Recreate
```
to the top level of the Pod spec.
- Leader election is currently hard-coded to use a ConfigMap named `contour` in this namespace for the leader election lock.
If you are using a newer installation of Contour, this may be present already, if not, the leader election library will create an empty ConfigMap for you.

Once these changes are made, add `--enable-leader-election` to your `contour serve` command.
The leader will perform and log its operations as normal, and the non-leaders will block waiting to become leader.
You can inspect the state of the leadership using

```bash
$ kubectl describe configmap -n heptio-contour contour
```

and checking the annotations that store exact details using

```bash
$ kubectl get configmap -n heptio-contour -o yaml contour
```

[1]: https://github.com/projectcontour/contour/tree/main/examples/contour
[2]: https://github.com/projectcontour/contour/blob/v1.0.0/examples
[3]: /docs/main/config/fundamentals
[4]: /docs/main/config/annotations
[5]: https://github.com/projectcontour/contour/issues/899
[6]: /docs/main/configuration
[7]: https://github.com/projectcontour/contour/blob/main/examples/contour/README.md
[8]: https://github.com/projectcontour/contour/blob/v1.0.0/examples/contour/02-rbac.yaml#L71
[9]: https://github.com/projectcontour/contour/blob/main/examples/contour/02-rbac.yaml
[10]: https://www.envoyproxy.io/docs/envoy/v1.11.2/intro/version_history
[11]: https://github.com/projectcontour/contour/blob/v0.15.3/examples/
[12]: /docs/main/deploy-options
[13]: https://hub.docker.com/r/projectcontour/contour
[14]: /docs/main/grpc-tls-howto
[15]: https://www.envoyproxy.io/docs/envoy/v1.12.2/intro/version_history
[16]: /getting-started
[17]: https://www.envoyproxy.io/docs/envoy/v1.13.1/intro/version_history
[18]: https://projectcontour.io/quickstart/main/contour.yaml
[19]: https://groups.google.com/forum/?utm_medium=email&utm_source=footer#!msg/envoy-announce/sVqmxy0un2s/8aq430xiHAAJ
[20]: https://www.envoyproxy.io/docs/envoy/v1.14.1/intro/version_history
[21]: https://www.envoyproxy.io/docs/envoy/v1.14.2/intro/version_history
[22]: https://www.envoyproxy.io/docs/envoy/v1.14.3/intro/version_history
[23]: https://www.envoyproxy.io/docs/envoy/v1.15.0/version_history/current
[24]: /guides/ingressroute-to-httpproxy/
[25]: https://www.envoyproxy.io/docs/envoy/v1.16.0/version_history/current
[26]: /guides/xds-migration
[27]: https://www.envoyproxy.io/docs/envoy/v1.16.2/version_history/current
[28]: https://www.envoyproxy.io/docs/envoy/v1.17.0/version_history/current
[29]: https://www.envoyproxy.io/docs/envoy/v1.17.1/version_history/current
[30]: https://www.envoyproxy.io/docs/envoy/v1.17.2/version_history/current
[31]: https://www.envoyproxy.io/docs/envoy/v1.18.2/version_history/current
[32]: https://www.envoyproxy.io/docs/envoy/v1.18.3/version_history/current
[33]: https://www.envoyproxy.io/docs/envoy/v1.19.0/version_history/current
[34]: https://www.envoyproxy.io/docs/envoy/v1.19.1/version_history/current
[35]: https://www.envoyproxy.io/docs/envoy/v1.21.0/version_history/current
[36]: https://www.envoyproxy.io/docs/envoy/v1.21.1/version_history/current
[37]: https://www.envoyproxy.io/docs/envoy/v1.22.0/version_history/current
[38]: https://www.envoyproxy.io/docs/envoy/v1.22.2/version_history/current
[39]: https://www.envoyproxy.io/docs/envoy/v1.21.3/version_history/current
[40]: https://www.envoyproxy.io/docs/envoy/v1.23.0/version_history/v1.23/v1.23.0
[41]: https://www.envoyproxy.io/docs/envoy/v1.23.1/version_history/v1.23/v1.23.1
[42]: https://www.envoyproxy.io/docs/envoy/v1.24.0/version_history/v1.24/v1.24.0
[43]: https://www.envoyproxy.io/docs/envoy/v1.22.6/version_history/current
[44]: https://www.envoyproxy.io/docs/envoy/v1.23.3/version_history/v1.23/v1.23.3
[45]: https://www.envoyproxy.io/docs/envoy/v1.24.1/version_history/v1.24/v1.24.1
[46]: https://www.envoyproxy.io/docs/envoy/v1.25.0/version_history/v1.25/v1.25.0
