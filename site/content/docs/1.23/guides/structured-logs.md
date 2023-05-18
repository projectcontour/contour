---
title: How to enable structured JSON logging
---

This document describes how to configure structured logging for Envoy via Contour.

## How the feature works

Contour allows you to choose from a set of JSON fields that will be expanded into Envoy templates and sent to Envoy.
There is a default set of fields if you enable JSON logging, and you may customize which fields you log.

The list of available fields are discoverable in the following objects:
- [JSONFields][1] are fields that have built in mappings to commonly used envoy
  operators.
- [envoySimpleOperators][2] are the names of simple envoy operators that don't
  require arguments, they are case-insensitive when configured.
- [envoyComplexOperators][3] are the names of complex envoy operators that require
  arguments.

The default list of fields is available at [DefaultFields][4].

## Enabling the feature

To enable the feature you have two options:

- Add `--accesslog-format=json` to your Contour startup line.
- Add `accesslog-format: json` to your configuration file.

Without any further customization, the [default fields][4] will be used.

## Customizing logged fields

To customize the logged fields, add a `json-fields` list of strings to your
config file.  If the `json-fields` key is not specified, the [default fields][4]
will be configured.

To use a value from [JSONFields][1] or [envoySimpleOperators][2], simply include
the name of the value in the list of strings. The JSONFields are case-sensitive,
but envoySimpleOperators are not.

To use [envoyComplexOperators][3] or to use alternative field names, specify strings as
key/value pairs like `"fieldName=%OPERATOR(...)%"`.

Unknown field names in non key/value fields will result in validation errors, as
will unknown Envoy operators in key/value fields. Note that the
`DYNAMIC_METADATA` and `FILTER_STATE` Envoy logging operators are not supported
at this time due to the complexity of their validation.

See the [example config file][5] to see this used in context.

## Sample configuration file

Here is a sample config:

```yaml
accesslog-format: json
json-fields:
  - "@timestamp"
  - "authority"
  - "bytes_received"
  - "bytes_sent"
  - "customer_id=%REQ(X-CUSTOMER-ID)%"
  - "downstream_local_address"
  - "downstream_remote_address"
  - "duration"
  - "method"
  - "path"
  - "protocol"
  - "request_id"
  - "requested_server_name"
  - "response_code"
  - "response_flags"
  - "uber_trace_id"
  - "upstream_cluster"
  - "upstream_host"
  - "upstream_local_address"
  - "upstream_service_time"
  - "user_agent"
  - "x_forwarded_for"
```

[1]: https://github.com/projectcontour/contour/blob/main/pkg/config/accesslog.go#L33-L45
[2]: https://github.com/projectcontour/contour/blob/main/pkg/config/accesslog.go#L49-L93
[3]: https://github.com/projectcontour/contour/blob/main/pkg/config/accesslog.go#L97-L102
[4]: https://github.com/projectcontour/contour/blob/main/pkg/config/accesslog.go#L4
[5]: {{< param github_url >}}/tree/{{< param branch >}}/examples/contour/01-contour-config.yaml
