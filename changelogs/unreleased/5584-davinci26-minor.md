## Adds support for treating missing headers as empty when they are not present as part of header matching

`TreatMissingHeadersAsEmpty` specifies if the header match rule specified header does not exist, this header value will be treated as empty. Defaults to false.
Unlike the underlying Envoy implementation this is **only** supported for negative matches (e.g. NotContains, NotExact).
