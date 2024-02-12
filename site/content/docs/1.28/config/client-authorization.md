# Client Authorization

Contour supports integrating external servers to authorize client requests.

Envoy implements external authorization in the [ext_authz][1] filter.
This filter intercepts client requests and holds them while it sends a check
request to an external server.
The filter uses the check result to either allow the request to proceed, or to
deny or redirect the request.

The diagram below shows the sequence of requests involved in the successful
authorization of a HTTP request:

<p align="center">
<img src="/img/uml/client-auth-sequence.png" alt="client authorization sequence diagram"/>
</p>

The [external authorization][7] guides demonstrates how to deploy HTTP basic
authentication using Contour and [contour-authserver](https://github.com/projectcontour/contour-authserver).

## Extension Services

The starting point for external authorization in Contour is the
[ExtensionService][2] API.
This API creates a cluster which Envoy can use to send requests to an external server.
In principle, the Envoy cluster can be used for any purpose, but in this
document we are concerned only with how to use it as an authorization service.

An authorization service is a gRPC service that implements the Envoy [CheckRequest][3] protocol.
Note that Contour requires the extension to implement the "v3" version of the protocol.
Contour is compatible with any authorization server that implements this protocol.

The primary field of interest in the `ExtensionService` CRD is the
`.spec.services` field.
This field lists the Kubernetes Services that will receive the check requests.
The `.spec.services[].name` field contains the name of the Service, which must
exist in the same namespace as the `ExtensionService` object.
The `ExtensionService` object must exist in the same namespace as the
Services they target to ensure that both objects are under the same
administrative control.

### Load Balancing for Extension Services

An `ExtensionService` can be configured to send traffic to multiple Kubernetes Services.
In this case, requests are divided proportionally across the Services according
to the weight in the `.spec.services[].weight` field.
The service weight can be used to flexibly shift traffic between Services for
reasons like implementing blue-green deployments.
The `.spec.loadBalancerPolicy` field configures how Envoy will load balance
requests to the endpoints within each Service.

### TLS Validation for Extension Services

Since authorizing a client request may involve passing sensitive credentials
from a HTTP request to the authorization service, the connection to the
authorization server should be as secure as possible.
Contour defaults the `.spec.protocol` field to "h2", which configures
Envoy to use HTTP/2 over TLS for the authorization service connection.

The [.spec.validation][4] field configures how Envoy should verify the TLS
identity of the authorization server.
This is a critical protection against accidentally sending credentials to an
imposter service and should be enabled for all production deployments.
The `.spec.validation` field should specify the expected server name
from the authorization server's TLS certificate, and the trusted CA bundle
that can be used to validate the TLS chain of trust.

## Authorizing Virtual Hosts

The [.spec.virtualhost.authorization][5] field in the Contour `HTTPProxy`
API connects a virtual host to an authorization server that is bound by an
`ExtensionService` object.
Each virtual host can use a different `ExtensionService`, but only one
`ExtensionService` can be used by a single virtual host.
Authorization servers can only be attached to `HTTPProxy` objects that have TLS
termination enabled.

### Migrating from Application Authorization

When applications perform their own authorization, migrating to centralized
authorization may need some planning.
The `.spec.virtualhost.authorization.failOpen` field controls how client
requests should be handled when the authorization server fails.
During a migration process, this can be set to `true`, so that if the
authorization server becomes unavailable, clients can gracefully fall back to
the existing application authorization mechanism.

### Scoping Authorization Policy Settings

It is common for services to contain some HTTP request paths that require
authorization and some that do not.
The HTTPProxy [authorization policy][6] allows authorization to be
disabled for both an entire virtual host and for specific routes.

The initial authorization policy is set on the HTTPProxy virtual host
in the `.spec.virtualhost.authorization.authPolicy` field. 
This configures whether authorization is enabled, and the default authorization policy context.
If authorization is disabled on the virtual host, it is also disabled by
default on all the routes for that virtual host that do not specify an authorization policy.
However, a route can configure its own authorization policy (in the
`.spec.routes[].authPolicy` field) that can configure whether authorization
is enabled, irrespective of the virtual host setting.

The authorization policy context is a way to configure a set of key/value
pairs that will be sent to the authorization server with each request check
request.
The keys and values that should be specified here depend on which authorization
server has been configured.
This facility is intended for configuring authorization-specific information, such as
the basic authentication realm, or OIDC parameters.

The initial context map can be set on the virtual host.
This sets the context keys that will be sent on every check request.
A route can overwrite the value for a context key by setting it in the
context field of authorization policy for the route.

[1]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter
[2]: api/#projectcontour.io/v1alpha1.ExtensionService
[3]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/auth/v3/external_auth.proto
[4]: api/#projectcontour.io/v1.UpstreamValidation
[5]: api/#projectcontour.io/v1.AuthorizationServer
[6]: api/#projectcontour.io/v1.AuthorizationPolicy
[7]: guides/external-authorization.md
