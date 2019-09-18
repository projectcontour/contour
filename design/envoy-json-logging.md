# Allow Envoy to output JSON logs

Status: Draft

This proposal is a design for adding to Contour the ability to configure Envoy to output JSON logs.
It is intended to allow some customisability of the JSON output while providing sensible defaults.

## Goals

- Configuration to enable Envoy to output JSON logs.
- Some customisability of the fields in the JSON logs.

## Non Goals

- Passthrough of JSON config straight to Envoy.
- Enriching the Envoy logs further with Contour information (eg details of the IngressRoute that generated the route)
- Arbitrary configuration of the JSON output.
- Adding JSON logging to Contour itself.

## Background

This feature was requested some time ago in [#624](https://github.com/projectcontour/contour/issues/624).
Since that time, two changes have come to Contour that assist with this design:

- Configuration file for Contour.
- Recommended deployment changed to separate pods for Contour and Envoy.

Contour is intended to be an *opinionated* piece of software, that sets defaults that you probably don't want to touch.
Until recently, we did not have a good way to allow the required customization of this feature while still providing good defaults.
Now that we have the configuration file available, it is the logical place for this configuration to sit.

## High-Level Design

This proposal will add three things to Contour:

- A boolean flag to enable JSON logging. If no custom format is specified, a 'you get everything' JSON format will be the default.
- A configuration stanza to allow the specification of the JSON field names that will be in the logs.
- A translation table to translate JSON field names into standard Envoy log template fields.

The translation table is present because of Contour's overarching design goal:
We want to ensure that you cannot create an invalid Envoy configuration.
The translation table is to ensure that the log template fields are parseable Envoy config.
Allowing the direct specification of Envoy template config is very risky, mistakes there *will* crash your Envoy deployment.

## Detailed Design

### Top-level config file change

A new stanza will be added to the config file:
`logging:`

A new struct will be created to hold this stanza, `LoggingConfig`.

### Logging config

The logging config will look like this:

```yaml
logging:
  format: json
  json-fields:
  - protocol
  - duration
  - request_method
  - ...more fields
  - @timestamp
```

#### `format`

There are two valid options here: `json` and `envoy`.

`envoy`  will use Envoy's [default format](https://www.envoyproxy.io/docs/envoy/latest/configuration/access_log#default-format).

Having only `format: json` present will set the Envoy logs to JSON format, with the default fields specified in the `json-fields` section.

Having `format: json` with custom `json-fields` will set the logs to *only those fields*. If you want to not log the HTTP method, that's on you.

#### `json-fields`

`json-fields` is an optional field, and has no effect if `logging.format` is not `json`.

By default, json-fields is set as follows:

```yaml
logging:
  format: json
  json-fields:
  - @timestamp
  - downstream_remote_address
  - path
  - authority
  - protocol
  - upstream_service_time
  - upstream_local_address
  - duration
  - downstream_local_address
  - user_agent
  - response_code
  - response_flags
  - method
  - request_id
  - upstream_host
  - client_ip
  - requested_server_name
  - bytes_received
  - bytes_sent
  - upstream_cluster
  - x-forwarded-for
```

Users can specify a subset of these fields, and Envoy will be configured to only log the specified fields.

See the Field Translations section for the exact Envoy spec the fields translate to.

### Field Translations

The initial canonical field translations will be as follows:

```go
var translations := map[string]string{
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
  "x_forwarded_for": "%REQ(X-FORWARDED-FOR)%",
  "client_ip": "%REQ(X-ENVOY-EXTERNAL-ADDRESS)%",
}
```
