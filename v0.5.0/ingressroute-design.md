# Executive Summary

**Status**: _Draft_

This document describes the design of a new CRD to replace the v1beta1.Ingress Kubernetes object.
This new CRD will be integrated into Contour 0.5/0.6.
Contour will continue to support the current v1beta1.Ingress object for as long as it is supported in upstream Kubernetes.

# Goals

- Support multi-tenant clusters, with the ability to limit the management of routes to specific virtua lhosts.
- Support delegating the configuration of some or all the routes for a virtual host to another Namespace
- Create a clear separation between singleton configuration items--items that apply to the virtual host--and sets of configuration items--that is, routes on the virtual host.

# Non-goals

- Support Ingress objects outside the current cluster
- IP in IP or GRE tunneling between clusters with discontinuous or overlapping IP ranges.
- Support non HTTP/HTTPS traffic ingress--that is, no UDP, no MongoDB, no TCP passthrough. HTTP/HTTPS ingress only.
- Deprecate Contour support for v1beta1.Ingress.

# Background

The Ingress object was added to Kubernetes in version 1.2 to describe properties of a cluster-wide reverse HTTP proxy.
Since that time, the Ingress object has not progressed beyond the beta stage, and its stagnation inspired an explosion of annotations to express missing properties of HTTP routing.

# High-level design

At a high level, this document proposes modeling ingress configuation as a graph of documents throughout a Kubernetes API server, which when taken together form a directed acyclic graph (DAG) of the configuration for virtual hosts and their constituent routes.

## Delegation

The working model for delegation is DNS.
As the owner of a DNS domain, for example `.com`, I _delegate_ to another nameserver the responsibility for handing the subdomain `heptio.com`.
Any nameserver can hold a record for `heptio.com`, but without the linkage from the parent `.com` TLD, its information is unreachable and non authoritative.

Each _root_ of a DAG starts at a virtual host, which describes properties such as the fully qualified name of the virtual host, any aliases of the vhost (for example, a `www.` prefix), TLS configuration, and possibly global access list details.
The vertices of a graph do not contain virtual host information. They are reachable from a root only by delegation.
This permits the _owner_ of an ingress root to both delegate the authority to publish a service on a portion of the route space inside a virtual host, and to further delegate authority to publish and delegate.

In practice the linkage, or delegation, from root to vertex, is performed with a specific type of route action.
You can think of it as routing traffic to another ingress route for further processing, instead of routing traffic directly to a service.

# Detailed Design

## IngressRoute CRD

This is an example of a fully populated root ingress route.

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

This is an example of the `google-finance` object which has been delegated responsibility for paths starting with `/finance`.

```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: google-finance
  namespace: finance
spec:
  # note that this is a vertex, so there is no virtualhost key
  # routes contains the set of routes for this virtual host.
  # routes must _always_ be present and non empty.
  # routes can be present in any order, and will be matched from most to least
  # specific, however as this is a vertex, only prefixes that match the prefix
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
2. If an `IngressRoute` object does not contain `spec.virtualhost` key is considered a vertex.
3. A vertex is reachable if a delegation to it exists in another `IngressRoute` object.
4. Vertices which are not reachable are considered orphened. Orphened vertices have no effect on the running configuration.

## Validation rules

The validation rules applied in this design are as follows.
Some of these rules may be relaxed in future designs.

1. If a validation error is encountered, the entire object is rejected. Partial application of the valid portions of the configuration is **not** attempted.
2. The prefix of the delegate route in the parent must match the routes in the child.
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

It is important to highlight that both root and vertex IngressRoute objects are of the same type.
This is a departure from other designs which treat the permission to create a VirtualHost type object and a Route type object as separate.
The DAG design treats the delegation from one IngressRoute to another as permission to create routes.

## Reporting status

The presence of a semantically valid object is not a guarantee that it will be used.
This is a break from the Kubernetes model, in which a valid object is generally be acted on by controllers.

An `IngressRoute` vertex may be present but not consulted if it is not part of a delegation chain from a root.
This models the DNS model above. You can add any zone file that you want to your local DNS server, but unless someone delegates to you, those records are ignored.

In the case where an IngressRoute is present, but has no active delegation, it is known as **orphaned**.
We record this information in a top level `status` key for operators and tools.

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

Note: `delegationStatuses` is a list, because in the non-orphaned case, an IngressRoute may be a part of several delegation chains.

An example of a correctly delegated IngressRoute with a single parent:
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

## IngressRoutes dispatch only to services in the same namespace

While a matching route may list more than one service/port pair, the services are always within the same namespace.
This is to prevent unintentional exposure of services running in other namespaces.

To publish a service in another namespace on a domain that you control, you must delegate the permission to publish to the namespace.
For example, the `kube-system/heptio` IngressRoute holds information about `heptio.com` but delegates the provision of the service to the `heptio-wordpress/wordpress` IngressRoute:

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
    # delegate everything to heptio-wordpress/wordpress
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

TLS configuration, certificates, and cipher suites, remain similar in form to the existing Ingress object. However, because the `spec.virtualhost.tls` is present only in root objects, there is no ambiguity as to which IngressRoute holds the canonical TLS information. (This cannot be said for the current Ingress object).
This also implies that the IngressRoute root and the TLS Secret must live in the same namespace.
However as mentioned above, the entire routespace (/ onwards) can be delegated to another namespace, which allows operators to define virtual hosts and their TLS configuration in one namespace, and delegate the operation of those virtual hosts to another namespace.

# Alternatives considered

This proposal is presented as an alternative to more traditional designs that split ingress into two classes of object: one describing the properties of the ingress virtual host, the other describing the properties of the routes attached to that virtual host.

The downside of these proposals is that the minimum use case, the http://hello.world case, needs two CRD objects per ingress: one stating the url of the virtual host, the other stating the name of the CRD that holds the url of the virtual host.

Restricting who can create this pair of CRD objects further complicates things and moves toward a design with a third and possibly fourth CRD to apply policy to their counterparts.

Overloading, or making the CRD polymorphic, creates a more complex mental model for complicated deployments, but in exchange scales down to a single CRD containing both the vhost details and the route details, because there is no delegation in the hello world example.
This property makes the proposed design appealing from a usability standpoint, as **most** ingress use cases are simple--publish my web app on this URL--so it feels right to favour a design that does not penalise the default, simple, use case.

- links to other proposals

# Future work

Future work outside the scope of this design includes:

- Limiting the set of namespaces where root IngressRoutes are valid. Only those permitted to operate in those namespaces can therefore create virtual hosts and delegate the permission to operate on them to other namespaces. This would most likely be acomplished with a command line flag or ConfigMap.
- Delegation to matching labels, rather than names. This may be added in the future. This is valid, as long as none of the matching IngressRoute objects are roots, because routes are a set, so can be merged from several objects _in the same namespace_.

# Security Concerns

As it relates to the CRD, the security implications of this proposal, the model of delegating a part of a vhost's route space to a CRD in another namespace implies a level of trust.
I, the owner of the virtual host, delegate to you the /foo prefix and everything under it, implies that you cannot operate on this vhost outside /foo, but it also means that whatever you do in /foo--host malware, leak k8s session keys--is something I'm trusting you not to do.

## No ability to prevent route delegation

In writing up this proposal an issue occured to me.
It happens when Contour is operating in the poorly specified enforcing mode, where Contour recognises root IngressRoutes only in a set of authorised namespaces.
(In fact it happens regardless of enforcement mode, but the argument is that if you do not turn on enforcement you are explicitly saying you trust you users, or you have another mechanism in place -- CI/CD -- to handle this).

Imaging this scenario:
```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  # this is www.google.com's root object
  name: google
  namespace: virtualhost-namespace
