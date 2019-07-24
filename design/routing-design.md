# Routing Design

_Status_: Draft

When IngressRoute was introduced a year ago (July 2018) the design chose to only support prefix matching for routes. This was mostly a time to market decision, but also reflected the fact that it was unknown the other ways customers wanted to match routes on. We knew that Envoy also supported other methods, but without a signal from our userbase, blindly adding support for all of the existing mechanisms felt like throwing the problem over the wall to our users to figure out what worked best.

It's clear today that only supporting prefix routing is too limited. Customers want to route not just on prefix, substring, and regex -- the three we identified last year -- but also header matching, source ip, user agent, and many more.

The scope of this design doc looks to improve on the IngressRoute.v1beta1 design and plan how the current design will change to support these additional routing features, yet still preserve the current multi-team capabilities with delegation.

# Background

There are many different ways to apply routing to L7 HTTP ingress controllers. The first few sections of this design outlines what might be possible with HTTP routing. So that its clear, not all aspects of the available routing options will be implemented. Later on in the implementation sections, we'll walk through which pieces that this design doc looks to implement.

## Virtual Host Mechanisms

This is the top level element in the routing configuration. Once a virtual host decision is made, then any routing mechanisms applied to that are taken into consideration. Contour has previously only suppported an `exact domain` match, meaning the domain specified *must* match exactly, however, there are some other types of vhost mapping that can be applied:

- **Suffix domain wildcards**: `*.foo.com` or `*-bar.foo.com`

- **Prefix domain wildcards**: `foo.*` or `foo-*`

- **Special wildcard**: `*` matching any domain

#### Use Cases

- **User or customer application subdomains:** Allowing a domain per specific customer (e.g. customera.foo.com & customerb.foo.com)
- **Default backend:** If any request for an existing vhost doesn't have a match, Envoy will return a HTTP 404 status code which isn't very user friendly, but allows for a way to have any request that doens't map to a k8s configured Ingress resource to be shown a user friendly backend
- **Feature branch deployments:** When developing applications, enable custom feature branches to have unique URLs for easy testing

## Routing Mechanisms

There are various ways to make routing decisions, as noted previously, Contour was originally designed to support prefix path based routin, however there are many other mechanisms that Contour could support. It's important to note that these routing mechanisms are taking into consideration ***after*** the virtual host routing decision is made:

- **Header:** Routing on an HTTP header that exists in the request (e.g. custom header, clientIP, HTTP method)
  - *Note: Still requires a path to match against*
- **Exact Path:** The route is an exact path rule meaning that the path must exactly match the *:path* header once the query string is removed
- **Regex Path:** The route is a regular expression rule meaning that the regex must match the *:path* header once the query string is removed. The entire path (without the query string) must match the regex
- ~~**Query Parameters**: Specify a set of URL query parameters on which the route should match~~
  - **Note:** *Query parameters could be supported, but are not a goal of this design document*

### Header Routing

Routing via Header allows Contour to route traffic by more than just fqdn or path matching, but by also routing based upon a header which exists on the request.

#### Use Cases

Contour was initially developed with prefix based routing and has served us well, but solely prefix based routing has shown to be underpowered and we have customers, external and internal, who want more flexible routing options.

- Allow for routing of platform to different backends. When a request come from an iOS device it should route to "backend-iOS" and requests from an Android device should route to "backend-Android". Each of these backends are managed by different teams and they live in separate namespaces.

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

### Exact Path / Regex Path Routing

Exact Path based routing makes decisions based upon the path of the request (e.g. `/path`), where the path defined must match explicitly. Regex path routing allows for a regex query to define how the path match is defined.

#### Use Cases

- Match only specific paths (i.e. `/path` should match but `/pathfoo` should ***not***), ensuring that only specified paths are served
- Allow a path to match a dynamic request that is not statically specified (e.g. A path that starts with "a" and has four digits)
- Match a wildcard in the middle of a path. Example: `/app2/*/foo`

## High-Level Design

