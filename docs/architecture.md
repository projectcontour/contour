# Contour architecture

The Contour Ingress controller is a collaboration between:

* Envoy, which provides the high performance reverse proxy.
* Contour, which acts as a management server for Envoy and provides it with configuration.

These containers are deployed as sidecars in a pod, although other configurations are possible.

During pod initialisation, Contour runs as an initcontainer and writes a bootstrap configuration to a temporary volume.
This volume is passed to the Envoy container and directs Envoy to treat its sidecar Contour container as a [management server][0].

After initialisation is complete, the Envoy container starts, retrieves the bootstrap configuration written by Contour, and starts to poll Contour for configuration.

Envoy will gracefully retry if the management server is unavailable, which removes any container startup ordering issues.

Contour is a client of the Kubernetes API. Contour watches Ingress, Service, and Endpoint objects, and acts as the management server for its Envoy sibling by translating its cache of objects into the relevant JSON stanzas: Service objects for CDS, Ingress for RDS, Endpoint objects for SDS, and so on).

The transfer of information from Kubernetes to Contour is by watching the API with the SharedInformer framework.
The transfer of information from Contour to Envoy is by polling from the Envoy side.

Kubernetes Readiness Probes are configured to check the status of Envoy.
These are enabled over the metrics port and are served over http via `/healthz`.

[0]: https://github.com/envoyproxy/data-plane-api#terminology
