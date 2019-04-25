# Contour Logging

## Envoy Access Logs

By default Contour configures Envoy to emit access logs in a structured JSON format. We use the envoy json_format dictionary to provide structured access logs.

https://www.envoyproxy.io/docs/envoy/latest/configuration/access_log#format-dictionaries

### Log Format

```json
"json_format": {
  "@timestamp": "%START_TIME%",
  "authority": "%REQ(:AUTHORITY)%",
  "bytes_received": "%BYTES_RECEIVED%",
  "bytes_sent": "%BYTES_SENT%",
  "downstream_local_address": "%DOWNSTREAM_LOCAL_ADDRESS%",
  "downstream_remote_address": "%DOWNSTREAM_REMOTE_ADDRESS%",
  "duration": "%DURATION%",
  "method": "%REQ(:METHOD)%",
  "path": "%REQ(X-ENVOY-ORIGINAL-PATH?:PATH)%",
  "protocol": "%PROTOCOL%",
  "request_id": "%REQ(X-REQUEST-ID)%",
  "requested_server_name": "%REQUESTED_SERVER_NAME%",
  "response_code": "%RESPONSE_CODE%",
  "response_flags": "%RESPONSE_FLAGS%",
  "uber_trace_id": "%REQ(UBER-TRACE-ID)%",
  "upstream_cluster": "%UPSTREAM_CLUSTER%",
  "upstream_host": "%UPSTREAM_HOST%",
  "upstream_local_address": "%UPSTREAM_LOCAL_ADDRESS%",
  "upstream_service_time": "%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)%",
  "user_agent": "%REQ(USER-AGENT)%",
  "x_forwarded_for": "%REQ(X-FORWARDED-FOR)%"
}
```