Contour will add the ability to route via headers by allowing an IngressRoute to define a set of key/value pairs to match against. Contour allows for path prefix based routing, it will add the ability to add regex path based routing. Additional rules & contraints will be added to enable this style of routing since it had a direct impact on how delegation can be implemented. Additionally, Contour will add duffix domain wildcard matching for virtual hosts.

### Delegation

The delegation concept is a key component to enable multi-team clusters as well as provide security for ingress limiting what users can specify within the cluster.

- **Path Prefix (Exists today)**: Path prefix delegation remains unchanged from how it functions in v1beta1
- **Headers:** Delegation with headers is a new option which allows a delegation path matching a key/value header pair
- **Regex Path** delegation is not permitted, but supported from a delegate
  - *The exception to this rule is the use of "glob" style paths*

## Detailed Design

### Header Routing

The IngressRoute spec will be updated to allow for a `header` field to be added which will allow for a set of key/value pairs to be applied to the route. Additionally, the path `match` moves to it's own `path` variable.

- **header:** Key used to match a specific header in the request
- **value**: The value of the header matching the key specified

**NOTE**: *Routing on a `header` value* still requires a path prefix to be specified.

### Path Match with Headers

In the following example, you can define for path `/foo` and send traffic to different services for the same path but which contain different headers. This example shows how to have 2 routes match specific headers and a last route match a request without specific headers.

```yaml
apiVersion: projectcontour.io/v1
kind: IngressRoute
metadata: 
  name: example
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

### Path Match with Cookie

In the following example, you can route any request to path `/foo` containing a cookie containing `user=test` to the backend named `cookiebackend` over port `80`.

```yaml
apiVersion: projectcontour.io/v1
kind: IngressRoute
metadata: 
  name: example
spec:
  routes:
  - match:
    path: /foo
    - header: cookie
      value: user=test
    services:
    - name: cookiebackend
      port: 80    
```

### Header Delegation

Currently with IngressRoute, path prefixes of a request can be delegated to teams, users, namespaces, within a single Kubernetes cluster. This design looks to add headers as a mechanism to pass authority off to teams, users or namespaces.

The following example shows how requests to a specific path with a specific header can be delegated to a team in another namespace as well as how requests that don't match the headers specified previously will be handled in the current namespace.

##### Root IngressRoute Example:

```yaml
apiVersion: projectcontour.io/v1
kind: IngressRoute
metadata: 
  name: example
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
    - name: backend-default
      port: 9999
```

##### Delegated IngressRoute Example:

```yaml
apiVersion: projectcontour.io/v1
kind: IngressRoute
metadata: 
  name: example
spec:
  routes:
  - match:
    path: /foo
    - header: x-header
      value: a
    - name: headerb-ir
      port: 80
  - match: 
      path: /foo
      - header: x-header
        value: b
      - header: user-agent 
        value: Chrome
    services:
    - name: headerb-ir
      port: 80
```

#### Requests

Following are sample requests and which backends will handle the request:

- `GET -H "x-header: a" /foo` —> `backend-a.team-a`
- `GET -H "x-header: b" /foo` —> `backend-b.team-b`
- `GET /foo` —>  `backend-default`

### Exact Path

Contour has support for a prefix-based match which means a request will route if the first part of the defined path matches. Sometimes this is not desired as it allows requests to be routed which may not be desired. 

To enable an exact path match, just leave the trailing slash (`/`) off of the defined path. To enable a prefix-based path match, add a trailing slash (`/`) on the defined path.

*Note: The convention of the trailing slash is subtle, but allows the spec to be cleaner. A more clear approach might be to have a field for the type of match (e.g. `exact` or `prefix`)*

##### Exact-Match:

```yaml
apiVersion: projectcontour.io/v1
kind: IngressRoute
metadata: 
  name: example
spec:
  routes:
  - match:
    path: /app   # <---- Note: NO trailing slash
    services:
    - name: prefix-service
      port: 80
```

##### Prefix-Match:

```yaml
apiVersion: projectcontour.io/v1
kind: IngressRoute
metadata: 
  name: example
spec:
  routes:
  - match:
    path: /app/  # <---- Note: trailing slash
    services:
    - name: prefix-service
      port: 80
