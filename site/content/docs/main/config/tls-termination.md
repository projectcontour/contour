# TLS Termination

HTTPProxy follows a similar pattern to Ingress for configuring TLS credentials.

You can secure a HTTPProxy by specifying a Secret that contains TLS private key and certificate information.
If multiple HTTPProxies utilize the same Secret, the certificate must include the necessary Subject Authority Name (SAN) for each fqdn.

Contour (via Envoy) requires that clients send the Server Name Indication (SNI) TLS extension so that requests can be routed to the correct virtual host.
Virtual hosts are strongly bound to SNI names.
This means that the Host header in HTTP requests must match the SNI name that was sent at the start of the TLS session.

Contour also follows a "secure first" approach.
When TLS is enabled for a virtual host, any request to the insecure port is redirected to the secure interface with a 301 redirect.
Specific routes can be configured to override this behavior and handle insecure requests by enabling the `spec.routes.permitInsecure` parameter on a Route.

The TLS secret must:
- be a Secret of type `kubernetes.io/tls`. This means that it must contain keys named `tls.crt` and `tls.key` that contain the certificate and private key to use for TLS, in PEM format.

The TLS secret may also:
- add any chain CA certificates required for validation into the `tls.crt` PEM bundle. If this is the case, the serving certificate must be the first certificate in the bundle and the intermediate CA certificates must be appended in issuing order.

```yaml
# ingress-tls.secret.yaml
apiVersion: v1
data:
  tls.crt: base64 encoded cert
  tls.key: base64 encoded key
kind: Secret
metadata:
  name: testsecret
  namespace: default
type: kubernetes.io/tls
```

The HTTPProxy can be configured to use this secret using `tls.secretName` property:

```yaml
# httpproxy-tls.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: tls-example
  namespace: default
spec:
  virtualhost:
    fqdn: foo2.bar.com
    tls:
      secretName: testsecret
  routes:
    - services:
        - name: s1
          port: 80
```

If the `tls.secretName` property contains a slash, eg. `somenamespace/somesecret` then, subject to TLS Certificate Delegation, the TLS certificate will be read from `somesecret` in `somenamespace`.
See TLS Certificate Delegation below for more information.

The TLS **Minimum Protocol Version** a virtual host should negotiate can be specified by setting the `spec.virtualhost.tls.minimumProtocolVersion`:

- 1.3
- 1.2  (Default)

## Fallback Certificate

Contour provides virtual host based routing, so that any TLS request is routed to the appropriate service based on both the server name requested by the TLS client and the HOST header in the HTTP request.

Since the HOST Header is encrypted during TLS handshake, it can’t be used for virtual host based routing unless the client sends HTTPS requests specifying hostname using the TLS server name, or the request is first decrypted using a default TLS certificate.

Some legacy TLS clients do not send the server name, so Envoy does not know how to select the right certificate. A fallback certificate is needed for these clients.

_**Note:**
The minimum TLS protocol version for any fallback request is defined by the `minimum TLS protocol version` set in the Contour configuration file.
Enabling the fallback certificate is not compatible with TLS client authentication._

### Fallback Certificate Configuration

First define the `namespace/name` in the [Contour configuration file][1] of a Kubernetes secret which will be used as the fallback certificate.
Any HTTPProxy which enables fallback certificate delegation must have the fallback certificate delegated to the namespace in which the HTTPProxy object resides.

To do that, configure `TLSCertificateDelegation` to delegate the fallback certificate to specific or all namespaces (e.g. `*`) which should be allowed to enable the fallback certificate.
Finally, for each root HTTPProxy, set the `Spec.TLS.enableFallbackCertificate` parameter to allow that HTTPProxy to opt-in to the fallback certificate routing.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: fallback-tls-example
  namespace: defaultub
spec:
  virtualhost:
    fqdn: fallback.bar.com
    tls:
      secretName: testsecret
      enableFallbackCertificate: true
  routes:
    - services:
        - name: s1
          port: 80
---
apiVersion: projectcontour.io/v1
kind: TLSCertificateDelegation
metadata:
  name: fallback-delegation
  namespace: www-admin
spec:
  delegations:
    - secretName: fallback-secret-name
      targetNamespaces:
      - "*"
```

## Permitting Insecure Requests

A HTTPProxy can be configured to permit insecure requests to specific Routes.
In this example, any request to `foo2.bar.com/blog` will not receive a 301 redirect to HTTPS, but the `/` route will:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: tls-example-insecure
  namespace: default
spec:
  virtualhost:
    fqdn: foo2.bar.com
    tls:
      secretName: testsecret
  routes:
    - services:
        - name: s1
          port: 80
    - conditions:
      - prefix: /blog
      permitInsecure: true
      services:
        - name: s2
          port: 80
```

