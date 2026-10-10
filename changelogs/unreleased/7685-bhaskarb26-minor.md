## HTTPProxy now supports emitting the x-envoy-attempt-count header

HTTPProxy virtual hosts gain two new optional fields, `includeRequestAttemptCount`
and `includeAttemptCountInResponse`. When enabled, Envoy includes the
`x-envoy-attempt-count` header in requests forwarded to the upstream (and,
optionally, in responses returned to the downstream client). The header value
starts at 1 for the initial attempt and is incremented for each retry, letting
upstream services and clients observe how many times a request was attempted.