```

### Regex Path

Regex path routing allows for a regex query to define how the path match is defined. This is useful since it can be very dynamic and allows for path matches that aren't static. However, due to this flexibility, it makes some aspects of security/routing more difficult since the behavior cannot be predicted easily. Due to this, *delegation of a path will not be supported* in IngressRoute/v1.

Regex paths can be defined and used, however, but they must be in a child delegate when used. Given this requirement, if a user wants to define a regex path, they cannot allow for delegation past the regext expression. 

For example, if a user has `/app2` delegated to them in their namespace. They are free to define further paths (e.g. `/app2/blog`) to satisfy their requirements, however, if they define a regex in their delegated path, then they can no longer define paths after that regex if the new paths match the regex paths. Contour will take the paths and verify if they collide. If a collision is detected, then the corresponding IngressRoute objects status' will be updated.

##### Example:

1. User is delegated: `/app2`
2. User can define a regex in the path: `/app2/bl[a-z]{2}`
3. User defines: `/app2/foo`. Contour checks this is valid!
4. User defines: `/app2/blog`. Conect checks and determines this collides and IngressRoute is disallowed

*Note: Since it's difficult to determine which IngressRoute should be the winner in the decision, Contour should use the most explicit path as the path that gets traffic. So in the previous example, the regex path would be marked as invalid.*

**Note2:** **This also requires a bit of discussion around implementation since there will be a lot tests to validate the logic. Additionally, there will be overhead since the regex's will need to be evaluated.**

##### Example:

```yaml
spec:
  routes:
  - match:
    path: /app2/bl[a-z]{2}  
    services:
    - name: regex-service
      port: 80
```

#### Glob-Style Regex:

A smaller sub-category of regex paths are a "glob" style, meaning `*`. This is simpler to manage since Contour can accuratly predict what matches this regex (e.g. everything!). Given so, glob-style should be able to delegate without issue as long as the `*` is bounded by paths. 

For example, we could define a glob style path: `/app2/*/foo`, but not `/app2/*`. The second example ending with `*` has too large of a match to allow for delegation. If this is encountered, Contour will set the status to be error for the corresponding IngressRoute.

##### Root IngressRoute:

```yaml
apiVersion: projectcontour.io/v1
kind: IngressRoute
metadata: 
  name: example
spec:
  routes:
  - match:
    path: /app/*/foo  
    delegate:
      name: glob
      namespace: prefix
```

##### Delegate IngressRoute:

```yaml
apiVersion: projectcontour.io/v1
kind: IngressRoute
metadata: 
  name: example
spec:
  routes:
  - match:
    path: /app/*/foo  
    services:
    - name: glob-service
      port: 80
```

### Virtual Host:

Contour today only supports exact domain name matching when defining the `fqdn` in an IngressRoute. However, supporting additional types is possible since the virtual host level doesn't require the same level of detail in regards to delegation. 

Contour wil support the same [domain search order](https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/route/route.proto#route-virtualhost) as Envoy. Given this order, IngressRoutes that are defined will follow this logic. Contour should also review this and mark the status of IngressRoutes accordingly:

1. Exact domain names: `www.foo.com`.
2. Suffix domain wildcards: `*.foo.com` or `*-bar.foo.com`.
3. Prefix domain wildcards: `foo.*` or `foo-*`.
4. Special wildcard `*` matching any domain.

**Note:** The wildcard will not match the empty string. e.g. `-bar.foo.com` will match `baz-bar.foo.com` but not `-bar.foo.com`. The longest wildcards match first. Only a single virtual host in the entire route configuration can match on `*`. A domain must be unique across all virtual hosts or the config will fail to load.

#### Delegation Security

Since delegation doesn't rely on the virtual host directly, Contour will add some options to disable specific types of vhost matching. For example, an Administrator might want to only enable "exact" domain names and disable the other types. 

## Security Considerations

- If the delegation is not properly implemented, there could be ways that users get access to portions of the host requests which they should not have access. This will need to be mitigated with proper testing and validation of the design implementation.
