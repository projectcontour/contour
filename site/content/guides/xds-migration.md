---
title: Migrating from xDS v2 --> v3
layout: page
---

This guide shows you how to migrate in-place instances of Envoy running xDS v2 to v3. 

## Summary

Envoy communicates with Contour over gRPC which allows for dynamic communication for Envoy configuration updates.
Until Contour v1.10, this gRPC xDS communication utilized the v2 xDS for transport as well as resource version.

In the beginning of Q1 2021, the Envoy community is [deprecating][0] the v2 xDS API in favor of the v3 which has been stable since Q1 2020.
Contour offers support for the v3 xDS API in Contour v1.10.0. 

Practically for users, this change has no effect on how Contour configures Envoy to route ingress traffic inside a Kubernetes cluster, however
it's important to upgrade to this new version immediately since newer versions of Envoy won't support the `v2` api. 

## Background

Envoy gets configured with a bootstrap configuration file which Contour provides via an `initContainer` on the Envoy daemonset.
This file configures the dynamic xDS resources, Listener Discovery Service (LDS) and Cluster Discovery Service (CDS), to point to Contour's xDS gRPC server endpoint.

The bootstrap configuration file has two settings in the LDS/CDS entries which tell Contour what Resource & Transport version they would like to use. 
In Contour v1.10.0, there's a new flag, `--xds-resource-version`, on the `contour bootstrap` command which is used in the `initContainer` that allows users to specify the xDS resource version.

Setting this flag to `v3` will configure Envoy to request the `v3` xDS Resource API version and will become the default in Contour v1.11.0.    

## In-Place Upgrade

When users have an existing Contour installation and wish to upgrade without dropping connections, users should first upgrade Contour to v1.10.0 which will serve both v2 and v3 xDS versions from the same gRPC endpoint.
Next, change the Envoy Daemonset or deployment to include `--xds-resource-version=v3` on the `initContainer` which runs the `contour bootstrap` command. 
Setting this new flag to `v3` tells Envoy to upgrade to the v3 resource version.
The usual rollout process will handle draining connections allowing a fleet of Envoy instances to move from the v2 xDS Resource API version gradually to the v3 version.

## Redeploy Upgrade

Redeploying Contour is a simple path for users who do not need to upgrade without dropping connections.
The only change to remember is to set the `--xds-resource-version=v3` on the bootstrap `initContainer` to configure the new instances of Envoy to use the `v3` xDS Resource API. 

[0]: https://www.envoyproxy.io/docs/envoy/latest/api/api_supported_versions