spec:
  virtualhost:
    fqdn: www.google.com
    tls:
      # required, the name of a secret in the current namespace
      secretName: google-tls
      # other properties like cipher suites may be added later
  routes:
  - match: /mail
    # delegate to the gmail service running in another namespace
    delegate:
      name: gmail
      namespace: gmail
    ...
```
And in the gmail namespace we have a IngressRoute vertex
```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: gmail 
  namespace: gmail
spec:
  routes:
  - match: /mail
    service:
      name: gmail-tomcat-svc
      port: 8000
```
That's all fine, the operators in the `gmail` namespace can't change the delegation in the `virtualhost-namespace` and the admins in the `virtualhost-namespace` feel confident that they control `www.google.com` and that gmail is being served over TLS.

However, at some time later ad money is getting tight and the operators take on a new customer, who is very cheap and doesn't want to pay for TLS.
```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: daves-cheap-webhosting
  namespace: virtualhost-namespace
spec:
  virtualhost:
    fqdn: dave.cheney.net
  routes:
  - match: /
    delegate:
      name: webapp
      namespace: customer-dave
```
Because the ingress operators don't want to have to handle tickets for dave's cheap webhosting, they delegate `/` to dave's namespace and tell him if he has a problem getting it working he can ask for help on the forum.

_However_, dave deploys this vertex ingressroute CRD.
```yaml
apiVersion: contour.heptio.com/v1alpha1
kind: IngressRoute
metadata:
  name: webapp 
  namespace: customer-dave
spec:
  routes:
  - match: /mail
    delegate:
      name: gmail
      namespace: gmail
```
And all of a sudden, `http://dave.cheney.net/mail` is serving up the full gmail application, sans TLS.

This is a large worked example, but the problem boils down to "vertices have no way to list who they are delegated to".
Without a way to deny or permit delegations, anyone who knows the name of the object (which I don't believe is a secret, and even if it was, is security via obscurity) can expose those routes on a virtualhost they own.

### Mitigations

The best way I've thought to mitigate this so far is the vertex ingress routes would have to list the name of either

- the name/namespace of the incoming delegation
- the virtualhost.fqdn at the root of a delegation chain.

This is unfortunate for two reasons:

- It's more boilerplate: a deletates to b, b has to list a as a cross check.
- It turns a DAG into a linked list (possibly not the right term), but it would be impossible for a web service to be a member of multiple roots, unless of course we made the list of incoming vertices be a list -- which would probably push the solution into using virtualhost.fqdn.

The last thing is that this boilerplate _should be required_ even when not in enforcing mode.
I'm not interested in proposing a design where the security interlock that prevents dave's cheap webhosting from publishing gmail without TLS is considered to be the customer's problem because they did not add an optional key. 
Said again, if this is the mitigation we choose to adopt, it has to be mandatory for all users, because we all know how effective optional security features are.