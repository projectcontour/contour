# projectcontour.io/IngressRoute/v1 Design

_Status_: Draft

The Ingress object was added to Kubernetes in version 1.2 to describe properties of a cluster-wide reverse HTTP proxy. Since that time, the Ingress object has not progressed beyond the beta stage, and its stagnation inspired an explosion of annotations to express missing properties of HTTP routing.

When IngressRoute was introduced a year ago (July 2018) the design chose to only support prefix matching for routes. This was mostly a time to market decision, but also reflected the fact that it was unknown the other ways customers wanted to match routes on. We knew that Envoy also supported other methods, but without a signal from our userbase, blindly adding support for all of the existing mechanisms felt like throwing the problem over the wall to our users to figure out what worked best.

It's clear today that only supporting prefix routing is too limited. Customers want to route not just on prefix, substring, and regex -- the three we identified last year -- but also header matching, source ip, user agent, and many more.

The scope of this design doc looks to improve on the IngressRoute.v1beta1 design and plan how the current design will change to support these additional routing features, yet still preserve the current multi-team capabilities with delegation.

## Header Routing

Routing via Header allows Contour to route traffic by more than just fqdn or path matching, but by also routing based upon a header which exists on the request. The scope of this design doc looks to improve on the IngressRoute.v1beta1 design and incorporate changes to enable not just path based routing, but also header based routing as a first class citizen.

### Use Cases

Contour was initially developed with prefix based routing and has served us well, but soley prefix based routing has shown to be underpowered and we have customers, external and internal, who want more flexible routing options.

- Allow for routing of platform to different backends. When a request come from an iOS device it should route to "backend-iOS" and requests from an Android device should route to "backend-Android". Each of these backends are managed by different teams and they live in seperate namespaces.

- Requests to a specific path `/weather` are handled by a backend, however, specific users are allowed to opt-in to a beta version.
  When authenticated, an additional header is added which identifies the user as a member of the "beta" program. Requests to the `/weather` path with the appropriate header (e.g. `X-Beta: true`) will route to `backend-beta`, all other requests will route to `backend-prod`.

- Similar to a opt-in beta version previously described, another header routing option is to route specific tenants to different backends.
  This may be to allows users to have a high tier of compute or run in segregated sections to limit where the data lives.
  When authenticated, an additional header is added which identifies the user as part of OrgA.
  The request is routed to the appropriate backend matching the user's organization membership.

- API Version Numbers in the header: Using a header to route requests to different backends based upon the header value (e.g. `apiversion: v2.0` vs `apiversion:v2.1-beta`)
  
- Does an Auth header exist in the request regardless of the value?

- Some headers will require a regex style expression for matching. If user wanted to target Chrome browsers, here's a sample header. We'd need to specify the `Chrome` bit out of the header value:

  ```bash
  Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.169 Safari/537.36
  ```

