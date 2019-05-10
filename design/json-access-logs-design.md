# JSON Access Logs

Status: Draft

This document proposes a change to re-configure the default access log format to a structure JSON format.

## Goals

- Create an accepted format for JSON structured access logs produced by envoy.
- Allow backends like Elasticsearch to consume structured access log data for easy ingestion and reporting.

## Non Goals

- We are not converting all envoy log output to JSON, just the access logs.

## Background

Access logs are currently only output using the default access log format that envoy provides.
The default format has several issues, but the biggest issues are that it makes querying more expensive and harder to craft for the average user.
Creating reports with unstructured data is almost impossible without pre-process the logs at ingestion time.
When using ingresses like NGINX we can structure our output format at the proxy layer, making ingestion easier and standardized.

## High-Level Design

We need to configure envoy via the listener api to change the access log format from the default.

## Detailed Design

The default access log format is as follows, as described in the envoy docs:
(https://www.envoyproxy.io/docs/envoy/latest/configuration/access_log#default-format-string)

```text
[%START_TIME%] "%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %PROTOCOL%"
%RESPONSE_CODE% %RESPONSE_FLAGS% %BYTES_RECEIVED% %BYTES_SENT% %DURATION%
%RESP(X-ENVOY-UPSTREAM-SERVICE-TIME)% "%REQ(X-FORWARDED-FOR)%" "%REQ(USER-AGENT)%"
"%REQ(X-REQUEST-ID)%" "%REQ(:AUTHORITY)%" "%UPSTREAM_HOST%"\n
```

The proposed structured JSON format can be configured via the API using the `json_format` dictionary:
(https://www.envoyproxy.io/docs/envoy/latest/configuration/access_log#format-dictionaries)

```json
{
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
},
```

The existing `accesslog` function is implemented in the `HTTPConnectionManager` and `TCPProxy` filter chains.
We can extend the `accesslog` function to add the `json_format` dictionary.

## Alternatives Considered

There doesn't appear to be any other way to configure the filter chains through envoy configs or defaults.
Dynamic listeners receieve all of their configuration via the API and don't appear to be able to inheret static configuration.
