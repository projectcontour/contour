# Global External Authorization

Status: Accepted

## Abstract

HTTP Support for External Authorization.

## Background
See [#4954](https://github.com/projectcontour/contour/issues/4954) for context.

## Goals
- Support Envoy's L7 External Authorization filter for HTTP Virtual Hosts.
- Extend the functionality of existing External Authorization.

## Non Goals
- Supporting more than one External Authorization Configuration for HTTP endpoints.

## High-Level Design

Contour will add HTTP support for Envoy's External Authorozation.

Currently, Contour supports External Authorization only over HTTPS. External Authorization allows for requests to be authenticated before they are forwarded upstream. However, External authorization over HTTPS is incompatible with setups that terminate TLS at the cloud provider's load balancer layer because the Envoy HCM relies on the SNI to choose the correct filter chain. 

A new type, `GlobalExtAuth`, will be defined as part of the `ContourConfiguration`.
A `GlobalExtAuth` config would define a global external authorization configuration for all hosts and routes. 

### Opting out from Global External Auth 
Global external auth would enable auth on all virtual hosts by default. 
However, individual HTTPProxy owners will have the option to toggle this setting. This can be done in one of the following ways. 

#### Disabling External Auth

##### Virtual Host level 
Disable global external auth on the virtual host. This setting would disable all routes on said virtual host. 

```yaml
kind: HTTPProxy
...
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    authorization:
      globalExtAuthDisabled: true
...
```

##### Route level
For finer grain toggles, global external auth can also be toggled on an individual route. This is similar to the existing per route authentication policy and we will reuse the same dials.

```yaml 
kind: HTTPProxy
metadata:
  name: echo
spec:
...
  routes:
    - authPolicy:
        context:
          route: apex
        disabled: false
      conditions:
        - prefix: /
      services:
        - name: ingress-conformance-echo
          port: 80
    - authPolicy:
        context:
          route: subpage
        disabled: true
      conditions:
        - prefix: /subpage
      services:
        - name: ingress-conformance-echo
          port: 80
```

#### HTTPS Override
If a HTTPS virtual host has external authorization enabled, that configuration will override the global external authorization.
This gives an individual service owner finer grain control of their services in case they have different requirements than the settings in the global external authorization. 

## Detailed Design

### Contour Configuration changes
An external authorization service can be configured in the Contour config file.
This External authorization configuration will be used for all HTTP routes. 

```yaml
globalExtAuth:
  extensionService: projectcontour-auth/htpasswd
  failOpen: false
  authPolicy:
    disabled: false
    context:
      header1: value1
      header2: value2
  responseTimeout: 1s
  withRequestBody:
    maxRequestBytes: 1024
```

```go
type Parameters struct {
  ...
  // GlobalExtAuth optionally holds properties of the global external auth configuration.
	GlobalExtAuth GlobalExtAuth `yaml:"globalExtAuth,omitempty"`
  ...
}

// GlobalExtAuth defines properties of a global external auth service.
type GlobalExtAuth struct {
	// ExtensionService identifies the extension service responsible for the authorization.
	// formatted as <namespace>/<name>.
	ExtensionService string `yaml:"extensionService,omitempty"`


	// AuthPolicy sets a default authorization policy for client requests.
	// This policy will be used unless overridden by individual routes.
	//
	// +optional
	AuthPolicy *GlobalAuthorizationPolicy `json:"globalAuthPolicy,omitempty"`

	// ResponseTimeout configures maximum time to wait for a check response from the authorization server.
	// Timeout durations are expressed in the Go [Duration format](https://godoc.org/time#ParseDuration).
	// Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".
	// The string "infinity" is also a valid input and specifies no timeout.
	//
	// +optional
	ResponseTimeout string `json:"responseTimeout,omitempty"`

	// If FailOpen is true, the client request is forwarded to the upstream service
	// even if the authorization server fails to respond. This field should not be
	// set in most cases. It is intended for use only while migrating applications
	// from internal authorization to Contour external authorization.
	//
	// +optional
	FailOpen bool `json:"failOpen,omitempty"`

	// WithRequestBody specifies configuration for sending the client request's body to authorization server.
	// +optional
	WithRequestBody *GlobalAuthorizationServerBufferSettings `json:"withRequestBody,omitempty"`
}

// GlobalAuthorizationServerBufferSettings enables ExtAuthz filter to buffer client request data and send it as part of authorization request
type GlobalAuthorizationServerBufferSettings struct {
	// MaxRequestBytes sets the maximum size of message body ExtAuthz filter will hold in-memory.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1024
	MaxRequestBytes uint32 `json:"maxRequestBytes,omitempty"`

	// If AllowPartialMessage is true, then Envoy will buffer the body until MaxRequestBytes are reached.
	// +optional
	AllowPartialMessage bool `json:"allowPartialMessage,omitempty"`

	// If PackAsBytes is true, the body sent to Authorization Server is in raw bytes.
	// +optional
	PackAsBytes bool `json:"packAsBytes,omitempty"`
}

// GlobalAuthorizationPolicy modifies how client requests are authenticated.
type GlobalAuthorizationPolicy struct {
	// When true, this field disables client request authentication
	// for the scope of the policy.
	//
	// +optional
	Disabled bool `json:"disabled,omitempty"`

	// Context is a set of key/value pairs that are sent to the
	// authentication server in the check request. If a context
	// is provided at an enclosing scope, the entries are merged
	// such that the inner scope overrides matching keys from the
	// outer scope.
	//
	// +optional
	Context map[string]string `json:"context,omitempty"`
}
```

### HTTPProxy Configuration changes

```go
type AuthorizationServer struct {
	...
  GlobalExternalAuthenticationDisabled bool `json:"globalExtAuthDisabled"`
  ...
}
```

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: opt-out-of-ext-auth
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    authorization:
      ...
      globalExtAuthDisabled: true
      ...
```

### Sample configurations

#### Global auth all HTTP virtual hosts.
```yaml
globalExtAuth:
  extensionService: projectcontour-auth/htpasswd
  failOpen: false
  authPolicy:
    disabled: false
    context:
      header1: value1
      header2: value2
  responseTimeout: 1s
  withRequestBody:
    maxRequestBytes: 1024
```
#### Global auth with one virtual host opted out.
** Assuming Global Auth is enabled. 
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    authorization:
      globalExtAuthDisabled: true
  routes:
    - conditions:
        - prefix: /
      services:
        - name: ingress-conformance-echo
          port: 80
      conditions:
        - prefix: /subpage
      services:
        - name: ingress-conformance-echo
          port: 80

```
#### Global auth with per route opted out.
** Assuming Global Auth is enabled. 
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
    - authPolicy:
        context:
          route: apex
        disabled: false
      conditions:
        - prefix: /
      services:
        - name: ingress-conformance-echo
          port: 80
    - authPolicy:
        context:
          route: subpage
        disabled: true
      conditions:
        - prefix: /subpage
      services:
        - name: ingress-conformance-echo
          port: 80
```
## Alternatives Considered

### Envoy Filter Matching API 
Having a separate external authorization configuration for each upstream is more desirable. We  can achieve this by leveraging Envoy's [Matching API](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/advanced/matching/matching_api)
Note: This feature is an Alpha release, and there are no guarantees that future changes will maintain backward compatibility. 

Below is an example of how this would work. 

Consider 2 HTTPProxy definitions defined below, both HTTP with external authorization filter enabled. 
##### HTTP Proxy 1 
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: local.projectcontour.io
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    authorization:
      extensionRef:
        name: htpasswd
        namespace: projectcontour-auth
      responseTimeout: 0.5s
  routes:
  - services:
    - name: local.projectcontour.io
      port: 80
```

##### HTTP Proxy 2
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: internal.projectcontour.io
spec:
  virtualhost:
    fqdn: internal.projectcontour.io
    tls:
      secretName: ingress-conformance-echo
    authorization:
      extensionRef:
        name: htpasswd
        namespace: projectcontour-auth
      responseTimeout: 1s
  routes:
  - services:
    - name: internal.projectcontour.io
      port: 80
```

With this proposal, contour will generate the envoy configuration snippet below. 
NOTE: this snippet only represents the relevant bits of the HTTP listener.
##### Envoy
```yaml
...
http_filters:
  - name: composite
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.common.matching.v3.ExtensionWithMatcher
      extension_config:
        name: composite
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.http.composite.v3.Composite
      xds_matcher:
        matcher_tree:
          input:
            name: envoy.matching.http.input
            typed_config:
              "@type": type.googleapis.com/envoy.type.matcher.v3.HttpRequestHeaderMatchInput
              header_name: host
          exact_match_map:
            map:
              "local.projectcontour.io":
                action:
                  name: composite-action
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.composite.v3.ExecuteFilterAction
                    typed_config:
                      name: envoy.filters.http.ext_authz
                      typed_config:
                        "@type": type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz
                        grpc_service:
                          envoy_grpc:
                            cluster_name: htpasswd                      
                          timeout: 0.5s
                        include_peer_certificate: true
                        failure_mode_allow: false
                        transport_api_version: V3
              "internal.projectcontour.io":
                action:
                  name: composite-action
                  typed_config:
                    "@type": type.googleapis.com/envoy.extensions.filters.http.composite.v3.ExecuteFilterAction
                    typed_config:
                      name: envoy.filters.http.ext_authz
                      typed_config:
                        "@type": type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz
                        grpc_service:
                          envoy_grpc:
                            cluster_name: htpasswd                      
                          timeout: 1s
                        include_peer_certificate: true
                        failure_mode_allow: false
                        transport_api_version: V3
...
```

## Compatibility
Global HTTP authorization will be an optional, opt-in feature for Contour users.