- JWT Token routing based upon claims in the token. Difficult currently with Envoy today (https://github.com/envoyproxy/envoy/issues/3763).

## Issues with current v1beta1.IngressRoute Spec

- **Delegate IngressRoute can also "look" like a root IngressRoute** (https://github.com/heptio/contour/issues/865): Currently, if I have an IngressRoute that delegates to another, it's possible that the delegated IngressRoute also references a virtualhost even though that virtualhost will be ignored
- **Permit Insecure in multi-team environments** (https://github.com/heptio/contour/issues/864): When a TLS cert is defined on an `IngressRoute`, Contour will automatically configure Envoy to return a 301 redirect response to any insecure request. Users can then optionally allow insecure traffic by setting the `permitInsecure` field on a route. This can introduce a security risk since administrators may not want to allow users who have been delegated, to be able to serve insecure routes from a Root delegation.
- **TCP Proxying** required a `routes` struct which felt out of place.
- No support for routing on anything other than a `Path Prefix`

## Goals

- **<u>Overall</u>**:
  - Support multi-team clusters, with the ability to limit the management of routes to specific virtual hosts.
  - Support delegating the configuration of some or all the routes for a virtual host to another Namespace
  - Create a clear separation between singleton configuration items--items that apply to the virtual host--and sets of configuration items--that is, routes on the virtual host.
- **<u>Routing</u>**:
  - Support path prefix based routing: The prefix specified must match the beginning of the *:path* header.
  - Make routing decisions can be made against following criteria:
    - Set of header `key`/`value` pairs in the request
    - Set of header `key`/`value` pairs in the request & Path Prefix combination
  - Support postitive & negative header matching via Regex matching
    - This regex string is a regular expression rule which implies the entire request header value must match the regex.
      The rule will not match if only a subsequence of the request header value matches the regex.
      The regex grammar used in the value field is defined [here](https://en.cppreference.com/w/cpp/regex/ecmascript).
  - Allow delegation from fqdn to header or path
    - Sub-delegation to additional header or path
  - Secure Backend: The idea to proxy to a backend service over a TLS connection  (Currently an annotation on the Kubernetes service)
  - Backend Protocol: Proxy to http/2 backend (Currently an annoation on the Kubernetes service)

## Non Goals

- **<u>Overall</u>**:
  - Deprecate Contour's support for v1beta1.Ingress.
  - Support Ingress objects outside the current cluster.
  - IP in IP or GRE tunneling between clusters with discontinuous or overlapping IP ranges.
- **<u>Routing:</u>**
  - Support JWT tokens with header routing

## High-Level Design

At a high level, this document proposes modeling ingress configuration as a graph of documents throughout a Kubernetes API server, which when taken together form a directed acyclic graph (DAG) of the configuration for virtual hosts, request headers, and their constituent routes.

The v1.IngressRoute design looks to add header based routing requiring changes to v1beta1.IngressRoute to allow for a spec to define the key/value pairs to match against. This change updates how IngressRoute is structured to allow for specifying both path and header per route match. 

### Delegation

The working model for delegation is DNS. As the owner of a DNS domain, for example `.com`, I *delegate* to another nameserver the responsibility for handing the subdomain `heptio.com`. Any nameserver can hold a record for `heptio.com`, but without the linkage from the parent `.com` TLD, its information is unreachable and non authoritative.

Each *root* of a DAG starts at a virtual host, which describes properties such as the fully qualified name of the virtual host, TLS configuration, and possibly global access list details. The vertices of a graph do not contain virtual host information. Instead they are reachable from a root only by delegation. This permits the *owner* of an ingress root to both delegate the authority to publish a service on a portion of the route space (path or request header) inside a virtual host, and to further delegate authority to publish and delegate.

In practice the linkage, or delegation, from root to vertex, is performed with a specific type of route action. You can think of it as routing traffic to another ingress route for further processing, instead of routing traffic directly to a service.

## Detailed Design

### Changes from v1beta1

Here are some changes that are directly different from the v1beta1 design:

- `services` under the `match` struct are renamed to `backends`
- `services` struct previously had a `name` parameter, now it has a set of options which define the protocol to implement for that specific backend
- `match` previously took a string arguement which was the pathPrefix to route on, now this has been moved to a `path` parameter under the `match` struct
- `tcpproxy`: Is removed and replaced by a `supported protoco` defined in the route
- Annotations on Kubernetes services are removed which previously defined what ports to proxy TLS connections to, etc
- **TODO**: Investigate moving from `spec.status` to the built-in [status sub resource type](https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#status-subresource).
- **retryPolicy** renamed to `failurePolicy`

### Routes

Routes in v1beta1 allows for a set of matches on a path as well as a set of upstream Kubernetes services that traffic could be routed. These upstreams assumed an l7 http protocol+port unless [special annotations](https://github.com/heptio/contour/blob/master/docs/annotations.md#contour-specific-service-annotations) were added to the corresponding Kubernetes service. These mappings needed to be tightly coupled and were often overlooked by users.

This design looks to update how services are defined by specifying the protocol in the service reference set. By defining the protocol:service name as well as the port, we can eliminate the need for annotations on the service.

<u>Supported Proctocols:</u>

- **http**: l7 insecure http proxy
- **https**: l7 secure (tls) http proxy
- **http2**: l7 insecure http2 proxy
- **https2**: l7 secure http2 proxy
- **tcp**: TLS encapsulated TCP sessions

#### Example:

```yaml
routes: 
  - match: /
    backends: 
    - http: kuard
      port: 80
    - https: kuard-tls
      port: 443
    - http2: kuard-2
      port: 80
    - https2: kuard-2-tls
      port: 443
```

**NOTE**: *Implementing specific protocols in the services section eliminates the `tcpproxy` field which previously implemented tcp proxying*

## Header Routing

The IngressRoute spec will be updated to allow for a `header` field to be added which will allow for a set of key/value pairs to be applied to the route. Additionally, the path `match` moves to it's own `path` variable.

**NOTE**: *Routing on a `header` value* still requires a path prefix to be specified. If pure header delegation is requested, then Contour will configure a wildcard path to match all paths resulting in a header match only across any path.

#### Header Values

IngressRoutev1 will support routing from various headers. It will implement commonly used headers such as user-agent & cookie since those will require a regex style implementation. By implementing them as speific types, we can apply those regex's automatically.

- **header:** This is a generic header type and allows for matching custom headers
- **cookie**: This is a built-in type and matches a `cookie` in the header while being implemented as a "contains" regex
- **user-agent**: This is a built-in type and matches the `user-agent` in the header while being implemented as a "contains" regex

### Path Match with Headers

In the following example, you can define for path `/foo` and send traffic to different services for the same path but which contain different headers. This example shows how to have 2 routes match specific headers and a last route match a request without specific headers.

```yaml
spec:
  routes:
  - match:
      path: /foo
      - header: x-header
        value: a
    services:
    - http: backend-a
      port: 9999
  - match: 
      path: /foo
      - header: x-header
        value: b
    services:
    - http: backend-b
      port: 9999
  - match: 
    path: /foo
    services:
    - http: backend-default
      port: 9999
```

#### Requests

Following are sample requests and which backends will handle the request:

- `GET -H "x-header: a" /foo` —> `backend-a`
- `GET -H "x-header: b" /foo` —> `backend-b`
- `GET /foo` —>  `backend-default`

### Path Match with Cookie

In the following example, you can route any request to path `/foo` containing a cookie containing `user=test` to the backend named `cookiebackend` over port `80`.

```yaml
spec:
  routes:
  - match:
      path: /foo
      - cookie: user=test
      services:
      - http: cookiebackend
        port: 80    
```

### Header Delegation

Currently with IngressRoute, path prefixes of a request can be delegated to teams, users, namespaces, within a single Kubernetes cluster. Additionally, this design looks to add headers as a mechanism to pass authority off to teams, users or namespaces.

The following example shows how requests to a specific path with a specific header can be delegated to a team in another namespace as well as how requests that don't match the headers specified previously will be handled in the current namespace.

```yaml
spec:
  routes:
  - match:
      path: /foo
      - header: x-header
        value: a
    delegate:
      name: headera-ir
      namespace: team-a
  - match: 
      path: /foo
      - header: x-header
        value: b
      - user-agent: Chrome
    delegate:
      name: headerb-ir
      namespace: team-b
  - match: 
    path: /foo
    services:
    - http: backend-default
      port: 9999
```

#### Requests

Following are sample requests and which backends will handle the request:

- `GET -H "x-header: a" /foo` —> `backend-a.team-a`
- `GET -H "x-header: b" /foo` —> `backend-b.team-b`
- `GET /foo` —>  `backend-default`

### Path Match with Headers (no path)

The following example demonstrates how to send traffic to different services for any path but which contain different headers. This example shows how to have 2 routes match specific headers.  

**Note:** *The missing `path` match defined here. Since we're only defining the header piece of the route match, Contour will insert a wildcard path match to satisfy the reverse proxy system.* 

```yaml
spec:
  routes:
  - match:
      - header: x-header
        value: a
    services:
    - http: backend-a
      port: 9999
  - match: 
      - header: x-header
        value: b
    services:
    - http: backend-b
      port: 9999
```

#### Requests

Following are sample requests and which backends will handle the request:

- `GET -H "x-header: a" /****` —> `backend-a`
- `GET -H "x-header: b" /****` —> `backend-b`

### Request failures

Currently, a `retryPolicy` has been implemented already which in the event of upstream request failures, the request will be retried automatically. This field will be renamed to `failurePolicy` and will implement both the current `retryPolicy` as well as a passive health check. 

A passive health check allows for health checking of an upstream endpoint, if an upstream host returns some number of consecutive 5xx, it will be ejected. When a `failurePolicy` is defined it will automatically implement the passive health check unless otherwise disabled. Similarly, the `retryPolicy` is automatically enabled unless otherwise specified.

```yaml
spec:
  virtualhost:
    fqdn: timeout.bar.com
  routes:
  - match: 
    path: /
    timeoutPolicy:
      request: 1s
    failurePolicy:
      count: 3
      perTryTimeout: 150ms
      passiveHealthCheckEnabled: true # <--- defaults to true
      retryPolicyEnabled: true # <---- defaults to true
      
```



### DAG Implementation

The `internal/dag/Route` will be updated to add a `Headers `struct which will store the values defined in the IngressRoute. The path match moves to its own spec within the `Route` struct.

The `envoy/route` will be updated to specify the headers defined previously in IngressRoute. This new feature will utilize a `prefix_match` when defining the [HeaderMatcher](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#route-headermatcher) field, meaning the match will be performed based on the prefix of the header value. Contour will need to create different Envoy routes based upon the path+header combination since the header matching is defined on the top level Route object (https://github.com/envoyproxy/go-control-plane/blob/master/envoy/api/v2/route/route.pb.go#L941-L947).

### Flow Diagrams

There are some diagrams showing how network traffic can flow through various scenarios here: https://docs.google.com/drawings/d/1Pxqfki0TkrUPJMVmiq2lUGiXAmqrRATRmJv9qiS1dtM/edit?usp=sharing

## Alternatives Considered

- Do not use a DAG to implement the discovered routes. This makes the design more flexible, but requires much more testing to validate the use-cases are properly covered with delegation.

## Security Considerations

- If the delegation is not properly implemented, there could be ways that users get access to portions of the host requests which they should not have access. This will need to be mitigated with proper testing and validation of the design implementation.

# Features to plan for?

List of features that we should think about implementing 

- Allow/Disallow IP's: Set of IP's that can or cannot access a rootIngressRoute (and thus the corresponding delegates). This could be imlemented via RateLimiting