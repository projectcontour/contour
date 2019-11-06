# HTTPProxy status redesign

## Background

Given its complexity, Contour's HTTPProxy CRD currently has insufficient information about what its status surfaced to the user.

Currently, it only supports a `currentStatus` and `description`, which are particularly poor user experience around inclusion and conditions.

In particular, the following questions about HTTPProxy are particularly problematic at the moment:

- What happens when *some* parts of a HTTPProxy configuration are invalid? What if one include is invalid, and the other is not? How can the user be aware of what's happening?
- What happens if there are problems on inclusion, particularly condition conflicts?
- What happens if there is a problem with composable HTTPProxy use, that is, including the same HTTPProxy object from multiple parents.
- How do users understanding the full tree of includes used to build an Envoy config from multiple HTTPProxy objects?

## Design Goals

### Goals Overview

In rethinking Status, we need to ensure that the whatever we choose can fit all the use cases our HTTPProxy design allows for.

In particular, the features that strongly affect how we design status:

- Inclusion: We need to have a clear set of rules about what classes of errors show up at the *includer* level and what at the *includee* level.
  - We also don't put limits on how big this structure should be, so we need to ensure that the status block can provide useful information for large levels of complexity.
- Composability: Because HTTPProxies can be included under more than one root, and can also be included in multiple places under a single root, Status needs to be able to work in both those cases.

We also need to consider the interaction between service admins when one side changes something that breaks something for the other side.

In addition, we need to keep some rules in mind about making the status useful:

- the status must be able to indicate multiple errors, not just one.
- errors must be able to be directly attributed to the part of the config that produced them.
  - To help with this, the status should have a way to show errors with routes and includes separately.
  - Most portions of the status must be namespaced by at least the fqdn of the root, if not some rendering of the include path.
- the status must be able to indicate problems that will stop the HTTPProxy working as expected, but that are not syntactic errors with the HTTPProxy construct. For example, the referenced service is invalid.
- the status should have a way to see the object as rendered by contour (that is, with merged conditions). This allows users to inspect what output Contour produces for a given input without using debug inputs or inspecting Envoy.

It's also worth noting that `status` does not have to *only* be status.
The `status` field in a Kubernetes object is generally used for any *computed information* (as opposed to the declarative configuration in `spec`)

Lastly, it's important to note that, now that HTTPProxy is GA, we can't change the existing status fields very much, if at all.

### Goals

Overall:

- Provide a better user experience for understanding HTTPProxy usage.

Required:

- Detail expected personas for users
- Provide a mechanism for detailed information about invalid inclusions, including specifying what is added to each object, includer and included.
- Provide a mechanism for detailed information about invalid routes/services.
- Provide a mechanism for HTTPProxy administrators to see what conditions are added at each level of inclusion.
- Provide the information required for higher-level parsing of the status (to enable both kubectl and other UI inspection).
- An external tool must be able to build a representation of the HTTPProxy include DAG without speaking using only the Kubernetes API.
- Provide a set of examples that can be used both for understanding this proposal and as the basis for a test suite for status.

Optional:

