# Allow Envoy to output arbitrary fields for JSON logs

Status: Accepted

## Abstract
Allow users to add arbitrary supported Envoy JSON fields to the access log.

## Background
This feature was requested in [#3032](https://github.com/projectcontour/contour/issues/3032).
Contour [allows Envoy to output JSON logs](https://github.com/projectcontour/contour/blob/main/design/envoy-json-logging.md).
The current opinionated implementation allows users to configure log fields from the [limited list](https://godoc.org/github.com/projectcontour/contour/internal/envoy#DefaultFields) of available Envoy fields.

The reason that the current implementation restricts users to an approved list of fields is that recovering from sending invalid config to Envoy is hard.
If invalid JSON fields are included in the Envoy config, then Envoy will reject the listener and will not accept updates from Contour until the Envoy is restarted (just removing the error will not fix the issue).

## Goals
* Support nearly all the fields listed in [the Envoy docs][1]
* Specifically, support configuration of parameterized JSON fields
* Validation of the JSON fields config
* Continue to support existing `json-fields` translations (existing config continues to work)

## Non Goals
* Supporting `DYNAMIC_METADATA` and `FILTER_STATE`.
  These parameterized fields are more complicated to validate.


## High-Level Design

* The current `json-fields` Contour configuration will be extended to accept values of the form: `field_name="literal string with Envoy field operator"`.


## Detailed Design

### Configuration

The current `json-fields` configuration accepts the list of fields from the predefined list of available translations, defined in the [source code](https://github.com/projectcontour/contour/blob/main/internal/envoy/accesslog.go#L19).
The configuration will be extended to also accept values in the form of: `field_name="literal string with Envoy field operator"`.

The value will be parsed in the following manner:

* Everything before the first `=` character will be treated as the JSON field name
* Everything after the first `=` character will be treated as Envoy field [format command][1]

If there is no `=`, then the whole value is the field name. The field will be translated using the already defined translation map.

### Validation

The Envoy field command will be validated to prevent Envoy misconfiguration.
The validation will be performed using regular expressions.
These expressions will be similar to [one used in Envoy](https://github.com/envoyproxy/envoy/blob/4d77fc802c3bc1c517e66c54e9c9507ed7ae8d9b/source/common/formatter/substitution_formatter.cc#L291), but with the specification of exact commands.
If the validation is failed an error will be raised and printed in the logs of Contour during startup.

### Edge Cases

#### Field name is repeated

If the field name is repeated the last entry in the configuration takes precedes and overwrites the previous value.

#### Unknown field name in old format

If the field name is not defined in the default fields transition map and the user doesn't specify the Envoy field format, then Contour will raise a validation error.
Currently, Contour silently ignores unknown fields (see [#1507](https://github.com/projectcontour/contour/issues/1507)).

#### Additional built-in translations

For newly supported non-parameterized methods like `%RESPONSE_DURATION%`, we will add them to the built-in translation mapping.

### Example

This Contour configuration contains both old- and new-style fields:

```yaml
json-fields:
   - @timestamp
   - method
   - response_duration
   - content-id=%REQ(X-CONTENT-ID)%
```

The following is the resulting JSON config for Envoy:

```json
{
   "timestamp": "%START_TIME%",
   "method:" "%REQ(:METHOD)%",
   "response_duration": "%RESPONSE_DURATION%",
   "content-id": "%REQ(X-CONTENT-ID)%"
}
```

## Alternatives Considered

### User-defined Translation Table
As implemented in [#3033](https://github.com/projectcontour/contour/pull/3033), another approach is to allow users to specify an additional translation table.
This mirrors current default translation table, but allows users to define custom translations.
Validation could be done in two stages:
  1. Validate that all the right-hand-sides of translations appear valid.
  2. Validate that all entries in `json-fields` are in the default or user-defined translation tables.
However, [#3033](https://github.com/projectcontour/contour/pull/3033) does not include validation logic.
Having a user-defined translation table makes adding a field like `content-id` require changes in two places (one to define `content-id` in the translation table and one to add it to the `json-fields` list).

In this alternative, the example configuration would be written as the following:
```yaml
json-fields:
   - @timestamp
   - method
   - response_duration
   - content-id
extra-json-fields:
   content-id: %REQ(X-CONTENT-ID)%
```

### Map Syntax
The proposed implementation would require parsing each value to split at the first `=`.
Instead of using an `=` within the string to separate a key and value, the Contour configuration could be a YAML key and value.
In this approach, the example configuration would look like the following:
```yaml
json-fields:
   @timestamp:
   method:
   response_duration:
   content-id: %REQ(X-CONTENT-ID)%
```

This approach is not backward-compatible because the structure of `json-fields` is changing from an array to a map.
In order to make this work, a new field would need to be introduced so that existing configuration would work.
Then, the original `json-fields` key would be deprecated over several versions.
The built-in translations would continue to work as before (just with empty values in the new map syntax).

The other drawback of this approach is aesthetic: it could be argued that empty values for the keys with default translations is ugly.

## Security Considerations
Misconfiguration of the property can leak unwanted information to the access log file.

## Compatibility
Both original and alternatives high-level designs don't introduce breaking changes.
The current implementation skips unknown fields so any new custom fields will be ignored.

## Open Issues
* Is one of the alternative approaches preferable?
   - [User-defined Translation Table](#user-defined-translation-table) seems easier to implement because it was partially implemented in #3033 and because the configuration is more structured and easier to work with as there no need for additional string parsing.
   - [Map Syntax](#map-syntax) is not backward-compatible, but does avoid string parsing.

[1]: https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/access_log/usage#command-operators
