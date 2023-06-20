## Routes with HTTP Method matching have higher precedence

For conformance with Gateway API v0.7.1+, routes that utilize HTTP method matching now have an explicit precedence over routes with header/query matches.
See the [Gateway API documentation](https://github.com/kubernetes-sigs/gateway-api/blob/v0.7.1/apis/v1beta1/httproute_types.go#L163-L167) for a description of this precedence order.

This change applies not only to HTTPRoute but also HTTPProxy method matches (implemented in configuration via a header match on the `:method` header).
