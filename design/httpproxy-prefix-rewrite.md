# Request Rewrite For HTTPProxy

**Status**: _Accepted_

This document specifies a design for supporting URL prefix rewriting in the HTTPProxy CRD.

## Goals

Envoy currently supports [path prefix][2] and [host header][3] rewriting.
It is likely that future Envoy releases will support [regex path](https://github.com/envoyproxy/envoy/issues/2092) rewriting.

The goal of this proposal is to eventually allow the user to access Envoy URL rewriting features from the HTTPProxy CRD.
The initial implementation target is path prefix rewriting, but the API should be specified such that it can be gracefully extended to other forms of request rewriting.

## Non-Goals

* Addressing rewrite limitations. Request rewriting in Envoy has
  very limited capabilities. The [prefix_rewrite][4] action only
  alters the prefix of a URL path. Since it does not alter response
  headers or content, it is not possible to use this feature to
  re-host a web application at a path prefix it is not expecting.

* Controlling request routing. In some proxies (particularly NGINX),
  rewriting the request URL can also restart the request routing
  machinery. In this proposal, requests are always fully routed
  before any changes are made to the URL (this is required by Envoy
  semantics).

## Background

Request rewriting is ultimately specified as properties of the Envoy [RouteAction](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-msg-route-routeaction) object.
The CRD semantics should avoid encouraging uses that can't be supported by Envoy.

The Envoy `prefix_rewrite` configuration is coupled to prefix matching on the route.
Each `prefix_rewrite` specification needs to be attached to a single prefix match, so each new `prefix_rewrite` needs a new prefix match.

HTTPProxy documents are organized into a collection of trees, where the root is a document that has a `virtualhost` field, and the leaves are documents that have a `routes` field.
A HTTPProxy tree could be a single document (that is both a root and a leaf), or it could be a series of documents in different namespaces, connected by the `includes` field.

HTTPProxy inclusion semantics allow a single HTTPProxy CRD to be included by more than one root HTTPProxy document.
This means that multiple routing path prefixes can end up referring to the same HTTPProxy.

To reason about most kinds of request rewriting, the user needs to know the final form of the request.
In other words, it doesn't make sense to want to rewrite a path prefix unless you have some expectations about the path prefix you will receive.
This implies that even if the owner of a leaf HTTPProxy doesn't control the root or intermediate documents, they still have fairly concrete expectations around the full URL path to expect.

Although there are a number of ways to conceptualize path prefix rewriting, path regex rewriting requires knowledge of the full request path at the time of writing the regex.
This is because a regex rewrite can be used to re-order arbitratry portions of the URL path, and this can't be done unless the regex is defined as being applied to the complete path.

## High Level Design

Add a `PathRewrite` field to the HTTPProxy [Route](https://github.com/projectcontour/contour/blob/main/apis/projectcontour/v1/httpproxy.go#L125) object.
This allows us to specify different kinds of rewrite for URL paths without also needing to specify a specific kind of request rewriting (i.e. we can add host rewrite later).

Locating the `PathRewritePolicy` on the `Route` means that rewriting is controlled by the team that controls the leaf HTTPProxy document.
This allows a useful initial implementation, but does not prevent adding `PathRewritePolicy` fields higher up the HTTPProxy tree (though allowing that would require detecting and resolving conflicts).

```Go
type Route struct
	...

	// The policy for rewriting the path of the request URL
	// after the request has been routed to a Service.
        //
        // +kubebuilder:validation:Optional
	PathRewrite *PathRewritePolicy `json:"pathRewrite,omitempty"`
}

```

The `PathRewritePolicy` struct initially only supports path prefix replacement.
Creating a policy struct allows future addition of other kinds of path rewriting.

```Go
// PathRewritePolicy specifies how a request URL path should be
// rewritten. This rewriting takes place after a request is routed
// and has no subsequent effects on the proxy's routing decision.
// No HTTP headers or body content is rewritten.
//
// Exactly one field in this struct may be specified.
type PathRewritePolicy struct {
    ReplacePrefix []struct {
        // Prefix specifies the URL path prefix to be replaced.
        //
	// If Prefix is specified, it must exactly match the Condition
	// prefix that is rendered by the chain of including HTTPProxies
	// and only that path prefix will be replaced by Replacement.
	// This allows HTTPProxies that are included through multiple
	// roots to only replace specific path prefixes, leaving others
	// unmodified.
        //
        // If Prefix is not specified, all routing prefixes rendered
        // by the include chain will be replaced.
        //
        // +kubebuilder:validation:Optional
        // +kubebuilder:validation:MinLength=1
        Prefix string `json:"prefix,omitempty"`

	// Replacement is the string that the routing path prefix
	// will be replaced with. This must not be empty.
        //
        // +kubebuilder:validation:Required
        // +kubebuilder:validation:MinLength=1
        Replacement string `json:"replacement"`
    } `json:"replacePrefix,omitempty"`
}
```

`PrefixRewrite` is a slice to allow a service to rewrite multiple path prefixes, depending on the shape of the HTTPProxy tree.
As discussed above, a HTTPProxy leaf can be reached from multiple HTTPProxy roots via different paths.
This implies that service owners could reasonably need to rewrite multiple routing paths to a canonical version.
This, in turn, implies that more than one prefix may need to be rewritten.

For example, assume that a service support multiple versions of an API and is using the routing prefixes `/v1/api` and `/v2/api`.
This can only be rewritten to `/api/v1` and `/api/v2` with two separate replacements.
Distinguishing between these replacements requires specifying the routing prefix.

* Prefix: This is the optional path prefix to match on.
  If this is not specified, the replacement is applied for every routing match prefix.

* Replacement: This is the string that replaces the path prefix. If a Prefix field is also specified, the replacement is only applied to the exact route prefix match.

Note that the replacement prefix must be an exact match to exactly one routing prefix.

## Implementation Notes

### Wildcard paths

Prefix replacement is incompatible with wildcard paths. This is
because wildcard paths will generate a Envoy [regex_match][1]
object but the Envoy prefix rewrite feature requires a [prefix][2]
and only one kind of path match can be used at a time.

This means that Contour must validate that prefix replacement may
not be used in conjunction with wildcard path matching.

### Prefix fragility

Since a replacement prefix must be an exact match to a single
routing prefix, a rewrite configuration can be broken by changes
in parent HTTPProxies or by deployment ordering. We can mitigate
this by not making the lack of a Prefix match a hard validation
error. In this case, we would want richer status information to
indicate that some path rewrites are not being used.

### Path component '/' ambiguity

This proposal does nothing to help users with the problem of
specifying trailing '/' characters correctly. If the prefix doesn't
end in a '/', then using a replacement of '/' can result in a
final path with a leading '//'.

The canonical Envoy solution to this is to add a [prefix match][2]
for both '/foo' and '/foo/' and perform the rewrite in both cases.
This approach seems plausible, though we would currently allow these
two cases to be routed to different clusters.

This table shows the rewritten path prefixes for matching on both
`/foo` and `/foo/` for two possible replacements.

| Replacement | Client Path | Rewritten Path |
|-------------|-------------|----------------|
| `/bar`      | `/foosball` | * `/barsball`  |
| `/bar`      | `/foo/type` |   `/bartype`   |
| `/bar/`     | `/foosball` |   `/bar/sball` |
| `/bar/`     | `/foo/type` | * `/bar/type`  |

In this case, the desired result needs two different replacements,
so we need to apply special handling to add or remove the trailing
`/` on the replacement depending on which match prefix we end up
generating.

In the case that the user has already specified distinct routes for
`/foo` and `/foo/`, we should not generate any implicit prefix
matches or rewrites. Contour should build the Envoy route config
exactly as the user specifies.

### Empty replacements

Empty replacements are not allowed.

### Relative replacements

Relative replacements (i.e. that do not begin with '/' are not allowed).

### Multiple replacements

Rather than allowing a `ReplacementPrefix` slice, we could
obtain similar semantics by requiring the user to always
specify separate leaf proxies for each rendered prefix. This
locally simplifies the API, but arguably forces more complex
deployment topologies. If we can expose richer status
information, then I think the additional API complexity
is preferable.

## Examples

Rewrite a single prefix:

```
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpbin
spec:
  virtualhost:
    fqdn: httpbin.jpeach.org
  routes:
  - conditions:
    - prefix: /
    services:
    - name: httpbin
      port: 80
    pathRewrite:
      replacePrefix:
      - replacement: /my/great/replacement
```

In this example, an application receives routes `/v1/` and `/v2`
and rewrites them both to a single canonical `/v3/` path:

```
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpbin-vhost
spec:
  virtualhost:
    fqdn: httpbin.jpeach.org
  includes:
  - name: httpbin-app
    conditions:
    - prefix: /v1/
  - name: httpbin-app
    conditions:
    - prefix: /v2/
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpbin-app
spec:
  routes:
  - services:
    - name: httpbin
      port: 80
    pathRewrite:
      replacePrefix:
      - replacement: /v3/
```

Taking the previous example, imagine that the application initially
shipped with an unversioned `/` route which we need to rewrite to
`/v1/`, but other prefixes should still get replaced by `/v3/`. We
can solve this by specifying the prefix field so that the replacement
only applies to the `/` route.

```
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpbin-vhost
spec:
  virtualhost:
    fqdn: httpbin.jpeach.org
  includes:
  - name: httpbin-app
    conditions:
    - prefix: /v1/
  - name: httpbin-app
    conditions:
    - prefix: /v2/
  - name: httpbin-app
    conditions:
    - prefix: /
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpbin-app
spec:
  routes:
  - services:
    - name: httpbin
      port: 80
    pathRewrite:
      replacePrefix:
      - replacement: /v3/
      - prefix: /
        replacement: /v1/
```

In this example, we rewrite multiple included proxies to flip the
components of a prefix:

```
---
# Create a root proxy that includes a proxy on the /api/ path:
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: artifactory
spec:
  virtualhost:
    fqdn: artifactory.jpeach.org
  includes:
  - name: artifactory-api

---
# Now split the root into 2 proxies for /v1/ and /v2/.
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: artifactory-api
spec:
 includes:
  - name: artifactory-v1
    conditions:
    - prefix: /v1/token/
  - name: artifactory-v2
    conditions:
    - prefix: /v2/token/

---
# Now, it turns out we don't need a separate proxy for /v1/ any
# more, so we can send that traffic over to /v2/
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: artifactory-v1
spec:
 includes:
  - name: artifactory-v2

---
# Now, rewrite the token path for different versions. We configure
# multiple services to justify specifying this as a single proxy
# with multiple replacements.
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: artifactory-v2
spec:
  routes:
    services:
    - name: artifactory-prod
      port: 80
      weight: 95
    - service: artifactory-dev
      port: 80
      weight: 4
    - service: artifactory-sampler
      port: 80
      weight: 1
      mirror: true
    pathRewrite:
      replacePrefix:
      - prefix: /v1/token/
        replacement: /artifactory/api/v1/token
      - prefix: /v2/token/
        replacement: /artifactory/api/v2/token
```

### Future Extensions

The proposal here is compatible with a number of future extensions:

* Regex path rewrites can be added as a new field in the `PathRewritePolicy` struct.
* Host header rewrites can be added as a new policy field in the `Route` struct, following similar semantics to this proposal.
* General purpose header rewriting can be added as a new policy field in the `Route` struct, following similar semantics to this proposal.
* Nothing in this proposal prevents adding prefix replacement specifications to non-leaf HTTPProxies.

[1]: https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routematch-safe-regex
[2]: https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routematch-prefix
[3]: https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-host-rewrite
[4]: https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#envoy-api-field-route-routeaction-prefix-rewrite
