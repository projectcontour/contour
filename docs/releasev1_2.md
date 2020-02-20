We are delighted to present version 1.2.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

All Contour users should upgrade to Contour 1.2.0 and Envoy 1.13.0.

## New and improved 

Contour 1.2.0 includes several new features as well as the usual smattering of fixes and minor improvements.

### Hot-Reload Certificates

Contour now has support for certificate rotation for XDS gRPC interface between Contour and Envoy.
It is achieved by lazily loading certificates and key every time new TLS connection is established by Envoy.

This change addresses only the certificate rotation in Contour (server) and similar support is needed for Envoy (client) to cover the whole use case.

PR: https://github.com/projectcontour/contour/pull/2198

Thanks @tsaarni

### Envoy Shutdown Manager

The Envoy process, the data path component of Contour, at times needs to be re-deployed.
This could be due to an upgrade, a change in configuration, or a node-failure forcing a redeployment.

Contour now offers a new sub-command to named `envoy shutdown-manager` which will assist in an Envoy rollout to minimize connection errors from clients.
The shutdown manager first sends the healthcheck fail request to Envoy and then begins polling the http/https listeners for active connections from the /stats endpoint available on localhost:9001.
When the connections reach zero or a configured parameter, the pod is allowed to be terminated.
If the pods configurable termination grace period seconds is met before the open connections are fully drained, the pod will be terminated.

This new component runs as another container in the Envoy pod.

Design: https://github.com/projectcontour/contour/blob/master/design/envoy-shutdown.md
PR: https://github.com/projectcontour/contour/pull/2227

Thanks @stevesloka

### Record EventHandler Operation Metrics

Contour adds some new Prometheus metrics for the various API operations and kinds observed from the Kubernetes informers.
This information is helpful to understand the amount of changes that Contour is processing from a Kubernetes cluster.
This change also includes a sample Grafana dashboard.

```yaml
% curl -s 127.0.0.1:8000/metrics |  grep eventhandler
# HELP contour_eventhandler_operation_total Total number of eventHandler operations received by operation and object kind
# TYPE contour_eventhandler_operation_total gauge
contour_eventhandler_operation_total{kind="contour.heptio.com/IngressRoutev1beta1",op="onAdd"} 2
contour_eventhandler_operation_total{kind="contour.heptio.com/TLSCertificateDelegationv1beta1",op="onAdd"} 1
contour_eventhandler_operation_total{kind="projectcontour.io/HTTPProxyv1",op="onAdd"} 1
contour_eventhandler_operation_total{kind="unknown",op="onAdd"} 76
```

PR: https://github.com/projectcontour/contour/pull/2244
PR: https://github.com/projectcontour/contour/pull/2261

Thanks @davecheney, @youngnick

### SafeRegex limit raised

Raise the SafeRegex size limit from 1,000 to 1048576.
There is no evidence that this number is sufficient for all possible regex patterns, thus the limit represents the "no limit" limit because it is currently not
possible for envoy to reject a regex entry in a way that Contour can trace back to the original input.

PR: https://github.com/projectcontour/contour/pull/2241

Thanks @davecheney

### Minor improvements

- Contour is built with Go 1.13.8
- Update Envoy to 1.13.0
- Envoy go-control-plane updated to v0.9.2
- Upgrade google/go-cmp to version 0.4.0
- Upgrade client-go to v0.17.0
- Contour now utilizes the Dynamic client for CRD resources

## Bug fixes

### Add HTTPProxy Service.Protocol Validation 

Adds an enum validation to limit the values for the service.protocol field.

_Note: Users will need to reapply the crd spec to get the validation_

PR: https://github.com/projectcontour/contour/pull/2158

Thanks @stevesloka

### HTTPProxy requestHeadersPolicy Validation

For an HTTPProxy, a requestHeaderPolicy can only be able to set host header at the HTTProxy.Routes.Service level.

PR: https://github.com/projectcontour/contour/pull/2157

Thanks @stevesloka

### Ensure certgen handles already-existing secrets correctly

The cert-gen example job now ensures that the the Job will succeed if the secrets already exist.

PR: https://github.com/projectcontour/contour/pull/2178

Thanks @youngnick

### Other changes

- Add Kubernetes Support Matrix
- Update Envoy list of required extensions

## Upgrading

Please consult the [Upgrading](https://projectcontour.io/resources/upgrading/) document for further information on upgrading from Contour 1.1.0 to Contour 1.2.0.