- Add usable printer columns for kubectl use (Fixes #1567)

### Non-Goals

- Considering how this will be implemented at anthing but the most basic level.

This is a high-level design effort, intended to think about what the ideal case would be.
Let's understand the dream first, then work back from there.

## Users/Personas

### Proxy Administrator

This is the person or team who controls the TLD and any associated TLS information.
They are responsible for root HTTPProxy objects, and specifying conditions on the includes for the root HTTPProy objects.

### Service Administrator

This is the person or team who controls a HTTPProxy that is included into another.
Similarly to HTTPProxy inclusion, there may be an arbitrarily long chain of Service Administrators before an actual service is reached.

### Upstream tool user

This is the person or team who wants to be able to visualize the overall set of HTTPProxy objects, how they interact, and get a picture of what the eventual Envoy config will look like.
This user only wants to use the Kubernetes API to retrieve this information.

Note that Proxy admins and Service admins may also be Upstream tool users.

## Overall Design

The overall design I have in mind is to add some further fields to the status object in each HTTPProxy, and allow for more information about the in-between states, other than 'everything is okay' and 'nothing is okay'.

Each one of these fields is intended to include enough information to enable either troubleshooting of specific errors, to expose information for higher-level tooling, or both.

### Extra fields

There are three extra fields I believe we must add, and one I believe we should.
The last is only optional only because I think that we could approximate its functions using the other fields.
I think that the design is cleaner with it included, however.

For each place where we add a status, I think we should keep the current naming scheme:

- `currentStatus` is exactly what it says - the current status. This should have three values, `Valid`, `Invalid`, and `Warning` (or similar).
- `description` performs the same function as the global `description` does right now, just closer to the source of the problem.

So, a generic `status` stanza for each type of thing below would look like:

```yaml
status:
  currentStatus: <Valid|Invalid|Warning>
  description: <text>
```

#### `status.includes`

This field should show the status of each include in the `spec.includes` array.
In order to fully identify them, this means that this must include all the information in each `spec.includes` item, plus the general `status` stanza.

That is, this field should include status of the includes *in this HTTPPRoxy*.

See the examples below to see this in action.

#### `status.routes`

Similarly to `status.includes`, this should include the information from each `route` stanza in `spec.routes`, with an added `status` stanza.

This field should include status of the routes in this HTTPProxy.

#### `status.appliedConditions`

This is where this gets trickier.
This field is intended to show what conditions are applied when this HTTPProxy object is included from somewhere.

In order to do this, the following information must be present, somehow:

- full list of merged conditions. That is, all prefixes  must be fully merged, and the full set of headers must be visible.
- a unique identifier for the condition set. In practice, this will end up being related to the inclusion path to this HTTPProxy.

The first is relatively straightforward, a `conditions` array, with the merge rules having been run on it, will suffice.

The second is important for higher-order tool use as well as service admin use.

The exact implementation here will depend on if we do the optional field I include below.

In any case, there should be an `includePath` field in the `status.appliedConditions[n]` object that ensures uniqueness.

This could be rendered as a string, representing the include path, preferably including the FQDN of the root.

So, something that looked like this:

```yaml
status:
  appliedConditions:
    - conditions:
      - prefix: /foo/bar/baz
      includePath: "root:www.foobar.com,foo,bar"
```

Alternatively, if we are not including a more verbose representation of the computed DAG in each object, something like this would be required:

```yaml
status:
  appliedConditions:
    - conditions:
      - prefix: /foo/bar/baz
      includePath:
        name: root
        namespace: default
        fqdn: www.foobar.com
        includes:
          name: foo
          namespace: default
          conditions:
          - prefix: /foo
          includes:
            name: bar
            namespace: default
            conditions:
            - prefix: /bar
            includes:
              name: baz
              namespace: default
              conditions:
              - prefix: /baz
```

#### Optional: `status.proxyGraph`

The intent of this field is to store enough breadcrumbs to allow a user (or external tool) to be able to deduce the entire HTTPProxy graph (if they have RBAC access to see the objects in all referenced namespaces).

It should store an include path leading to this HTTPProxy, any configuration defined in this HTTPProxy, and references to included HTTPProxies.
I think that it also should inline the status in each of these, making it a one-stop parsing shop for upstream tools.

This means that, for each place this HTTPProxy is included, there will be an include path leading to this HTTPProxy, including merging conditions along the way, and then a rendering of all routes and includes in this HTTPProxy, with the *effective* conditions on each.

So, this field is a superset of the other three, which are convenience fields that make it easier and quicker to get to information about *this* proxy without having to do as much parsing.

In including this field, we are trading object size and storage volume, against ease of retrieval, both in terms of number of requests and ease of parsing. I did consider including the full child tree, but I think that would be a lot of information stored for very little gain.
I think that including this in its current state is a good trade-off.

If this field is not included, then the `appliedConditions` field will need to fulfill the same function by building in the breadcrumb trail.

### Design goal summary

This section is about how this design meets the stated goals, so let's review:

- Provide a mechanism for detailed information about invalid inclusions, including specifying what is added to each object, includer and included.

This is the purpose of `status.includes`.

- Provide a mechanism for detailed information about invalid routes/services.

This is the purpose of `status.routes`.

- Provide a mechanism for HTTPProxy administrators to see what conditions are added at each level of inclusion.

`status.appliedConditions`.

- Provide the information required for higher-level parsing of the status (to enable both kubectl and other UI inspection).

All four status fields.

- An external tool must be able to build a representation of the HTTPProxy include DAG without speaking using only the Kubernetes API.

`status.proxyGraph`

#### Risks and other notes

There is a risk of information leakage (for example, both header names and matched contents would be visible from outside this HTTPProxy's namespace), but I think these risks are managed:

- All headers and path information will be accessible from the eventual service anyway. So the service admin (and all the intermediate service admins) having access to this information is nothing they would not know anyway.
- All TLS details are protected by both the requirement that TLS can only be included in a root HTTPProxy, and the TLSCertificateDelegation construct. SO they can't be exposed.

The other downside of this approach is that there is a lot of information redundancy.
However, I think that given the complexity of what we are delivering, and the organizational complexity that users of this require to actually make the overhead worthwhile, that the benefits are worth the costs.

## Use cases

Okay, so let's go through what some examples of this would look like.
In these examples, I'll have a short explanation, then show each of the HTTPProxy objects involved, with the status object filled out for each.

I've also concentrated on examples that will produce interesting status results, rather than being exhaustive.

### Simple exposing of a service

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: basic
spec:
  virtualhost:
    fqdn: foo-basic.bar.com
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
status:
  currentStatus: valid
  description: valid HTTPProxy
  proxyGraph:
  - fqdn: foo-basic.bar.com
    routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
  routes:
  - fqdn: foo-basic.bar.com
    conditions:
    - prefix: /
    services:
      - name: s1
        port: 80
    status:
      currentStatus: Valid
      descriptions: Valid route
    includePath: "basic:foo-basic.bar.com"
  includes: {}
  appliedConditions: {}
```

In simple cases like this, the information duplication is very apparent.
In fact, for HTTPProxies with no inclusions, most of the spec will be included in the status.
I think this is an acceptable tradeoff for the cases with more complexity (which is, after all, what this whole thing is aimed at.)

### Basic Inclusion

This is a reasonably straightforward example of inclusion within the same namespace.
Most notable here is the fact that it is the rendered prefix that is visible in the status object.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: include-root
  namespace: default
spec:
  virtualhost:
    fqdn: root.bar.com
  includes:
  - name: www
    namespace: default
    conditions:
    - prefix: /service2
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
status:
  currentStatus: valid
  description: valid HTTPProxy
  proxyGraph:
  - fqdn: root.bar.com
    includes:
      name: www
      namespace: default
      conditions:
      - prefix: /service2
      status:
        condition: Valid
        description: Valid include
  includes:
    - name: www
      namespace: default
      conditions:
      - prefix: /service2
      status:
        currentStatus: Valid
        description: Valid include
  routes:
    - fqdn: root.bar.com
      conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
      status:
        currentStatus: Valid
        description: Valid route
      includePath: "include-root:root.bar.com"
  appliedConditions: {}
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: service2
  namespace: default
spec:
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s2
          port: 80
    - conditions:
      - prefix: /blog
      services:
        - name: blog
          port: 80
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes: {}
  routes:
    - fqdn: root.bar.com
      conditions:
      - prefix: /service2
      services:
        - name: s2
          port: 80
      status:
        currentStatus: Valid
        description: Valid route
      includePath: "include-root:root.bar.com,service2"
    - fqdn: root.bar.com
      conditions:
      - prefix: /service2/blog
      services:
        - name: blog
          port: 80
      status:
        currentStatus: Valid
        description: Valid route
      includePath: "include-root:root.bar.com,service2"
  proxyGraph:
  - fqdn: root.bar.com
    includes:
      name: www
      namespace: default
      conditions:
      - prefix: /service2
      status:
        condition: Valid
        description: Valid include
      routes:
        - conditions:
          - prefix: /service2
          services:
            - name: s2
              port: 80
          status:
            currentStatus: Valid
            description: Valid route
        - conditions:
          - prefix: /service2/blog
          services:
            - name: blog
              port: 80
          status:
            currentStatus: Valid
            description: Valid route
  appliedConditions:
  - fqdn: root.bar.com
    includes:
      - name: www
        namespace: default
        conditions:
        - prefix: /service2
```

### Single HTTPProxy included by two roots

This example illustrates the reason for namespacing the `proxyGraph` and `appliedConditions` fields.

```yaml
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-root
  namespace: default
spec:
  virtualhost:
    fqdn: bar.com
  includes:
  - name: main
    namespace: default
    conditions:
      - prefix: /
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes:
    - name: main
      namespace: default
      conditions:
      - prefix: /
      status:
        currentStatus: Valid
        description: Valid include
  routes: {}
  proxyGraph:
  - fqdn: bar.com
    includes:
    - name: main
      namespace: default
      conditions:
      - prefix: /
      status:
        condition: Valid
        description: Valid include
  appliedConditions: {}
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: multiple-root-www
  namespace: default
spec:
  virtualhost:
    fqdn: www.bar.com
  includes:
  - name: main
    namespace: default
    conditions:
      - prefix: /
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes:
  - name: main
    namespace: default
    conditions:
      - prefix: /
    status:
      currentStatus: Valid
      description: Valid include
  routes: {}
  proxyGraph:
  - fqdn: www.bar.com
    includes:
    - name: main
      namespace: default
      conditions:
      - prefix: /
      status:
        condition: Valid
        description: Valid include
  appliedConditions: {}

---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: main
  namespace: default
spec:
  routes:
  - services:
    - name: s2
      port: 80
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes: {}
  routes:
    - fqdn: bar.com
      conditions:
      - prefix: /
      services:
      - name: s2
        port: 80
      status:
        currentStatus: Valid
        description: Valid route
      includePath: ""multiple-root:bar.com,main""
    - fqdn: www.bar.com
      conditions:
      - prefix: /
      services:
      - name: s2
        port: 80
      includePath: "multiple-root-www:www.bar.com,main"
      status:
        currentStatus: Valid
        description: Valid route
  proxyGraph:
  - fqdn: bar.com
    includes:
    - name: main
      namespace: default
      conditions:
      - prefix: /
      status:
        currentStatus: Valid
        description: Valid include
      routes:
      - conditions:
        - prefix: /
        services:
        - name: s2
          port: 80
        status:
          currentStatus: Valid
          description: Valid route
  - fqdn: www.bar.com
    includes:
    - name: main
      namespace: default
      conditions:
      - prefix: /
      status:
        currentStatus: Valid
        description: Valid include
      routes:
      - conditions:
        - prefix: /
        services:
        - name: s2
          port: 80
        status:
          currentStatus: Valid
          description: Valid route
  appliedConditions:
  - fqdn: bar.com
    includes:
      - name: main
        namespace: default
        conditions:
        - prefix: /
  - fqdn: www.bar.com
    includes:
      - name: main
        namespace: default
        conditions:
        - prefix: /
```

### Complex includes

This example is complicated, to try and illustrate the value of the additional fields in `status`.

A single root HTTPProxy has two includes, with each of those having two includes.

Things to look for in this example:

- The included HTTPProxy object's `status.proxyGraph` field only shows the edge they are a part of. That is, siblings are not visible.
- Conditions are displayed in the merged state.
That is, prefixes are displayed in the merged state, not the configured state, and headers are displayed as the full union of all header conditions.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: root
  namespace: default
spec:
  virtualhost:
    fqdn: www.widgetsandsprockets.com
  includes:
  - name: api
    namespace: default
    conditions:
      - prefix: /api
  - name: defaultsite
    namespace: default
    conditions:
      - prefix: /
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes:
  - name: api
    namespace: default
    conditions:
      - prefix: /api
    status:
      currentStatus: Valid
      description: Valid include
  - name: defaultsite
    namespace: default
    conditions:
      - prefix: /
    status:
      currentStatus: Valid
      description: Valid include
  routes: {}
  proxyGraph:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: api
      namespace: default
      conditions:
      - prefix: /api
      status:
          condition: Valid
          description: Valid include
    - name: defaultsite
      namespace: default
      conditions:
      - prefix: /
      status:
        condition: Valid
        description: Valid include
  appliedConditions: {}
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: api
  namespace: default
spec:
  includes:
  - name: sprockets
    namespace: default
    conditions:
      - prefix: /sprockets
  - name: widgets
    namespace: default
    conditions:
      - prefix: /widgets
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes:
  - name: sprockets
    namespace: default
    conditions:
      - prefix: /api/sprockets
    status:
      currentStatus: Valid
      description: Valid include
  - name: widgets
    namespace: default
    conditions:
      - prefix: /api/widgets
    status:
      currentStatus: Valid
      description: Valid include
  proxyGraph:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: api
      namespace: default
      conditions:
      - prefix: /api
      includes:
      - name: sprockets
        namespace: default
        conditions:
        - prefix: /api/sprockets
        status:
          condition: Valid
          description: Valid include
      - name: widgets
        namespace: default
        conditions:
        - prefix: /api/widgets
        status:
          condition: Valid
          description: Valid include
  appliedConditions:
    - fqdn: www.widgetsandsprockets.com
      includes:
        - name: api
          namespace: default
          conditions:
          - prefix: /api
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: defaultsite
  namespace: default
spec:
  includes:
  - name: ios
    namespace: default
    conditions:
      - prefix: /
      - header:
          name: x-os
          contains: ios
  - name: defaultbackend
    namespace: default
    conditions:
      - prefix: /
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes:
  - name: ios
    namespace: default
    conditions:
      - prefix: /
      - header:
          name: x-os
          contains: ios
    status:
      currentStatus: Valid
      description: Valid include
  - name: defaultbackend
    namespace: default
    conditions:
      - prefix: /
    status:
      currentStatus: Valid
      description: Valid include
  proxyGraph:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: defaultsite
      namespace: default
      conditions:
      - prefix: /
      includes:
      - name: ios
        namespace: default
        conditions:
        - prefix: /
        - header:
            name: x-os
            contains: ios
        status:
          condition: Valid
          description: Valid include
    - name: defaultsite
      namespace: default
      conditions:
      - prefix: /
      includes:
      - name: defaultbackend
        namespace: default
        conditions:
        - prefix: /
        status:
          condition: Valid
          description: Valid include
  appliedConditions:
    - fqdn: www.widgetsandsprockets.com
      includes:
        - name: defaultsite
          namespace: default
          conditions:
          - prefix: /
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: sprockets
  namespace: default
spec:
  routes:
  - services:
    - name: sprockets
      port: 80
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes: {}
  routes:
    - fqdn: www.widgetsandsprockets.com
      conditions:
      - prefix: /api/sprockets
      services:
      - name: sprockets
        port: 80
      includePath: "root:www.widgetsandsprockets.com,api,sprockets"
      status:
        currentStatus: Valid
        description: Valid route
  proxyGraph:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: api
      namespace: default
      conditions:
      - prefix: /api
      status:
        currentStatus: Valid
        description: Valid include
      includes:
      - name: sprockets
        namespace: default
        conditions:
        - prefix: /api/sprockets
        status:
          currentStatus: Valid
          description: Valid include
        routes:
        - conditions:
          - prefix: /api/sprockets
          services:
          - name: widgets
            port: 80
          status:
            currentStatus: Valid
            description: Valid route
  appliedConditions:
    - fqdn: www.widgetsandsprockets.com
      includes:
      - name: api
        namespace: default
        conditions:
        - prefix: /api
        includes:
        - name: sprockets
          namespace: default
          conditions:
          - prefix: /api/sprockets
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: widgets
  namespace: default
spec:
  routes:
  - services:
    - name: widgets
      port: 80
  - conditions:
    - prefix: /order
    services:
    - name: widget-order
      port: 80
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes: {}
  routes:
   routes:
  - fqdn: www.widgetsandsprockets.com
    conditions:
    - prefix: /api/widgets
    services:
    - name: widgets
      port: 80
    includePath: "root:www.widgetsandsprockets.com,api,sprockets"
    status:
      currentStatus: Valid
      description: Valid route
  - fqdn: www.widgetsandsprockets.com
    conditions:
    - prefix: /api/widgets/order
    services:
    - name: widget-order
      port: 80
    includePath: "root:www.widgetsandsprockets.com,api,sprockets"
    status:
      currentStatus: Valid
      description: Valid route
proxyGraph:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: api
      namespace: default
      conditions:
      - prefix: /api
      status:
        currentStatus: Valid
        description: Valid include
      includes:
      - name: widgets
        namespace: default
        conditions:
        - prefix: /api/widgets
        status:
          currentStatus: Valid
          description: Valid include
        routes:
        - conditions:
          - prefix: /api/widgets/order
          services:
          - name: widget-order
            port: 80
          status:
            currentStatus: Valid
            description: Valid route
        - conditions:
          - prefix: /api/widgets
          services:
          - name: widgets
            port: 80
          status:
            currentStatus: Valid
            description: Valid route
  appliedConditions:
    - fqdn: www.widgetsandsprockets.com
      includes:
      - name: api
        namespace: default
        conditions:
        - prefix: /api
        includes:
        - name: sprockets
          namespace: default
          conditions:
          - prefix: /api/widgets
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: ios
  namespace: default
spec:
  routes:
  - services:
    - name: ios
      port: 80
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes: {}
  routes:
    - fqdn: www.widgetsandsprockets.com
      conditions:
      - prefix: /
      - header:
          name: x-os
          contains: ios
      services:
      - name: ios
        port: 80
      status:
        currentStatus: Valid
        description: Valid route  
proxyGraph:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: defaultsite
      namespace: default
      conditions:
      - prefix: /
      status:
        currentStatus: Valid
        description: Valid include
      includes:
      - name: ios
        namespace: default
        conditions:
        - prefix: /
        - header:
            name: x-os
            contains: ios
        status:
          currentStatus: Valid
          description: Valid include
        routes:
        - services:
          - name: ios
            port: 80
          conditions:
          - prefix: /
          - header:
              name: x-os
              contains: ios
          status:
            currentStatus: Valid
            description: Valid route
  appliedConditions:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: defaultsite
      namespace: default
      conditions:
      - prefix: /
      includes:
      - name: ios
        namespace: default
        conditions:
        - prefix: /
        - header:
            name: x-os
            contains: ios
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: defaultbackend
  namespace: default
spec:
  routes:
  - services:
    - name: defaultbackend
      port: 80
status:
  currentStatus: valid
  description: valid HTTPProxy
  includes: {}
  routes:
  - fqdn: www.widgetsandsprockets.com
    conditions:
    - prefix: /
    services:
    - name: defaultbackend
      port: 80
    includePath: "root:www.widgetsandsprockets.com,defaultsite,default"
  proxyGraph:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: defaultsite
      namespace: default
      conditions:
      - prefix: /
      status:
        currentStatus: Valid
        description: Valid include
      includes:
      - name: default
        namespace: default
        conditions:
        - prefix: /
        status:
          currentStatus: Valid
          description: Valid include
       routes:
        - services:
          - name: defaultbackend
            port: 80
          conditions:
          - prefix: /
          status:
            currentStatus: Valid
            description: Valid route
  appliedConditions:
  - fqdn: www.widgetsandsprockets.com
    includes:
    - name: defaultsite
      namespace: default
      conditions:
      - prefix: /
      includes:
      - name: default
        namespace: default
        conditions:
        - prefix: /
```

### Invalid: Root with no FQDN

In this case, the root HTTPProxy does not include a FQDN key, so the included HTTPProxy will be left orphaned.

I think that whether or not `proxyGraph` should be filled out in the case that the HTTPProxy is invalid at the root level is probably very dependent on exactly how this is implemented.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: invalidParent
  namespace: roots
spec:
  virtualhost:
  includes:
  - name: validChild
    namespace: roots
    conditions:
      - prefix: /foo
status:
  currentStatus: invalid
  description: Spec.VirtualHost.Fqdn must be specified
  proxyGraph: {}
  appliedConditions: {}
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: validChild
  namespace: roots
spec:
  routes:
  - services:
    - name: s2
      port: 80
status:
  currentStatus: orphaned
  description: this HTTPProxy is not part of a delegation chain from a root HTTPProxy
  proxyGraph: {}
  appliedConditions: {}

```

### One HTTPProxy included at multiple places

TODO

This one will show why we need `appliedConditions`, `routes`, and `includes` need to be namespaced.

### Invalid: Included HTTPProxy contains an invalid Service

TODO
These show how the invalid status will be communicated.

### Invalid: Two included HTTPProxies, one with invalid Service

This is to show how the new Status can handle the `Warning` overall state.
TODO

### Invalid: Two layers of includes with typo on the middle

This is to show that the lower layer will be orphaned.

TODO

### Invalid: Conflicting header conditions

TODO

This is to show how we handle a more complicated error condition.
