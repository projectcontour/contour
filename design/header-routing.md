# Header Based Routing

_Status_: Draft

The goal of this feature is to allow Contour to route traffic by more than just fqdn or path matching, but by also routing based upon a header which exists on the request.
The scope of this design doc looks to improve on the IngressRoute.v1beta1 design and incorporate changes to enable not just path based routing, but also header based routing as a first class citizen.

## Use Cases

Contour was initially developed with prefix based routing and has served us well, but soley prefix based routing has shown to be underpowered and we have customers, external and internal, who want more flexible routing options.

- Allow for routing of platform to different backends. When a request come from an iOS device it should route to "backend-iOS" and requests from an Android device should route to "backend-Android". Each of these backends are managed by different teams and they live in seperate namespaces.

- Requests to a specific path `/weather` are handled by a backend, however, specific users are allowed to opt-in to a beta version.
  When authenticated, an additional header is added which identifies the user as a member of the "beta" program. Requests to the `/weather` path with the appropriate header (e.g. `X-Beta: true`) will route to `backend-beta`, all other requests will route to `backend-prod`.

- Similar to a opt-in beta version previously described, another header routing option is to route specific tenants to different backends.
  This may be to allows users to have a high tier of compute or run in segregated sections to limit where the data lives.
  When authenticated, an additional header is added which identifies the user as part of OrgA.
  The request is routed to the appropriate backend matching the user's organization membership.

- Some headers will require a regex style expression for matching. If user wanted to target Chrome browsers, here's a sample header. We'd need to specify the `Chrome` bit out of the header value:

  ```
  Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/74.0.3729.169 Safari/537.36
  ```

## Goals

- Make routing decisions can be made against following criteria:
  - Set of header `key`/`value` pairs in the request
  - Set of header `key`/`value` pairs in the request & Path Prefix combination
- Support postitive & negative header matching via Regex matching
  - This regex string is a regular expression rule which implies the entire request header value must match the regex.
  The rule will not match if only a subsequence of the request header value matches the regex.
  The regex grammar used in the value field is defined [here](https://en.cppreference.com/w/cpp/regex/ecmascript).
- Allow delegation from fqdn to header or path
  - Sub-delegation to additional header or path

## Non Goals

- 

## High-Level Design

Allowing Header based routing requires changes to IngressRoute to allow for a spec to define the key/value pairs to match against.
This change updates how IngressRoute is structured to allow for specifying both path and header per route match.
In addition routing via header, delegation will be added such that an IngressRoute can delegate IngressRoutes via a set of headers.

Routes passed to Envoy from Contour will also optionally implement the Envoy `HeaderMatcher` field which is where the Headers defined on the IngressRoute will be passed.

## Detailed Design

The IngressRoute spec will be updated to allow for a `header` field to be added which will allow for a set of key/value pairs to be applied to the route. Additionally, the path `match` moves to it's own `path` variable.

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
    - name: backend-a
        port: 9999
  - match: 
      path: /foo
      - header: x-header
          value: b
    services:
    - name: backend-b
        port: 9999
  - match: 
    path: /foo
    services:
    - name: backend-default
        port: 9999
```

#### Requests

Following are sample requests and which backends will handle the request:

- `GET -H "x-header: a" /foo` —> `backend-a`
- `GET -H "x-header: b" /foo` —> `backend-b`
- `GET /foo` —>  `backend-default`

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
      - header: user-agent
          value: Chrome  # <--- This requires a regex style expression
    delegate:
      name: headerb-ir
      namespace: team-b
  - match: 
    path: /foo
    services:
    - name: backend-default
        port: 9999
```

#### Requests

Following are sample requests and which backends will handle the request:

- `GET -H "x-header: a" /foo` —> `backend-a.team-a`
- `GET -H "x-header: b" /foo` —> `backend-b.team-b`
- `GET /foo` —>  `backend-default`

### DAG Implementation

The `internal/dag/Route` will be updated to add a `Headers `struct which will store the values defined in the IngressRoute.
The path match moves to its own spec within the `Route` struct.

The `envoy/route` will be updated to specify the headers defined previously in IngressRoute.
This new feature will utilize a `prefix_match` when defining the [HeaderMatcher](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#route-headermatcher) field, meaning the match will be performed based on the prefix of the header value.
Contour will need to create different Envoy routes based upon the path+header combination since the header matching is defined on the top level Route object (https://github.com/envoyproxy/go-control-plane/blob/master/envoy/api/v2/route/route.pb.go#L941-L947).

### Flow Diagrams

There are some diagrams showing how network traffic can flow through various scenarios here: https://docs.google.com/drawings/d/1Pxqfki0TkrUPJMVmiq2lUGiXAmqrRATRmJv9qiS1dtM/edit?usp=sharing

## Alternatives Considered

- Do not allow for Header based delegation, only apply to Path.
This does not solve all user use-cases

## Security Considerations

- If the delegation is not properly implemented, there could be ways that users get access to portions of the host requests which they should not have access.
This will need to be mitigated with proper testing and validation of the design implementation.