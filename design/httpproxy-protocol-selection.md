# Protocol Selection Overrides in HTTPProxy

**Status**: _Draft_

This document specifies a design for overriding annotation-based protocol selection in HTTPProxy.

## Goals

Allow for the HTTPProxy to encode all of the additional configuration needed to program Envoy beyond what's provided in a vanilla K8s Service.

## Non Goals

- Deprecation of existing annotation-based mechanisms

## Background

Today, Contour requires K8s Services to be annotated with additional protocol metadata encoded in annotations to use more than `http1`.  This presents a problem for controllers looking to emit HTTPProxy resources which don't own the K8s services being targeted because it involves manipulating resources they don't control.  The motivating issue (https://github.com/projectcontour/contour/issues/1962) also calls out the use of annotations as somewhat anithetical to some of the underlying motivations for HTTPProxy in the first place!

## High-Level Design

Add a single new field to `Service` that encodes the protocol data that must currently be extracted from the annotation `projectcontour.io/upstream-protocol.{protocol}`.  From #1962:

```yaml
spec:
  virtualhost:
    fqdn: dashboard.kubernetes.com
    tls:
      secretName: kubernetes-dashboard-tls
  routes:
    - conditions:
      - prefix: /
      services:
        - name: kubernetes-dashboard
          protocol: https # <--- NEW FIELD
          port: 443
```

## Detailed Design

At creation the `dag.Cluster` will incorporate either the specified protocol, or
fallback on the protocol of the referenced service, then the protocol selection
logic in `envoy.Cluster` can simply key off of the `Protocol` field of the
`dag.Cluster` instead of `c.Upstream.Protocol`.

## Alternatives Considered

Require downstream consumers to annotate K8s Services with Contour annotations (and every other provider they might integrate with).

Adopt a port-naming convention like the one defined by [Istio](https://istio.io/docs/ops/configuration/traffic-management/protocol-selection/).

Waiting for [KEP 1422](https://github.com/kubernetes/enhancements/pull/1422), which proposes building similar hints into the Kubernetes service itself.  Realistically this is over a year away from being a viable alternative.

## Security Considerations

None at this time.
