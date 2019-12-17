# Header Rewrite For HTTPProxy

**Status**: _Draft_

This document specifies a design for supporting request / response header manipulation in the HTTPProxy CRD.

## Goals

Setting and removing headers from requests and responses at two levels:
1. Per-route (pre-split),
1. Per-cluster (post-split).

## Non Goals

- Support for projecting dynamic values into header values.
- Appending to pre-existing headers (for now).

## Background

There are a number of use cases where having this raw capability is useful, and many are called out in #70.

## High-Level Design

Add a new type: `HeaderPolicy` that captures setting and removing headers.

```Go
// HeaderPolicy defines alterations to the headers being sent to or returned from a service.
type HeaderPolicy struct {
	// Set sets the specified headers replacing any existing values associated with the header names.
	// +optional
	Set []HeaderAddition `json:"set,omitempty"`
	// Remove removes any headers whose name matches those specified.
	// +optional
	Remove []string `json:"remove,omitempty"`
}

// HeaderAddition defines a header key/value pair to be added to those sent to or return from a service.
type HeaderAddition struct {
	// Name is the header key to add.
	Name string `json:"name"`
	// Value is the header value to add.
	Value string `json:"value"`
}
```

This will be added in two flavors (request and response) to both Route (pre-split) and Service (post-split).

```Go
	// RequestHeadersPolicy defines how to set or remove headers from requests.
	// +optional
	RequestHeadersPolicy *HeaderPolicy `json:"requestHeadersPolicy,omitempty"`
	// ResponseHeadersPolicy defines how to set or remove headers from responses.
	// +optional
	ResponseHeadersPolicy *HeaderPolicy `json:"responseHeadersPolicy,omitempty"`
```

## Detailed Design

For the most part, these fields will be directly translated to the following fields in the respective Envoy proto:
 - `RequestHeadersToAdd` (with `append: false`)
 - `RequestHeadersToRemove`
 - `ResponseHeadersToAdd` (with `append: false`)
 - `ResponseHeadersToRemove`

There are two notable exceptions to this translation:
1. `Host` header manipulations must be done via a separate Envoy directive.
1. [`%`-encoded variables](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#custom-request-response-headers) are not supported, so literal `%`'s must be escaped in header values.


## Alternatives Considered

There were a few alternatives discussed around API shapes for general header manipulation in #70, but this shape was chosen to address the various scenarios raised there (e.g. request/response).

There was some discussion of API shapes for `Host` rewriting in #1982.


## Security Considerations

None at this time.