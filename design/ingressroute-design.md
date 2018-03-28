# Executive Summary

This document describes the design of a new CRD designed to replace the v1beta1.Ingress Kubernetes object.
This new CRD will be integrated into Contour 0.5/0.6.
Contour will continue to support the current v1beta1.Ingress object for as long as it is supported in upstream Kubernetes.

# Goals

- Support multi tennant clusters with the ability to limit the management of routes to specific virtualhosts.
- Support the ability to delegate the configuration of some or all of the routes for a virtual host to another namespace
- Create a clear separation between "singleton" configure items--those that relate to the virtual host--and the "set" configuration items--routes on said virtual host.

# Non-goals

- Support ingress objects outside the current cluster
- IP in IP or GRE tunneling between custers with discontinious or overlapping IP ranges.
- Support non HTTP/HTTPS traffic ingress (ie, no UDP, no MongoDB, no TCP passthrough). We just do HTTP/HTTPS ingress.
- Deprecate Contours support for v1beta1.Ingress.

# Background

The ingress object was added to kubernetes in 1.2 as a way of describing properties of a cluster wide reverse HTTP proxy.
Since that time the ingress object has not progressed beyond the beta stage, and its stagnation inspired a cambrian explosion of annotations to express missing properties of http routing.

# High-Level Design

At a high level, this document proposes modeling ingress configuation as a graph of documents throughout a Kubernetes API server which when taken together form a directed acyclic graph (DAG) of the configuration for virtual hosts and their constituent routes.

## Delegation

The working model for delegation is DNS.
As the owner of a DNS domain, for example `.com`, I _delegate_ to another nameserver the responsibility for handing the subdomain `heptio.com`.
Any nameserver can hold a record for `heptio.com`, but without the linkage from the parent `.com` TLD, its information is unreachable and non authorative.

Each _root_ of a DAG starts at a virtual host, which describes properties like the fully qualified name of the virtual host, any aliases (for example a www. prefix) of that vhost, TLS configuration, and possibly global access list details.
The edges of a graph do not contain virtual host information, they are only reachable from a root via a delegation.
This permits the _owner_ of an ingress root to both delegate the authority to publish a service on a portion of the route space inside a virtual host, and to itself further delegate authority to publish and delegate.

This also means that 

In practice the linkage, or delegation, from root to edge, is performed with a specific type of route action.
You can think of it that rather than routing traffic to a service, it is routed to another ingressroute object for further processing.


##

# Detailed Design

## IngressRoute CRD

This is an example of a fully populated root ingressroute.

```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: google
  namespace: prod
spec:
  # virtualhost appears at most once. If it is present, the object is considered
  # to be a "root".
  virtualhost:
    # the fully qualified domain name of the root of the ingress tree
    # all leaves of the DAG rooted at this object relate to the fqdn (and its aliases)
    fqdn: www.google.com
    # a set of aliases for the domain, these may be alternative fqdns which are considered
    # aliases of the primary fqdn
    aliases:
      - www.google.com.au
      - google.com
    # if present describes tls properties. The CNI names that will be matched on
    # are described in fqdn and aliases, the tls.secretName secret must contain a
    # matching certificate
    tls:
      # required, the name of a secret in the current namespace
      secretName: google-tls
      # other properties like cipher suites may be added later
  # routes contains the set of routes for this virtual host.
  # routes must _always_ be present and non empty.
  # routes can be present in any order, and will be matched from most to least
  # specific.
  routes:
  # each route entry starts with a prefix match
  # and one of service or delegate
  - match: /static
    # service defines the properties of the service in the current namespace
    # that will handle traffic matching the route
    service:
    - name: google-static
      port: 9000
  - match: /finance
    # delegate delegates the matching route to another IngressRoute object.
    # This delegates the responsibility for /finance to the IngressRoute matching the delegate parameters
    delegate:
      # the name of the IngressRoute object to delegate to
      name: google-finance
      # the namespace of the IngressRoute object, if blank the current namespace is assumed
      namespace: finance
  - match: /ads
    # more than one service name/port pair may be present allowing the services for a route to
    # be handled by more than one service.
    service:
    - name: ads-red
      port: 9090
    - name: ads-blue
      port: 9090
```

This is an example of the `google-finance` object which has been delegated responsibilty for paths starting with `/finance`.

```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: google-finance
  namespace: finance
spec:
  # note that this is an edge, so there is no virtualhost key
  # routes contains the set of routes for this virtual host.
  # routes must _always_ be present and non empty.
  # routes can be present in any order, and will be matched from most to least
  # specific, however as this is an edge, only prefixes that match the prefix
  # that delegated to this object.
  routes:
  # each route in this object must start with /finance as this was the prefix which
  # delegated to this object.
  - match: /finance
    service:
    - name: finance-app
      port: 9999
  - match: /finance/static
    service:
    - name: finance-static-content
      port: 8080
  - match: /finance/partners
    # delegate the /finance/partners prefix to the partners namespace
    delegate:
      name: finance-partners
      namespace: partners
```

## Delegation rules

The delegation rules applied are as follows

