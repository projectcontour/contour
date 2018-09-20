# Contour Roadmap

This document captures open questions and features for Contour.

Upcoming release milestones are tracked via [GitHub Milestones][0].

The ordering of items is unrelated to their priority or order of completion.

- **Update Ingress status**. Contour does not update the `status` section of the Ingress object. This doesn't appear to be critical if Contour is the single Ingress controller for a cluster, but if multiple Ingress controllers are in play in a cluster, users won't be able to assume that all Ingress traffic is routed through a single IP.  We will also need to handle similar behavior in the new IngressRoute CRD
- **Expanded IngressRoute Specification**. In v0.6, we shipped an initial implementation of the new IngressRoute Custom Resource Definition.  Over the next several releases, we'll be expanding the API specification to add support (or sane defaults) for common features that are typically managed via annotations.
- **Envoy Upgrades**.  We need to keep Contour up-to-date with the latest Envoy, envoy-data-plane, and GRPC updates.

[0]: https://github.com/heptio/contour/milestones
