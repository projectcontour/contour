## Max HTTP requests per IO cycle is configurable as an additional mitigation for HTTP/2 CVE-2023-44487

Envoy v1.27.1 mitigates CVE-2023-44487 with some default runtime settings, however the `http.max_requests_per_io_cycle` does not have a default value.
This change allows configuring this runtime setting via Contour configuration to allow administrators of Contour to prevent abusive connections from starving resources from other valid connections.
The default is left as the existing behavior (no limit) so as not to impact existing valid traffic.

The Contour ConfigMap can be modified similar to the following (and Contour restarted) to set this value:

```
listener:
  max-requests-per-io-cycle: 10
```

(Note this can be used in addition to the existing Listener configuration field `listener.max-requests-per-connection` which is used primarily for HTTP/1.1 connections and is an approximate limit for HTTP/2)

See the [Envoy release notes](https://www.envoyproxy.io/docs/envoy/v1.27.1/version_history/v1.27/v1.27.1) for more details.