1. If an `IngressRoute` object contains a `spec.virtualhost` key it is considered a root.
2. If an `IngressRoute` object does not contain `spec.virtualhost` key is considered an edge.
3. An edge is reachable if a delegation to it exists in another `IngressRoute` object.
4. Edges which are not reachable are considered orphened. Orphened edges have no effect on the running configuration.

## Validation rules

The validation rules applied in this design are as follows
Some of these rules may be relaxed in future designs.

1. If a validation error is encountered, the entire object is rejected. Partial application of the valid portions of the configuration is _not_ attempted.
2. The prefix of the delegate route in the parent must be match the routes in the child.
  ```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: root
  namespace: root
spec:
  virtualhost:
    fqdn: heptio.com
    aliases:
      - www.heptio.com
  routes:
  - match: /static
    delegate:
      name: child
      namespace: child
---
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: child 
  namespace: child
spec:
  routes:
  # valid
  - match: /static/css
  # invalid
  - match: /css
```    

## Authorisation

It is important to highlight that both root and edge IngressRoute objects are of the same type.
This is a departure from other designs which treat the permission to create a VirtualHost type object and a Route type object as separate.
The DAG design treats the delegation from one IngressRoute to another as permission to create routes.

## Reporting status

The presence of semantically valid object is not proof that it will be used.
This is a break from the kubernetes model whereby an object which is valid will generally be acted on by controllers.

An `IngressRoute` edge may be present but not consulted if it is not part of a delegation chain from a root.
This models the DNS model above, you can add any zone file that you want to your local DNS server, but unless someone delegates to you, those records are ignored.

In the case there an IngressRoute is present, but has no active delegation, it is known as _orphened_.
We record this information on a top level `status` key for operators and tools.

An example of an ophaned IngresRoute object:

```yaml
status:
  delegationStatuses:
  - state:
     orphaned:
       lastParent:
         name: ...
         namespace: ...
```

Note: `delegationStatuses` is a list, because in the non orphaned case, an IngressRoute may be a part of several delegation chains.

And example of a correctly delegated IngressRoute with a single parent:
```yaml:
status:
  delegationStatuses:
  - state:
      connected:
        parent:
          name: ...
          namespace: ...
          prefix: ...
        connectedAt: ...
```

## IngressRoutes only dispatch to services in the same namespace

Why a matching route may list more than one service/port pair, those services are always within the same namespace.
This is to prevent unintentional exposure of services running in other namespaces.

To publish a service in another namespace on a domain that you control, you must delegate the permission to publish to that namespace.
For example, the `kube-system/heptio` IngressRoute holds information about `heptio.com` but delegates the provision of the service to the `heptio-wordpress/wordpress` IngressRoute

```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: heptio 
  namespace: kube-system
spec:
  virtualhost:
    fqdn: heptio.com
    aliases:
      - www.heptio.com
  routes:
  - match: /
    delegate:
      name: wordpress
      namespace: heptio-wordpress
---
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: wordpress 
  namespace: heptio-wordpress
spec:
  routes:
  - match: /
    service:
      name: wordpress-svc
      port: 8000
```

## TLS

TLS configuration, certificates and cipher suites, remains similar in form to the existing ingress object, however because the `spec.virtualhost.tls` is is only present in root objects.

# Alternatives Considered

This proposal presents itself as an alternative to the more traditional proposals that split ingress into two classes of objects; one describing the properties of the ingress virtual host, the other describing the properties of the routes attached to that virtual host.

The downside of these proposals is the minimum use case, the http://hello.world case, needs to two CRD objects per ingress, one stating the url of the virtual host, the other stating the name of the CRD that holds the url of the virtual host.

Restricting who can create this pair of CRD objects further complicates things and draws towards a design with a third and possibly fourth CRD to apply policy to their counterparts.

By "overloading", or as Joe likes to say, making the CRD polymorphic, creates a more complex mental model for complex deployments, but in return "scales down" to a single CRD containing both the vhost details and the route details (as there is no delegation in the hello world example).
This property makes the design proposed appealing from a usability standpoint as _most_ ingress usecases are simple--publish my web app on this url--so it feels right to favour a design that does not penalise the default, simple, use case.

- links to other proposals

# Future work

Future work outside the scope of this design includes:

- Limit the set of namespaces where root IngressRoutes are valid. Only those permitted to operate in those namespaces can therefore create virtual hosts and delegate the permission to operate on them to other namespaces. This would most likely be acomplished with a command line flag or ConfigMap.
- Delegation to matching labels, rather than names, may be added in the future. This is valid, as long as none of the matching IngressRoute objects are roots, because routes are a set, so can be merged from several objects _in the same namespace_.

# Security Concerns

As it relates to the CRD the security implications of this proposal, the model of delegating a part of a vhost's route space to a CRD in another namespace is implies a level of trust.
I, the owner of the virtual host, delegate to you the /foo prefix and everything under it, implies that you cannot operate on this vhost outside /foo, but it also means that whatever you do in /foo; host malware, leak k8s session keys, is something I'm trusting you not to do.

