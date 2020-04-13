# Listener Design

**Status**: _Draft_

This document describes Contour's additional HTTP listener configuration for the purpose of separating host/port configuration for health checking and metrics services.

# Goals

- Provide support to define custom host/port for each of the health and metrics services within Contour
- Maintain default backwards compatible configuration for how this is currently achieved

# Non-goals

- Build a method of authentication/authorization around supporting service listeners

# Background

Contour, provides listeners for health-checking (for liveness/readiness probes), and metrics(for observability). These services are often defaulted to be exposed. Allowing configuration for these additional services provides a more intentful deployment, and helps cluster operators run their application securely by default.

# High-level design

This document proposes Contour listener configuration that should allow for custom configuration of it's exposed services. This enables end-users to maintain separate models of authentication/authorization around these services, where each of the underlying services may only be intended for specific consumers.