# Header Based Routing

_Status_: Draft

The goal of this feature is to allow Contour to route traffic by more than just fqdn or path matching, but by also routing based upon a header which exists on the request.

## Goals

- Allow traffic routing based upon a set of `key`/`value` pairs in the request
- Apply Header routing per route
- Support postitive & negative header matching

## Non Goals

- Utilize regex for defining `key`/`value` pairs
- IngressRoute delegation will still be implemented based upon path matching

## Background

In addition to path based routing, specifying a set of headers that the route should match on allows the router to check the request’s headers against all the specified headers in the route config.
A match will happen if all the headers in the route are present in the request with the same values (or based on presence if the value field is not in the config).

## High-Level Design

Allowing Header based routing requires changes to IngressRoute to allow for a spec to define the key/value pairs to match against.
This change updates how IngressRoute is structured to allow for specifying both path and header per route match.

Routes passed to Envoy from Contour will also optionally implement the `HeaderMatcher` field which is where the Headers defined on the IngressRoute will be passed.

## Detailed Design

The IngressRoute spec will be updated to allow for a `header` field to be added which will allow for a set of key/value pairs to be applied to the route.
In the following example, you can define for path `/foo` and send traffic to different services for the same path but which contain different headers.

```
spec:
  routes:
  - match: /foo
      header:
        name: "x-header"
        value: "A"
    services:
    - name: backend-A
        port: 9999
  - match: /foo
      header:
         name: "x-header"
         value: "B"
    services:
    - name: backend-B
        port: 9999
  - match: /foo
    services:
    - name: backend-B
        port: 9999
```

The `internal/dag/Route` will be updated to add a `Headers map[string]string` which will store the values defined in the IngressRoute.

The `envoy/route` will be updated to specify the headers defined previously in IngressRoute.
This new feature will utilize a `prefix_match` when defining the `HeaderMatcher` field, meaning the match will be performed based on the prefix of the header value.
Contour will need to create different Envoy routes based upon the path+header combination since the header matching is defined on the top level Route object (https://github.com/envoyproxy/go-control-plane/blob/master/envoy/api/v2/route/route.pb.go#L941-L947).

## Alternatives Considered

Routing based upon just the Header, but this would require a rethink as to how delegation could be implemented and how the DAG is built.

## Security Considerations

None.