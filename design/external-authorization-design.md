# External Authorization Support

Status: Approved

## Abstract
This document describes a design for performing request authorization for virtual hosts hosted by Contour.

## Background

- [#432][5] Provide support for integrating with an external auth service.
- [#2459][3] External auth background and goals.
- [#2325][9] Failures in External services configured in envoy should be visible to the application developer.

## Goals
- Allow operators to integrate existing authorization services with Contour.
- Allow arbitrary types of authentication and authorization to be supported.
- Decouple Contour from external authorization, so it can evolve at an independent rate.
- Integrate cleanly with the HTTPProxy API.

## Non Goals

- Support for multiple external authorization mechanisms (i.e. the protocol the proxy uses to communicate with the authorization server).
  Various proxies have different mechanisms for integrating external services, but it is impractical to support them all.
  This means that an authorization servers written to integrate with NGINX or Traefik will not work without modification.
- Abstracting the Envoy authorization mechanism.
  The scope of abstracting the Envoy authorization mechanism is too large to be tractable.
  Abstracting the protocol would also work against the goal of being able to integrate existing authorization servers.
- Implement specific authorization protocols.
  This design does not address implementing specific methods of authorizing client requests (e.g. OAuth2, SAML, LDAP, NTLM).
  These specific authorization protocols can be implemented in external authorization servers (see discussion below).
- Support for Ingress. Supporting Ingress resources is out of scope an not addressed in this design.

## High-Level Design
A new `ExtensionService` CRD adds a way to represent and track an authorization service.
This CRD is relatively generic, so that it can be reused for Envoy rate limiting and logging services.
The core of the `ExtensionService` CRD is subset of the `projectcontour.v1.HTTPProxy` `Service` specification.
Reusing the `Service` type allows the operator to specify configuration in familiar and consistent terms, especially TLS configuration.

Note that only the Envoy [GRPC authorization protocol][2] will be supported.
The GRPC protocol is a superset of the HTTP protocol and requires less configuration.
The drawback of only supporting the GRPC protocol is that many existing Envoy authorization servers only support the HTTP protocol.
Until Contour adds support for the v3 Envoy API, only the version 2 of the GRPC authorization protocol can be configured.

Operators can bind an authorization service to a `HTTPProxy` using a new field in the `VirtualHost` struct.
This field configures which `ExtensionService` resource to use, and allows the operator to set initial authorization policy.
Authorization policy can also be set on a `Route`, so that application owners can pass metadata and disable authorization on specific routes.

## Detailed Design

### ExtensionService Changes

The `ExtensionService` CRD has API version `projectcontour.io/v1alpha1`.

There are a number of benefits to creating a CRD to represent a supporting service:

- The CRD directly generates an Envoy [Cluster][4] that Contour can use for any purpose.
  This means that with a single new API, Contour can add support for authorization, rate limiting and logging support services.
- A CRD gives Contour a way to communicate the operational status of the support service, which was a [desired][9] [goal][7].
- A CRD allows the team operating the support service and the team operating Contour to collaborate more loosely.

```Go
// SupportProtocolVersion is the version of the GRPC protocol used
// to request support services.
type SupportProtocolVersion string

// SupportProtocolVersion2 requests the "v2" support protocol version.
const SupportProtocolVersion2 SupportProtocolVersion = "v2"

type ExtensionService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtensionServiceSpec   `json:"spec,omitempty"`
	Status ExtensionServiceStatus `json:"status,omitempty"`
}

// ExtensionServiceStatus should follow the pattern being established in [#2642][10].
// This field will updated by Contour.
type ExtensionServiceStatus struct {
	Conditions []Condition `json:"conditions"`
}

type ExtensionServiceSpec struct {
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	Services []contourv1.Service `json:"services"`

	// The load balancing policy for sending requests to the service.
	// +optional
	LoadBalancerPolicy *contourv1.LoadBalancerPolicy `json:"loadBalancerPolicy,omitempty"`

	// +optional
	TimeoutPolicy *contourv1.TimeoutPolicy `json:"timeoutPolicy,omitempty"`

	// This field sets the version of the GRPC protocol that Envoy uses to
	// send requests to the support service. Since Contour always uses the
	// v2 Envoy API, this is currently fixed at "v2". However, other
	// protocol options will be available in future.
	//
	// +kubebuilder:validation:Enum=v2
	ProtocolVersion SupportProtocolVersion `json:"protocolVersion,omitempty"`
}
```

The `Conditions` field in `ExtensionServicesStatus` follows the standardized Contour structure (proposed in #2642).
This type should be shared with other Contour API status types.

This document does not yet define how Contour monitors the support service.
However, the presence of the `Conditions` field allows Contour to expose the status of the support service fairly directly.
At minimum, Contour should observe the underlying Kubernetes Service and expose whether there are sufficient healthy Endpoints.
This information, analogous to Deployments observing and exposing Pod status, can be published as a `Ready` condition.

Each `ExtensionService` CRD generates a unique upstream Envoy [Cluster][4], which will emit standard Envoy metrics.
Note that the Envoy cluster name can be non-obvious, so exposing it in status may be helpful.

If the `Service` refers to a Kubernetes `ExternalName`, Contour should program Envoy to send the traffic to the external destination.

The `ExtensionService` CRD reuses the `Service` type from the `projectcontour.io/v1` API.
However, the setting following fields can generate a validation errors:

- `Protocol` may only be set to `h2` or `h2c` (the default should be `h2`).

Note that the `Service` type does not include a field for a namespace name.
This constrains `ExtensionService` resources to be in the same namespace as the `Service` they expose,
ensuring that whoever creates the `ExtensionService` also has authority over the `Service` backing it.

### HTTPProxy Changes

```Go
// ExtensionReference names a Contour extension resource (ExtensionService
// by default).
type ExtensionExtensionReference struct {
	// API version of the referent.
        // If this field has no value, Contour will use a default of "projectcontour.io/v1alpha1".
	// +optional
	APIVersion string `json:"apiVersion,omitempty" protobuf:"bytes,5,opt,name=apiVersion"`
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
        // If this field has no value, Contour will use a default of "ExtensionService".
	// +optional
	Kind string `json:"kind,omitempty" protobuf:"bytes,1,opt,name=kind"`
	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
        // If this field has no value, Contour will use the namespace of the enclosing HTTPProxy resource.
	// +optional
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,2,opt,name=namespace"`
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +required
	Name string `json:"name,omitempty" protobuf:"bytes,3,opt,name=name"`
}

type AuthorizationPolicy struct {
	// When true, this field disables client request authentication
	// for the scope of the policy.
	//
	// +optional
	Disabled bool `json:"disabled,omitempty"`

	// Context is a set of key/value literal strings that are sent to the
	// authentication server in the check request. If a context
	// is provided at an enclosing scope, the entries are merged
	// such that the inner scope overrides matching keys from the
	// outer scope.
	//
	// +optional
	Context map[string]string `json:"context,omitempty"`
}

type AuthorizationExtension struct {
        // ExtensionRef names the authorization service.
	// +required
	ExtensionRef ExtensionReference `json:"extensionRef"`

        // AuthPolicy is the default authorization policy applied to client requests.
	// +optional
	AuthPolicy *AuthorizationPolicy `json:"authPolicy,omitempty"`

        // If FailOpen is true, the client request is forwarded to the upstream service
        // even if the authorization server fails to respond. This field should not be
        // set in most cases. It is intended for use only while migrating applications
        // from internal authorization to Contour external authorization.
        //
        // +optional
        FailOpen bool `json:"failOpen,omitempty"`
}

// VirtualHost appears at most once. If it is present, the object is considered
// to be a "root".
type VirtualHost struct {
    ...

	// This field configures an extension service to perform
	// authorization for this virtual host. Authorization can
	// only be configured on virtual hosts that have TLS enabled.
	// If the TLS configuration requires client certificate
	///validation, the client certificate is always included in the
	// authentication check request.
	//
	// +optional
	Authorization *AuthorizationExtension `json:"authorization,omitempty"`
}

// Route contains the set of routes for a virtual host.
type Route struct {
    ...

        // AuthPolicy updates the authorization policy for client
	// requests that match this route.
	// +optional
	AuthPolicy *AuthorizationPolicy `json:"authPolicy,omitempty"`
}
```

An operator configures authorization on a root `HTTPProxy` by setting the `AuthorizationExtension` field.
Setting this field without also setting the `TLS` field is an error.

The `AuthorizationExtension` field carries and optional default authorization policy.
This policy sets the default policy for each subsequent `Route`.
If `authPolicy.Disabled` is true, included `Routes` will not require authorization unless they configure their own authorization policy with `Disabled` set to false.
The `authPolicy.Context` entries from the root `HTTPProxy` are merged with the context entries on each `Route`.
Where the two policies have context keys in common, the key from the root `HTTPProxy` is overwritten.
The merged `Context` entries are send to the authorization service as part of the check request.

Contour should require authorization to be disabled on all routes that set `PermitInsecure` to true (see [Compatibility](#compatibility)).

Contour sets the following Envoy external authorization configurations that operators cannot configure:

- TLS client certificates are always sent if they are present
- The client request body is never sent

The `FailOpen` field in the `AuthorizationExtension` struct is present to make it easier to migrate applications.
Currently, applications must perform authorization internally.
Operators make experience bugs or availability issues migrating to external authorization, so the `FailOpen` field
provides a way to gracefully migrate applications while the external authorization service may still be unreliable.

See the [Compatibility](#compatibility) section for information on how authorization interacts with other `HTTPProxy` features.
To summarize:
1. Authorization is only available on HTTPS
1. Routes that are marked `PermitInsecure` must also disable authorization
1. Authorization is not compatible with using the fallback TLS certificate

### Authorization Flows

Authorizing requests with an external server involves a number of
collaborating parties, so it can be helpful so understand how requests
flow through the various services.

- **Client:** A HTTP client.
- **Proxy:** An Envoy proxy server.
- **ExtAuth:** A external authorization server that implements the Envoy GRPC protocol.
- **AuthProvider:** A server that can authenticate requests.
  This could be an OIDC server, a server that can do LDAP authentication, or HTTP Basic auth.
  It could be implemented in the same process as the ExtAuth service.
- **Origin:** A HTTP origin server that responds to authenticated requests.

#### Verifying an authorized request

This flow describes what happens if the client already has already authenticated and obtained some sort of authorization token.

```
+---------+                  +-------+                    +---------+             +---------------+ +---------+
| Client  |                  | Proxy |                    | ExtAuth |             | AuthProvider  | | Origin  |
+---------+                  +-------+                    +---------+             +---------------+ +---------+
     |                           |                             |                          |              |
     | Send HTTP request         |                             |                          |              |
     |-------------------------->|                             |                          |              |
     |                           |                             |                          |              |
     |                           | Authorize HTTP request      |                          |              |
     |                           |---------------------------->|                          |              |
     |                           |                             | -----------------\       |              |
     |                           |                             |-| Verify request |       |              |
     |                           |                             | |----------------|       |              |
     |                           |                             |                          |              |
     |                           |                      200 OK |                          |              |
     |                           |<----------------------------|                          |              |
     |         ----------------\ |                             |                          |              |
     |         | Inject        |-|                             |                          |              |
     |         | authorization | |                             |                          |              |
     |         | metadata      | |                             |                          |              |
     |         |---------------| | Forward HTTP request        |                          |              |
     |                           |---------------------------------------------------------------------->|
     |                           |                             |                          |              |
     |                           |                             |                 Respond to HTTP request |
     |                           |<----------------------------------------------------------------------|
     |                           |                             |                          |              |
     |     Forward HTTP response |                             |                          |              |
     |<--------------------------|                             |                          |              |
     |                           |                             |                          |              |
```

#### Signing in to a service

This flow describes what happens if the client doesn't already have any authentication information and needs to sign in to some authentication service.

Note that in the sign in flow, the ExtAuth server may need to generate a 302 redirection to send the client back to the authentication provider to obtain the proper authorization tokens.
Once that happens, the client has to resend the original request and it will enter the verification flow above.

```
+---------+                   +-------+                    +---------+      +---------------+ +---------+
| Client  |                   | Proxy |                    | ExtAuth |      | AuthProvider  | | Origin  |
+---------+                   +-------+                    +---------+      +---------------+ +---------+
     |                            |                             |                   |              |
     | Send HTTP request          |                             |                   |              |
     |--------------------------->|                             |                   |              |
     |                            |                             |                   |              |
     |                            | Authorize HTTP request      |                   |              |
     |                            |---------------------------->|                   |              |
     |                            |                             |                   |              |
     |                            |                             | Authorize         |              |
     |                            |                             |------------------>|              |
     |                            |                             |                   |              |
     |                            |                             |         Challenge |              |
     |                            |                             |<------------------|              |
     |                            |                             |                   |              |
     |                            |                302 Redirect |                   |              |
     |                            |<----------------------------|                   |              |
     |                            | ----------------\           |                   |              |
     |                            |-| Generate      |           |                   |              |
     |                            | | authorization |           |                   |              |
     |                            | | redirection   |           |                   |              |
     |     Authorization redirect | |---------------|           |                   |              |
     |<---------------------------|                             |                   |              |
     |                            |                             |                   |              |
     | Sign in                    |                             |                   |              |
     |----------------------------------------------------------------------------->|              |
     |                            |                             |                   |              |
     |                            |                          Authorization response |              |
     |<-----------------------------------------------------------------------------|              |
     |                            |                             |                   |              |
     | Resend HTTP request        |                             |                   |              |
     |--------------------------->|                             |                   |              |
     |                            |                             |                   |              |
```

## Alternatives Considered

1. Contour could install itself as the authorization server.
   This could remove some of the limitations of the Envoy configuration structure at the cost of more complexity in Contour.
1. Integrate external authorization directly into `HTTPProxy`.
   This increases the complexity of the `HTTPProxy` structure and makes it difficult to reuse the same authorization service acrtoss multiple proxies.
   A separate CRD gives better opportunities to expose useful operational status.
   Integrating specific authorization parameters into `HTTPProxy` prevents operators implementing their own authorization flows.

## Security Considerations

- Most HTTP authorization protocols depend on HTTPS to keep headers and URL query parameters private.
  Supporting only authorization on HTTPS, Contour reduces the changes of misconfiguring this.
- HTTPS sessions between Envoy and the authorization server are required.
  TLS validation can be configured if necessary (should be recommended),
- Authorization services can run in separate Kubernetes namespaces with limited privilege.

## Compatibility

Contour can currently support v2 of the Envoy GRPC authorization protocol.
When Contour moves to v3 of the Envoy API, it can support both v2 and v3 authorization protocols.

The `ExtensionService` CRD supports GRPC authorization support services, but not HTTP ones.

### HTTPS Only

As discussed in [#2459][3], the Envoy external authorization HTTP filter is configured on the HTTP Connection Manager.
This means that the scope of how Contour can configure authorization depends on the structure of the HTTP configuration.
For HTTP, there is only one HTTP Connection Manager that contains all the virtual hosts.
Configuring a single authorization service for all the virtual hosts might be acceptable for some organizations, but it doesn't really fix the Contour multi-tenancy model.
On the other hand, Contour configures HTTPS with a separate HTTP Connection Manager for each virtual host, so different authorization servers can naturally be attached to each HTTPS virtual host.

The result of this is that authorization servers are not supported on HTTP.
While it is convenient from an implementation perspective, this policy is also consistent with Contour's security-first posture.
Note that using the TLS fallback certificate (for non-SNI clients) has the same HTTP Connection Manager properties as HTTP, so authentication servers also cannot be configured on virtual hosts that share the fallback certificate.

### PermitInsecure

If a `Route` has the `PermitInsecure` field set to true, Contour will program it into both the secure and insecure Envoy listeners.
This means that the same `Route` will be authenticated differently depending on how the client accesses it.
In this case, it may be impossible for the upstream service to reliably detect whether the request is authorized.
To resolve this, Contour should require authorization to be disabled on all routes that set `PermitInsecure` to true.

### Fallback Certificates

The fallback certificate feature is used to support TLS access by clients that do not support the SNI extension.
This is implemented by a default TLS inspection matcher that catches session requests that do not match an existing SNI name.
This matcher contains a HTTP Connection Manager that routes requests to all the virtual hosts that request fallback certificate support.
Because the Envoy `ext_authz` filter applies to a whole HTTP Connection Manager, it is incompatible with this structure.
There is no way to express that different virtual hosts can have different `ext_authz` configuration while using a single HTTP Connection Manager.

## Implementation

The Contour implementation provides a framework, but isn't useful until authorization services are available for the protocols that Contour operators need.

The Contour implementation is quite large, but can be separated into smaller stages:

- Support service CRD implementation
- Support service monitoring and status
- `HTTPProxy` integration
- Integration tests
- Documentation

Since the scope of authorization is quite large, documentation should be written as a new top level section.
If the documentation is included in the main `httpproxy.md` page, it will be difficult to organize and consume.

### Contour Authserver

The Contour project should host a simple server that can be used for testing and for simple authorization use cases.

The `contour-authserver` server implements two authentication backends.
First, a no-op test server that authorizes every request.
Second, a [HTTP basic authentication][1] server that authenticates request using htpasswd files stored as Kubernetes secrets.

### OIDC Authentication

Most enterprise users will want some combination of OAUTH2, SAML or OIDC.
Writing an Envoy authorization proxy for these protocols is considerably more complex than for HTTP basic authentication, and will need to be staffed if they are to be supported.
There are existing implementations of all these protocols, though to my knowledge none are exposed through the Envoy GRPC.
It should be possible to write an authorization proxy that integrates with specific implementations (e.g. [Dex](https://github.com/dexidp/dex)).

One open issue for OIDC is whether Contour ought to program the Envoy [JWT][6] filter.
Configuring this filter would substantially increase the required `HTTPProxy` API, and it's likely that the authorization server would still need to be invoked.
This issue should be revisited after gaining some implementation experience.

## Sample Configuration

First, add a cert-manager issuer so that we can easily self-sign TLS
certificates.

```yaml
apiVersion: cert-manager.io/v1alpha3
kind: ClusterIssuer
metadata:
  name: selfsigned
spec:
  selfSigned: {}
```

Next, deploy the service that needs to be authorized, and request a TLS
certificate for it.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ingress-conformance-echo

---

apiVersion: v1
kind: Service
metadata:
  name: ingress-conformance-echo

---

apiVersion: cert-manager.io/v1alpha2
kind: Certificate
metadata:
  name: echo
spec:
  dnsNames:
  - echo.projectcontour.io
  secretName: echo
  issuerRef:
    name: selfsigned
    kind: ClusterIssuer

```

Next, deploy the authorization service.
This is a test service that will just authorize all requests.
The authorization service, it's TLS secret and service are all placed into the `contour-auth` namespace.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: contour-auth

---

apiVersion: cert-manager.io/v1alpha2
kind: Certificate
metadata:
  name: testserver
  namespace: contour-auth
spec:
  dnsNames:
  - testserver.auth.projectcontour.io
  secretName: testserver
  issuerRef:
    name: selfsigned
    kind: ClusterIssuer

---

apiVersion: apps/v1
kind: Deployment
metadata:
  name: testserver
  namespace: contour-auth
  labels:
    app.kubernetes.io/name: contour-testserver
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: contour-testserver
  replicas: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: contour-testserver
    spec:
      containers:
      - name: testserver
        image: contour-authserver:latest
        imagePullPolicy: IfNotPresent
        command:
        - /contour-authserver
        args:
        - testserver
        - --address=:9443
        - --tls-ca-path=/tls/ca.crt
        - --tls-cert-path=/tls/tls.crt
        - --tls-key-path=/tls/tls.key
        ports:
        - name: auth
          containerPort: 9443
          protocol: TCP
        volumeMounts:
        - name: tls
          mountPath: /tls
          readOnly: true
      terminationGracePeriodSeconds: 10
      volumes:
      - name: tls
        secret: testserver

---

apiVersion: v1
kind: Service
metadata:
  name: testserver
  namespace: contour-auth
  labels:
    app.kubernetes.io/name: contour-testserver
spec:
  ports:
  - name: auth
    protocol: TCP
    port: 9443
    targetPort: 9443
  selector:
    app.kubernetes.io/name: contour-testserver
  type: ClusterIP

```

Now we can create a `ExtensionService` to expose the test authentication service to
Contour.

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ExtensionService
metadata:
  name: testserver
  namespace: contour-auth
spec:
  services:
  - name: testserver
    port: 9443
    validation:
      subjectName: testserver.auth.projectcontour.io
      caSecret: testserver

```

Finally, expose the echo service with authorization enabled:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: echo.projectcontour.io
    tls:
      secretName: echo
    authorization:
      extensionRef:
        apiVersion: projectcontour.io/v1alpha1
        kind: ExtensionService
        name: testserver
        namespace: contour-auth
    timeout: 500ms
    authPolicy:
      disabled: false
      context:
        key1: value1
        key2: value2
  routes:
  - services:
    - name: echo
      port: 80
```

# Discussion Topics

Some of these topics are discussed in the main sections of the design.
They are extracted here to guide reviewers and to ensure consensus.

## Degrees of Authorization Abstraction

This design fully abstracts the configuration of the authorization service and the client authorization protocol.
It does not provide any in-band configuration for popular authorization protocols like OIDC and SAML,
nor does it give any guidance on how to configure those.

Keeping Contour oblivious to the type of authorization means that:

1. Operators can implement any kind of authorization protocol
2. The authorization systems can evolve at a different rate than Contour
3. Authorization servers can be hosted in the Contour project by separate maintainers

It also means that:

1. Configuration is necessarily more complex and involves more components
1. Some client authorization protocols may not be supported by Contour

***Decision:*** This design is a necessary building block.
The Contour project should build on it to support what users need in production.

## Which Client Authorization Protocols to Support

The Contour project should give operators practically useful authorization capabilities.
This means that the project should build and support one or more authorization servers that will be useful to a wide audience.

***Decision:*** The Contour project should ensure that there is a well-supported path for OIDC and OAuth2 [#2664][11].
Other authorization protocols can be addressed based on demand.

## Deployment Implications

Adding authorization servers as a separate component increases the number of deployment options.
Contour does not have a supported installer, and this would exacerbate that need.

## GRPC Dependency

This design proposes to only support the Envoy GRPC mechanism.
This is a superset of the HTTP mechanism and allows more kinds of authorization to be implemented.
The drawback is that many languages do not have good support for GRPC, so there is a smaller potential developer pool.
Adding support for the HTTP mechanism would increase configuration complexity, but also introduce asymmetric feature support that depends on the auth server mechanism.
There are few existing authorization servers that support the Envoy GRPC mechanism (most support HTTP).

***Decision:*** Maintainer consensus was that this trade-off is OK.
We can help contribute Envoy GRPC support to existing authorization servers.

## No Authorization Intermediaries

In contrast to [#1014](https://github.com/projectcontour/contour/pull/1014), Contour itself is not in the authorization path.
This results in some restrictions (as noted above), and prevents the implementation of very fine-grained authorization (e.g different
client authorization protocols per path).
However, this approach means that the Contour changes are relatively limited.
There is enough API that an external server can still remove any restrictions, but it will not feel like a native part of the Contour API.

***Decision:*** Maintainer consensus was that this trade-off is OK.

## Envoy JWT Filter

As discussed earlier, the Envoy [JWT][6] filter can be used in conjunction with an OIDC authorization service.
However, in general, an OIDC authorization server will need to implement JWT checking already, so the Envoy filter
is really more of an optimization.
Configuring the JWT filter adds more configuration complexity.
Adding a JWT validation stanza to the `AuthorizationExtension` struct would be possible, though arguably ugly.

## ExtensionServiceReference kind field

Contour doesn't need to support arbitrary types of support service resources.
The design here includes a `Kind` field because it is modeled after `TypedLocalObjectReference`.

***Decision:*** Keep the API version and Kind fields, but make them optional and default them internally.

## ExtensionService status

One of the original requirements for adding dependencies on other services was being able to expose useful operational status.
The `ExtensionService` status field is where this information would be exposed, but this design does not address what status should be exposed.

Candidates could be:
1. Status of the underlying `Service`
1. Envoy metrics from the underlying `Cluster`
1. Health check status (from Envoy or Contour)

***Decision:*** Follow the Status standards being established in [#2642][10].

## ExtensionService health checks

Since health checks are a property of the `Route`, there's no way to express a `ExtensionService` health check.
Perhaps a `HTTPHealthCheckPolicy` field should be added.
Since the design assumes an extension service is GRPC, an implicit [`GrpcHealthCheck`][8] could be added.

***Decision:*** Not for this version. File and issue and address later.

[1]: https://tools.ietf.org/html/rfc7617
[2]: https://www.envoyproxy.io/docs/envoy/latest/api-v2/service/auth/v2/external_auth.proto
[3]: https://github.com/projectcontour/contour/issues/2459
[4]: https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/cluster.proto
[5]: https://github.com/projectcontour/contour/issues/432
[6]: https://www.envoyproxy.io/docs/envoy/latest/api-v2/config/filter/http/jwt_authn/v2alpha/config.proto
[7]: https://github.com/projectcontour/contour/issues/432#issuecomment-587963331
[8]: https://www.envoyproxy.io/docs/envoy/latest/api-v2/api/v2/core/health_check.proto#core-healthcheck-grpchealthcheck
[9]: https://github.com/projectcontour/contour/issues/2325
[10]: https://github.com/projectcontour/contour/pull/2642
[11]: https://github.com/projectcontour/contour/issues/2664
