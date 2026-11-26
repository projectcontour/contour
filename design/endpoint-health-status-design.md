# Export unready endpoints

Status: Draft

## Abstract
Contour, right now, leaves endpoints that are not ready out of the ClusterLoadAssignments it sends to Envoy, so Envoy reports every endpoint it knows of as healthy.
This proposal sends all endpoints to Envoy with an explicit health status, so that the `envoy_cluster_membership_*` metrics reflect the real state of a Service.

## Background
Kubernetes reports the state of each endpoint of a Service in its EndpointSlices, using the `ready`, `serving` and `terminating` conditions.
The EndpointSliceTranslator skips every endpoint whose `ready` condition is false.

As a result `envoy_cluster_membership_healthy` always equals `envoy_cluster_membership_total`.
A Service whose pods crash, fail readiness probes, or are terminating looks "smaller" to Envoy, not unhealthy.
The "Endpoint Percentage Health" and "Unhealthy Endpoints" panels of the [example Grafana dashboards](../examples/grafana/dashboards.yaml) always show 100% and 0 for this reason.

Envoy supports endpoint statuses, see the `health_status` [EDS documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/endpoint/v3/endpoint_components.proto#envoy-v3-api-msg-config-endpoint-v3-lbendpoint).

## Goals
- `envoy_cluster_membership_total`, `_healthy` and `_excluded` reflect the endpoints of the Service, including the ones that are not ready.
- Traffic keeps going only to ready endpoints, as it does today.

## Non Goals
None

## High-Level Design
The EndpointSliceTranslator keeps every endpoint of an EndpointSlice, and maps the Kubernetes endpoint conditions to an Envoy health status.
That is it.

## Detailed Design
`EndpointSliceCache.RecalculateEndpoints` no longer skips endpoints that are not ready.
It sets `LbEndpoint.health_status` from the endpoint conditions with some new function:

```go
func endpointHealthStatus(conditions discovery_v1.EndpointConditions) envoy_config_core_v3.HealthStatus
```

| Kubernetes conditions | Envoy health status | Envoy behavior |
|---|---|---|
| `ready` is true, or not set | `UNKNOWN` (not set) | Healthy, as today. |
| `ready` is false, `terminating` is true | `DRAINING` | No new requests. _Counted in `membership_excluded`._ |
| `ready` is false | `UNHEALTHY` | No new requests. _Counted in `membership_total` but not in `membership_healthy`._ |

Ready endpoints keep an unset status, so the generated configuration for Services whose endpoints are all ready does not change.

## Alternatives Considered
I guess, the `DRAINING` status can be replaced with `UNHEALTHY`, it will only affect the metrics as far as I understand.

## Security Considerations
None.

## Compatibility
- Dashboards and alerts that compare `envoy_cluster_membership_healthy` to `envoy_cluster_membership_total` start to show the endpoints that are not ready.
  This is the intended change, but alerts on `healthy < total` may start to fire for pods that are crash looping or starting up.
- ClusterLoadAssignments of Services with pods that are not ready get bigger, and Envoy keeps hosts for pods that stay not ready for a long time.
- The number of EDS updates does not change: a readiness change updated the ClusterLoadAssignment before, and still does.
- All supported Envoy versions support `health_status` in EDS.

## Implementation
The change is in `internal/xdscache/v3/endpointslicetranslator.go`, with tests in `endpointslicetranslator_test.go` and `internal/featuretests/v3/endpoints_test.go`.