## Client Certificate Validation

It is possible to protect the backend service from unauthorized external clients by requiring the client to present a valid TLS certificate.
Envoy will validate the client certificate by verifying that it is not expired and that a chain of trust can be established to the configured trusted root CA certificate.
Only those requests with a valid client certificate will be accepted and forwarded to the backend service.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: with-client-auth
spec:
  virtualhost:
    fqdn: www.example.com
    tls:
      secretName: secret
      clientValidation:
        caSecret: client-root-ca
  routes:
    - services:
        - name: s1
          port: 80
```

The preceding example enables validation by setting the optional `clientValidation` attribute.
Its mandatory attribute `caSecret` contains a name of an existing Kubernetes Secret that must be of type "Opaque" and have only a data key named `ca.crt`.
The data value of the key `ca.crt` must be a PEM-encoded certificate bundle and it must contain all the trusted CA certificates that are to be used for validating the client certificate.
If the Opaque Secret also contains one of either `tls.crt` or `tls.key` keys, it will be ignored.

When using external authorization, it may be desirable to use an external authorization server to validate client certificates on requests, rather than the Envoy proxy.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: with-client-auth-and-ext-authz
spec:
  virtualhost:
    fqdn: www.example.com
    authorization:
      # external authorization server configuration
    tls:
      secretName: secret
      clientValidation:
        caSecret: client-root-ca
        skipClientCertValidation: true
  routes:
    - services:
        - name: s1
          port: 80
```

In the above example, setting the `skipClientCertValidation` field to `true` will configure Envoy to require client certificates on requests and pass them along to a configured authorization server.
Failed validation of client certificates by Envoy will be ignored and the `fail_verify_error` [Listener statistic][2] incremented.
If the `caSecret` field is omitted, Envoy will request but not require client certificates to be present on requests.

Optionally, you can enable certificate revocation check by providing one or more Certificate Revocation Lists (CRLs).
Attribute `crlSecret` contains a name of an existing Kubernetes Secret that must be of type "Opaque" and have a data key named `crl.pem`.
The data value of the key `crl.pem` must be one or more PEM-encoded CRLs concatenated together.
Large CRL lists are not supported since individual Secrets are limited to 1MiB in size.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: with-client-auth-and-crl-check
spec:
  virtualhost:
    fqdn: www.example.com
    tls:
      secretName: secret
      clientValidation:
        caSecret: client-root-ca
        crlSecret: client-crl
  routes:
    - services:
        - name: s1
          port: 80
```

CRLs must be available from all relevant CAs, including intermediate CAs.
Otherwise clients will be denied access, since the revocation status cannot be checked for the full certificate chain.
This behavior can be controlled by `crlOnlyVerifyLeafCert` field.
If the option is set to `true`, only the certificate at the end of the certificate chain will be subject to validation by CRL.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: with-client-auth-and-crl-check-only-leaf
spec:
  virtualhost:
    fqdn: www.example.com
    tls:
      secretName: secret
      clientValidation:
        caSecret: client-root-ca
        crlSecret: client-crl
        crlOnlyVerifyLeafCert: true
  routes:
    - services:
        - name: s1
          port: 80
```

## TLS Session Proxying

HTTPProxy supports proxying of TLS encapsulated TCP sessions.

_Note_: The TCP session must be encrypted with TLS.
This is necessary so that Envoy can use SNI to route the incoming request to the correct service.

If `spec.virtualhost.tls.secretName` is present then that secret will be used to decrypt the TCP traffic at the edge.

```yaml
# httpproxy-tls-termination.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
  namespace: default
spec:
  virtualhost:
    fqdn: tcp.example.com
    tls:
      secretName: secret
  tcpproxy:
    services:
    - name: tcpservice
      port: 8080
    - name: otherservice
      port: 9999
      weight: 20
```

The `spec.tcpproxy` key indicates that this _root_ HTTPProxy will forward the de-encrypted TCP traffic to the backend service.

### TLS Session Passthrough

If you wish to handle the TLS handshake at the backend service set `spec.virtualhost.tls.passthrough: true` indicates that once SNI demuxing is performed, the encrypted connection will be forwarded to the backend service.
The backend service is expected to have a key which matches the SNI header received at the edge, and be capable of completing the TLS handshake. This is called SSL/TLS Passthrough.

```yaml
# httpproxy-tls-passthrough.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
  namespace: default
spec:
  virtualhost:
    fqdn: tcp.example.com
    tls:
      passthrough: true
  tcpproxy:
    services:
    - name: tcpservice
      port: 8080
    - name: otherservice
      port: 9999
      weight: 20
```

[1]: ../configuration#fallback-certificate
[2]: https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/stats#tls-statistics
