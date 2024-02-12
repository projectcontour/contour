# Troubleshooting Common Proxy Errors

## Unexpected HTTP errors

Here are some steps to take in investigating common HTTP errors that users may encounter.
We'll include example error cases to debug with these steps.

1. Inspect the HTTP response in detail (possibly via `curl -v`).

    Here we're looking to validate if the error response is coming from the backend app, Envoy, or possibly another proxy in front of Envoy.
    If the response has the `server: envoy` header set, the request at least made it to the Envoy proxy so we can likely rule out anything before it.
    The error may originate from Envoy itself or the backend app.
    Look for headers or a response body that may originate from the backend app to verify if the error is in fact just the intended app behavior.
    In the example below, we can see the response looks like it originates from Envoy, based on the `server: envoy` header and response body string.

    ```
    curl -vvv example.projectcontour.io
    ...
    > GET / HTTP/1.1
    > Host: example.projectcontour.io
    ...
    >
    < HTTP/1.1 503 Service Unavailable
    < content-length: 91
    < content-type: text/plain
    < vary: Accept-Encoding
    < date: Tue, 06 Feb 2024 03:44:30 GMT
    < server: envoy
    <
    * Connection #0 to host example.projectcontour.io left intact
    upstream connect error or disconnect/reset before headers. reset reason: connection failure
    ```

1. Look at the Envoy pod logs for the access logs corresponding to the erroring request/response.

    The exact fields/field ordering present in the access log may vary if you have [configured a custom access log string or JSON access logs][0].
    For example for a Contour installation using the [default Envoy access log format][1] we would want to inspect:
    * `%REQ(:METHOD)%`, `%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%`, `%REQ(:AUTHORITY)%`, `%PROTOCOL%`: Ensure these are sensible values based on your configured route and HTTP request
    * `%RESPONSE_FLAGS%`: See the [documentation on Envoy response flags][2] and below how to interpret a few of them in a Contour context:
      * `UF`: Likely that Envoy could not connect to the upstream
      * `UH`: Upstream Service has no health/ready pods
      * `NR`: No configured route matching the request
    * `%DURATION%`: Can correlate this with any configured timeouts
    * `%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%`: Can correlate this with any configured timeouts. If `-` then this is a hint the request was never forwarded to the upstream.
    * `%UPSTREAM_HOST%`: This is the IP of the upstream pod that was selected to proxy the request to and can be used to verify the exact upstream instance that might be erroring.

    For example in this access log:

    ```
    [2024-02-06T15:18:17.437Z] "GET / HTTP/1.1" 503 UF 0 91 1998 - "103.67.2.26" "curl/8.4.0" "d70640ec-2feb-46f8-9f63-24c44142c42e" "example.projectcontour.io" "10.244.8.27:8080"
    ```

    We can see the `UF` response flag as the cause of the `503` response code.
    We also see the `-` for upstream request time.
    It is likely in this case that Envoy was not able to establish a connection to the upstream.
    That is further supported by the request duration of `1998` which is approximately the default upstream connection timeout of `2s`.

1. Inspect Envoy metrics

   This method of debugging can be useful especially for deployments that service a large volume of traffic.
   In this case, access logs are possibly not suitable to use, as the volume of logs may be too large to pinpoint an exact erroring request.

   Metrics from individual Envoy instances can be viewed manually or scraped using Envoy's prometheus endpoints and graphed using common visualization tools.
   See the `/stats/prometheus` endpoint of the [Envoy admin interface][3].

   Metrics that may be useful to inspect:
   * [Listener metrics][4]
     * `downstream_cx_total`
     * `ssl.connection_error`
   * [HTTP metrics][5]
     * `downstream_cx_total`
     * `downstream_cx_protocol_error`
     * `downstream_rq_total`
     * `downstream_rq_rx_reset`
     * `downstream_rq_tx_reset`
     * `downstream_rq_timeout`
     * `downstream_rq_5xx` (and other status code groups)
   * [Upstream metrics][6]
     * `upstream_cx_total`
     * `upstream_cx_connect_fail`
     * `upstream_cx_connect_timeout`
     * `upstream_rq_total`
     * `upstream_rq_timeout`

1. Send a direct request to the backend app to narrow down where the error may be originating.

    This can be done via a port-forward to send a request to the app directly, skipping over the Envoy proxy.
    If this sort of request succeeds, we know the issue likely originates from Contour configuration or the Envoy proxy rather than the app itself.

[0]: /docs/{{< param latest_version >}}/config/access-logging/
[1]: https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log/usage#default-format-string
[2]: https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log/usage#config-access-log-format-response-flags
[3]: /docs/{{< param latest_version >}}/guides/prometheus/#envoy-metrics
[4]: https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/stats
[5]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/stats
[6]: https://www.envoyproxy.io/docs/envoy/latest/configuration/upstream/cluster_manager/cluster_stats
