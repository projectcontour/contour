# JWT Verification

Contour supports verifying JSON Web Tokens (JWTs) on incoming requests, using Envoy's [jwt_authn HTTP filter][1].
Specifically, the following properties can be checked:
- issuer field
- audiences field
- signature, using a configured JSON Web Key Store (JWKS)
- time restrictions (e.g. expiration, not before time)

If verification succeeds, the request will be proxied to the appropriate upstream.
If verification fails, an HTTP 401 (Unauthorized) will be returned to the client.

JWT verification is only supported on TLS-terminating virtual hosts.

## Configuring providers and rules

A JWT provider is configured for an HTTPProxy's virtual host, and defines how to verify JWTs:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: example-com-tls-cert
    jwtProviders:
      - name: provider-1
        issuer: example.com
        audiences:
          - audience-1
          - audience-2
        remoteJWKS:
          uri: https://example.com/jwks.json
          timeout: 1s
          cacheDuration: 5m
        forwardJWT: true
  routes:
    ...
```

The provider above requires JWTs to have an issuer of example.com, an audience of either audience-1 or audience-2, and a signature that can be verified using the configured JWKS.
It also forwards the JWT to the backend via the `Authorization` header after successful verification.

To apply a JWT provider as a requirement to a given route, specify a `jwtVerificationPolicy` for the route:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: example-com-tls-cert
    jwtProviders:
      - name: provider-1
        ...
  routes:
    - conditions:
        - prefix: /
      jwtVerificationPolicy:
        require: provider-1
      services:
        - name: s1
          port: 80
    - conditions:
        - prefix: /css
      services:
        - name: s1
          port: 80
```

In the above example, the default route requires requests to carry JWTs that can be verified using provider-1.
The second route _excludes_ requests to paths starting with `/css` from JWT verification, because it does not have a JWT verification policy.

### Configuring TLS validation for the JWKS server

By default, the JWKS server's TLS certificate will not be validated, but validation can be requested by setting the `spec.virtualhost.jwtProviders[].remoteJWKS.validation` field.
This field has mandatory `caSecret` and `subjectName` fields, which specify the trusted root certificates with which to validate the server certificate and the expected server name.
The `caSecret` can be a namespaced name of the form `<namespace>/<secret-name>`.
If the CA secret's namespace is not the same namespace as the `HTTPProxy` resource, [TLS Certificate Delegation][5] must be used to allow the owner of the CA certificate secret to delegate, for the purposes of referencing the CA certificate in a different namespace, permission to Contour to read the Secret object from another namespace.

**Note:** If `spec.virtualhost.jwtProviders[].remoteJWKS.validation` is present, `spec.virtualhost.jwtProviders[].remoteJWKS.uri` must have a scheme of `https`.

## Setting a default provider

The previous section showed how to explicitly require JWT providers for specific routes.
An alternate approach is to define a JWT provider as the default by specifying `default: true` for it, in which case it is automatically applied to all routes unless they disable JWT verification.
The example from the previous section could alternately be configured as follows:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: example-com-tls-cert
    jwtProviders:
      - name: provider-1
        default: true
        ...
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

In this case, the default route automatically has provider-1 applied, while the `/css` route explicitly disables JWT verification.

One scenario where setting a default provider can be particularly useful is when using [HTTPProxy inclusion][2].
Setting a default provider in the root HTTPProxy allows all routes in the child HTTPProxies to automatically have JWT verification applied.
For example:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-root
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: example-com-tls-cert
    jwtProviders:
      - name: provider-1
        default: true
        ...
  includes:
    - name: jwt-verification-child
      namespace: default
      conditions:
        - prefix: /blog
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: jwt-verification-child
  namespace: default
spec:
  routes:
    - conditions:
        - prefix: /
      services:
        - name: s1
          port: 80
```

In this case, all routes in the child HTTPProxy will automatically have JWT verification applied, without the owner of this HTTPProxy needing to configure it explicitly.

## API documentation

For more information on the HTTPProxy API for JWT verification, see:

- [JWTProvider][3]
- [JWTVerificationPolicy][4]


[1]: https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/jwt_authn_filter
[2]: /docs/{{< param version >}}/config/inclusion-delegation/
[3]: /docs/{{< param version >}}/config/api/#projectcontour.io/v1.JWTProvider
[4]: /docs/{{< param version >}}/config/api/#projectcontour.io/v1.JWTVerificationPolicy
[5]: tls-delegation.md
