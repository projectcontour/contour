# Omit Route Sorting for `HTTPProxy`

Status: In-Review

## Abstract

This document describes a feature flag that allows operators to disable the default route sorting algorithm used for `HTTPProxy`.

## Background

Since Envoy is greedy matching path routes, order is important. Contour sorts the routes using a heuristics which attempts to put more specific routes first. The heuristic is implemented in [sorter.go](https://github.com/projectcontour/contour/blob/main/internal/sorter/sorter.go). The sorter implements the following order:

1. Exact Match
2. Regex Match
3. Prefix Match

And it breaks ties in the following way:

1. Matches of the same type are ordered by their length.
2. Matches of the same length are sorted by the presense of additional matching conditions(e.g. if a particular header exists)
3. Matches of the same type, length and match conditions are sorted lexicographically.

## Personas

There are two main personas in question are:

* Cluster Administrator: A cluster administrator is responsible for providing core functionality to a k8s cluster. For the purposes of this document the cluster administrator installs Contour to the cluster. The administrator is responsible for the health of the Contour services.

* ResourceOwner: Someone who manages an HTTPProxy resource inside the Kubernetes cluster configuring the Route/Service combination to allow users of the application to access. A resource owner might not be familiar with Envoy, doesn't have access to the Envoy admin interface and has surface level understanding of how the the HTPProxy Routes are enforced.

## Goals

- Allow `Cluster Administrator` to disable the sorting logic

## Non-goals
Improving or changing the heuristics used for sorting routes.

## High-level design

At a high level we want to offer a configuration for `Cluster Administrator` to be abe to disable route sorting. From the configurator perspective they would have access to a `featureFlag` called `omitRouteSorting` that disables all route sorting. This can be implemented similar to the feature gate we used for [Endpoint Slices](https://github.com/projectcontour/contour/pull/5745)

For Contour developers this means that we need to be careful of the data-structures we use to store the routes as we translate them from `HTTPProxy` to Envoy `Routes`. With sorting enabled we could use data structures like a `map` that don't offer any ordering guarantees but this wouldn't matter since Contour decided the order. To avoid this we need to switch to using `slices` as the only (Golang native) data structure for all cases. This might make some operations theoretically inefficient but in practice the number of possible routes within a virtual host is pretty small so it shouldn't matter. This invariance can be guaranteed with unit tests which already cover this.

### Basic Case

In the basic case a single `HTTPProxy` with the following routes:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
spec:
  virtualhost:
    fqdn: example.com
  routes:
    - conditions:
      - prefix: /
      - header:
          exact: bar
          name: x-experiment
      services:
        - name: b1
          port: 400
    - conditions:
      - exact: /bar
      services:
        - name: b2
          port: 400
    - conditions:
      - exact: /
      services:
        - name: b3
          port: 400
```

Would result in the following:

| Request                          | With Sorting  | Without Sorting |
| -------------                    | ------------- | --------------- |
| GET -H "x-experiment: bar" /bar  | b2            | b1              |
| GET /bar | Content Cell          | b2            | b2              |
| GET /                            | b3            | b3              |


### Route Matching with Include Statements

Include statements are resolved in a depth first fashion.

```yaml
# httpproxy-inclusion-samenamespace.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-include
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
  includes:
  - name: service2
    namespace: default
  routes:
    - conditions:
      - prefix: /bar
      services:
        - name: s1
          port: 80
      - prefix: /
      services:
        - name: s2
          port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: service2
  namespace: default
spec:
  routes:
    - conditions:
      - prefix: /blog
      services:
        - name: s3
          port: 80
```

Will result in the following traffic table in Envoy:

```yaml
virtual_hosts:
- name: example
  domains: ["example.com"]
  routes:
  - match:
      prefix: /blog
    route:
      cluster: s3
  - match:
      prefix: /bar
    route:
      cluster: s1
  - match:
      prefix: /
    route:
      cluster: s2

```


### Route Matching for Objects managed by both an HTTPProxy and Ingress (or Gateway)

Since `Gateway` and `Ingress` have routing conformance which are defined outside of the repository any `virtualHost` that is managed by both `CRDs` will follow the `Gateway`/`Ingress` sorting regardless of
the configuration.

## Alternatives Considered

Continue to improve the heuristic used to sort the routes. The main issues with this approach are:

1. It is hard to create one heuristic that fit all uses cases. While the intuition behind the heuristic is reasonable there might be different organizations adopting Contour which disagree on what is `more-specific` routing rule. For example consider the two following routing rules:
    1.  If header `X-Experiment-Header: foo` route the requests to backend `foo` regardless of the path
    2.  If the `ExactPath` is `/bar` route the request to backend `bar` regardless of the headers.

    An argument could be made that for sorting either as `1 > 2` or `2 > 1`

2. The sorting algorithm is hard to reason about as a developer and tricky to make deterministic. The hard part is that for the algorithm to be stable it needs to satisfy the following conditions:
    1. if `a > b` then `b < a`
    2. if `a > b` and `b > c` then `a > c`

The above makes a lot of sense mathematically but in practice it becomes not-intuitive when dealing with `MatchConditions` and their length.
