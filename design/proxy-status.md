# Proxy Status

## Background

A key feature that Contour provided with `IngressRoute` and now `HTTPProxy` is a way to safely enable ingress with teams spread across namespaces.

A likely scenario is that `root HTTPProxies` are managed by Administrators in a namespace that users do not have access. Users also will have `Includes` defined to their HTTPProxies which delegate path prefixes and headers across namespaces (i.e Conditions).

Additionally, users working in their team namespace need to self-manage their proxies so that they can route traffic to applications (i.e. services).

## User Story

As a user working in a namespace within a single Kubernetes cluster, I want to be able to self-manage my HTTPProxy resources and understand what Includes have been added to my proxies from other parent/root HTTPProxy resources.

## Problem

Users have no visibility as to what conditions are delegated to them from parent/root HTTPProxy resources. This makes understanding what Conditions are delegated to them very difficult. Users also need to understand exactly what names are used in the parent HTTPProxy to properly link them together and enable the delegation.

## Solutions

### Add fields to the Status object

Currently the `status` field of an `HTTPProxy` contains two fields:

- **Status**: States a quick one-word status of the object (i.e. `Valid` / `Invalid`)
- **Description**: States a more detailed explanation of the current status

The Status object could be expanded to also hold a set of merged `Conditions`. When a user creates an HTTPProxy, the status field will be updated with this information regardless of the state of the object (i.e. invalid or valid).

```yaml
Status:
  Current Status:  valid
  Description:     valid HTTPProxy
  Included Conditions:
     prefix: /marketing/blog
     headerContains:
       x-header: abc
     headerExact:
       another-header: somethingelse
```

Addionally, HTTPProxies could include a "dry-run" field or possibly add a second CRD type (e.g. `httpproxy-dryrun`). With this capability, users could first see what conditions they were Included, understand if their proxy would be valid (although would probably be orphan type).

#### Issues

The problem with this design is that users still need to know the exact name to create the delegated HTTPProxy and traffic could be routed in a way that's undesirable before users understood what conditions are applied yet. 

*Note: This is mitigated by the `dry-run` type of HTTPProxy*

### New Status Object

Contour could write a new CRD "Status Object" whenever an HTTPProxy is created and contains an Include delegating conditions to another namespace. Contour would need write permission to all namespaces where HTTPProxies could exist. Users could then consult this CRD before creating a new HTTPProxy to understand their namespaces environment.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxyStatus
metadata:
  name: teama  # Name matches current namespace
  namespace: teama
spec:
  httpproxies:
    - name: proxy01  # Name of proxy that was Included from a parent
      conditions:
      - prefix: /marketing/blog
      - headerContains:
          x-header: abc
      - headerExact:
          another-header: somethingelse
    - name: proxy02  # Name of proxy that was Included from a parent
       conditions:
       - prefix: /marketing/blog
       - headerContains:
           x-header: abc
       - headerExact:
           another-header: somethingelse
```

####  Issues

The problem with this design is that Contour needs write access to any namespace that contains Proxies which could pose as a security risk.

### Dashboard

Users could consume information from a dashboard as an Octant plugin possibly. This adds additonal applications that are required to use Contour which feels messy. Contour should work at least (at a minimum) at the `kubectl` layer. Any additional visualizations on top would extend the UX/Functionality, but shouldn't be a requirement. 

### Contour Command

Contour could release a client kubectl plugin or 

