# JWT Verification Support

## Abstract
This document describes a design for performing JSON Web Token (JWT) verification for requests to virtual hosts hosted by Contour.

## Background
JSON Web Token (JWT) is an open standard (RFC 7519) that defines a compact and self-contained way for securely transmitting information between parties as a JSON object. 
This information can be verified and trusted because it is digitally signed. (ref. https://jwt.io/introduction)

Envoy Proxy has built-in support for verifying JWTs that are attached to incoming requests, via the [JWT Authentication HTTP filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/jwt_authn_filter#config-http-filters-jwt-authn). (Note, this document will use the term **JWT verification** to describe this process).
Specifically, Envoy can verify the signature, audience, issuer and time restrictions of a JWT.
If verification fails, the request will be rejected.
It's important to note that this filter does not itself obtain a JWT for an incoming request; the request must already have one attached.

Contour does not currently have support for configuring the JWT authentication filter.
This document proposes a design for adding that support to Contour's custom resource, `HTTPProxy`.

## Goals
- JWT verification for requests to TLS-enabled virtual hosts.
- Expose a subset of the Envoy filter configuration to cover the most common use cases.
- Be able to easily expose additional configuration settings if/when needed in the future.

## Non Goals
- JWT verification for requests to non-TLS enabled virtual hosts.
- Exposing all possible Envoy configuration settings.
- Supporting end-to-end OAuth2/OIDC flows.

## High-Level Design
Contour's `HTTPProxy` resource will get a new optional field, `spec.virtualhost.jwtProviders`, to define the details of how to verify JWTs for requests to a given virtual host.
This field will only be supported for virtual hosts for which Envoy is terminating TLS.
The structure of this field will be similar to the [Envoy filter's providers field](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/jwt_authn/v3/config.proto#envoy-v3-api-msg-extensions-filters-http-jwt-authn-v3-jwtauthentication), with some simplifications. 
Specifically, a provider will define an issuer, 0+ audiences, and a JSON Web Key Set (JWKS) that can be used to verify a JWT (see [the Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/jwt_authn/v3/config.proto#envoy-v3-api-msg-extensions-filters-http-jwt-authn-v3-jwtprovider) for more information).
Any number of providers can be defined for a virtual host, to allow different routes to be verified differently.
At most one provider can be marked as the "default", meaning it will automatically be applied as a requirement to all routes unless they explicitly opt out (more below).

`HTTPProxy` routes will also get a new optional field, `spec.routes.jwtVerificationPolicy`, to provide details on how to apply JWT verification.
The JWT verification policy will allow a specific named provider to be required for the route if there is no default or if a provider other than the default should apply for the route.
It will also allow explicitly opting out of using the proxy's default provider.

Contour will validate the contents of `spec.virtualhost.jwtProviders` and `spec.routes.jwtVerificationPolicy` if present, and will configure the JWT authentication filter on the HTTP Connection Manager for the relevant virtual host.
Contour will also add a CDS cluster for the remote JWKS, as required by [the Envoy configuration](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/jwt_authn/v3/config.proto#envoy-v3-api-msg-extensions-filters-http-jwt-authn-v3-remotejwks).

JWT verification will only be supported for TLS-enabled virtual hosts because (a) JWTs generally should not be transmitted in cleartext; and (b) all plain HTTP virtual hosts share a single HTTP Connection Manager & associated filter config, which creates challenges when configuring different JWT verification providers and rules for different virtual hosts.
This constraint *may* be revisited at a later date if a compelling use case is identified.

## Detailed Design
The detailed structure of the new fields is shown via YAML below:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tls-cert
    jwtProviders:
      - 
        # name is a unique name for the provider.
        name: provider-1
        # default is whether or not this provider should be applied
        # to routes by default. At most one provider can have this
        # flag set to true.
        default: true
        # issuer (optional) must match the "iss" field in the JWT.
        # If not specified, the "iss" field is not checked.
        issuer: foo.com
        # audiences (optional) allowlist for the "aud" field in the JWT.
        # If not specified, the "aud" field is not checked.
        audiences:
          - audience-1
          - audience-2
        # remoteJWKS is an HTTP endpoint that returns the JWKS
        # to use to verify the JWT signature. Exactly one of
        # remoteJWKS or localJWKS must be set.
        remoteJWKS:
          httpURI:
            uri: https://example.com/jwks.json
            timeout: 1s
            # Upstream TLS validation options. If not provided,
            # the TLS server cert will not be verified.
            validation:
              caSecret: ca-crt
              subjectName: example.com
          # cacheDuration is how long to cache the fetched JWKS
          # locally.
          cacheDuration: 5m
        # localJWKS can be used instead of remoteJWKS and defines
        # an in-cluster secret containing the JWKS for this provider.
        # Exactly one of localJWKS or remoteJWKS must be set.
        localJWKS:
          secretName: my-jwks
          key: jwks.json
  routes:
    # This route specifies jwtProvider: provider-1, which requires requests
    # to have a JWT has issuer=foo.com, audience of either "audience-1" or 
    # "audience-2", and signature must be able to be verified using the JWKS
    # at https://example.com/jwks.json).
    - conditions:
        - prefix: /
      jwtVerificationPolicy:
        # requires opts into requiring a particular provider if it
        # is not the default. In this case, it can be omitted since
        # provider-1 is the default; it's shown only for explanation.
        requires: provider-1
        # disabled allows disabling JWT verification for specific
        # routes. In this case, it can be omitted because it's false,
        # but it's shown for explanation. 
        disabled: false
      services:
        - name: s1
          port: 80
    # This route disables JWT verification (it would otherwise be applied
    # by default, requiring the default provider-1), so requests
    # with paths starting with /js are excluded from JWT verification.
    # Note that Contour orders routes such that longer prefixes take
    # priority over shorter prefixes, so requests starting with /js will
    # be handled by this route rather than the previous catch-all route.
    - conditions:
        - prefix: /js
      jwtVerificationPolicy:
        disabled: true
      services:
        - name: s1
          port: 80
```

It is worth highlighting some of the configuration options that Envoy's filter has, that Contour will *not* expose, at least not initially:
- non-default extract locations for the JWT (the default locations are (1) the `Authorization` header using the Bearer schema; and (2) the `access_token` query parameter, in that order).
- complex rule requirements (e.g. `requires_any`, `requires_all`)
- rule route matches are limited to what Contour already supports for routing (path prefix and some header matching)

Contour's API is structured such that these and other more complex/less common options may be added at a later date, if there is user demand for them.

A complete/valid HTTPProxy using JWT verification is shown below:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-proxy
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tls-cert
    jwtProviders:
      - name: provider-1
        default: true
        issuer: example.com
        audiences:
          - audience-1
          - audience-2
        remoteJWKS:
          httpURI:
            uri: https://example.com/jwks.json
            timeout: 1s
          cacheDuration: 5m
  routes:
    - conditions:
        - prefix: /
      services:
        - name: s1
          port: 80
    - conditions:
        - prefix: /css
      jwtVerificationPolicy:
        disabled: true
      services:
        - name: s1
          port: 80
```


## Alternatives Considered

### OAuth2 filter
Envoy also has an [OAuth2 HTTP filter](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/oauth2_filter), which supports end-to-end OAuth2/OIDC flows.
This provides related but separate functionality to the JWT authentication filter.
This [excellent blog post](https://www.jpmorgan.com/technology/technology-blog/protecting-web-applications-via-envoy-oauth2-filter) shows an example of how to use the OAuth2 and JWT filters together in Envoy.
Contour [may pursue adding support for the OAuth2 filter](https://github.com/projectcontour/contour/issues/2664), but it will be designed and implemented separately.


### Defining JWT verification rules separately from routes
An alternate API design is to define JWT verification rules separately from routing rules, as shown in the below YAML:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-proxy
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tls-cert
    jwtVerificationPolicy:
      providers:
        - name: provider-1
          issuer: example.com
          audiences:
            - audience-1
            - audience-2
          remoteJWKS:
            httpURI:
              uri: https://example.com/jwks.json
              timeout: 1s
            cacheDuration: 5m
      rules:
        - match:
            prefix: /js
        - match:
            prefix: /
          providerName: provider-1
  routes:
    - conditions:
        - prefix: /
      services:
        - name: s1
          port: 80
```

This design has the potential benefit of reducing duplication in the routing rules, by not requiring an entire route to be duplicated in order to express an "except" condition for JWT verification.
It also allows the user to have direct control over the order in which JWT rules are applied, versus being subject to Contour's rules for sorting routes.

However, this design results in two separate places where route-related behavior are defined, and also creates cognitive overhead for users by having two separate ways of ordering match criteria.

### Options considered for working with HTTPProxy Inclusion

#### Option 1: Routes on included HTTPProxies opt into JWT verification
This option largely follows the design laid out above.
JWT providers are defined on the root HTTPProxy.
Routes on either the root HTTPProxy, or any included HTTPProxies, can then opt into JWT verification by specifying `jwtProvider: [name]`.

This option puts the responsibility for opting into JWT verification on the owner of the child HTTPProxy.

#### Option 2: Inclusions of HTTPProxies opt into JWT verification
In this option, the `Include` type itself can opt into JWT verification, by specifying `jwtProvider: [name]`.
All routes in the included HTTPProxy are then opted into JWT verification.
An additional field is added to routes to explicitly opt out of JWT verification (`disableJWTVerification: true`).

This option allows the owner of the root HTTPProxy to opt the child HTTPProxy's routes into JWT verification.
The downside of this model is that it is somewhat inconsistent with the single HTTPProxy model, where routes have to explicitly opt into verification.

#### Option 3: Single JWT provider per root HTTPProxy, enabled by default, with opt-out option
In this option, each root HTTPProxy only allows a single JWT provider.
All routes in the root HTTPProxy, as well as its included HTTPProxies, have JWT verification enabled by default.
An additional field is added to routes to explicitly opt out of JWT verification (`disableJWTVerification: true`).

This option allows the owner of the root HTTPProxy to opt all routes (including any child HTTPProxies' routes) into JWT verification.
The owner of the child HTTPProxies can still opt out specific routes if needed.

The downside of this option is that it does limit each root HTTPProxy to a single provider.

## Compatibility
JWT verification will be an optional feature that is disabled by default.
Existing users should not be affected by its addition.

## Appendix 1: Examples

**HTTPProxy with a single provider, marked as the default:**

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-proxy
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tls-cert
    jwtProviders:
      # This provider is marked as the default so
      # will be applied to all routes unless they
      # opt out.
      - name: provider-1
        default: true
        issuer: example.com
        audiences:
          - audience-1
          - audience-2
        remoteJWKS:
          httpURI:
            uri: https://example.com/jwks.json
            timeout: 1s
          cacheDuration: 5m
  routes:
    # The first route has the default "provider-1"
    # provider applied as a requirement.
    - conditions:
        - prefix: /
      services:
        - name: s1
          port: 80
    # The "/css" route disables all JWT verification.
    - conditions:
        - prefix: /css
      jwtVerificationPolicy:
        disabled: true
      services:
        - name: s1
          port: 80
```

**HTTPProxy with a single provider, NOT marked as the default:**

```yaml
# Note, this proxy definition is functionally the same
# as the previous example, but it uses opt-in behavior
# instead of opt-out behavior.
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-proxy
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tls-cert
    jwtProviders:
      - name: provider-1
        issuer: example.com
        audiences:
          - audience-1
          - audience-2
        remoteJWKS:
          httpURI:
            uri: https://example.com/jwks.json
            timeout: 1s
          cacheDuration: 5m
  routes:
    # The first route has "provider-1"
    # specified as a requirement.
    - conditions:
        - prefix: /
      jwtVerificationPolicy:
        require: provider-1
      services:
        - name: s1
          port: 80
    # The "/css" route does not have JWT
    # verification applied because there
    # is no default provider and it does
    # not explicitly specify one.
    - conditions:
        - prefix: /css
      services:
        - name: s1
          port: 80
```

**HTTPProxy with multiple providers, with one marked as the default:**

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-proxy
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tls-cert
    jwtProviders:
      # This provider is marked as the default so
      # will be applied to all routes unless they
      # opt out.
      - name: provider-1
        default: true
        issuer: example.com
        audiences:
          - audience-1
          - audience-2
        remoteJWKS:
          httpURI:
            uri: https://example.com/jwks.json
            timeout: 1s
          cacheDuration: 5m
      # This is another provider that routes can
      # opt into.
      - name: provider-2
        issuer: foo.com
        remoteJWKS:
          httpURI:
            uri: https://foo.com/jwks.json
            timeout: 1s
          cacheDuration: 5m
  routes:
    # The first route has the default "provider-1"
    # provider applied as a requirement.
    - conditions:
        - prefix: /
      services:
        - name: s1
          port: 80
    # The "/foo" route requires "provider-2" instead
    # of the default.
    - conditions:
        - prefix: /foo
      jwtVerificationPolicy:
        require: provider-2
      services:
        - name: s2
          port: 80
    # The "/css" route disables all JWT verification.
    - conditions:
        - prefix: /css
      jwtVerificationPolicy:
        disabled: true
      services:
        - name: s1
          port: 80
```

**HTTPProxy with a single provider, marked as the default, using inclusion:**

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-root-proxy
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: tls-cert
    jwtProviders:
      # This provider is marked as the default so
      # will be applied to all routes in both the
      # root proxy and its included proxies unless
      # they opt out.
      - name: provider-1
        default: true
        issuer: example.com
        audiences:
          - audience-1
          - audience-2
        remoteJWKS:
          httpURI:
            uri: https://example.com/jwks.json
            timeout: 1s
          cacheDuration: 5m
  includes:
    - name: jwt-verification-child-proxy
      conditions:
        - prefix: /service-1
---  
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-child-proxy
  routes:
    # The first route has the default "provider-1"
    # provider applied as a requirement.
    - conditions:
        - prefix: /
      services:
        - name: s1
          port: 80
    # The "/css" route disables all JWT verification.
    - conditions:
        - prefix: /css
      jwtVerificationPolicy:
        disabled: true
      services:
        - name: s1
          port: 80
```
