
We are delighted to present version 1.1.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

All Contour users should upgrade to Contour 1.1.0 and Envoy 1.12.2 as there are some critical vulnerabilities that should be patched.

## New and improved 

Contour 1.1.0 includes several new features as well as the usual smattering of fixes and minor improvements.

### Envoy CVEs

Three CVEs have been addressed by Envoy, the highest security defect is considered 9.0 (critical) severity.

[See the Envoy 1.12.2 announcement for details on the vulnerabilities.](https://groups.google.com/forum/#!topic/envoy-announce/Z4_JwSksPpY)

As Envoy have not provided fixes for Envoy 1.11 and earlier all Contour users should also upgrade to Envoy 1.12.2.

### Prefix Rewrite Support

Prefix rewrite support was removed right before HTTPProxy was released in Contour v1.0.0. Support has now been added back to HTTPProxy and is expressed as a `pathRewritePolicy`:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: rewrite-example
spec:
  virtualhost:
    fqdn: rewrite.bar.com
  routes:
  - services:
    - name: s1
      port: 80
    conditions:
    - prefix: /v1/api
    pathRewritePolicy:
      replacePrefix:
      - prefix: /v1/api
        replacement: /app/api/v1
```

Thanks @jpeach

### Support for specifying a service's protocol in HTTPProxy

Contour now supports defining what protocol Envoy should use when proxying to an upstream application.
(See design doc: https://github.com/projectcontour/contour/blob/master/design/httpproxy-protocol-selection.md)

A new field has been added to the `Service` spec which encodes the protocol data.

_Note: Previously, that data was extracted from the Kubernetes service annotation `projectcontour.io/upstream-protocol.{protocol}`._

```yaml
spec:
  virtualhost:
    fqdn: dashboard.kubernetes.com
    tls:
      secretName: kubernetes-dashboard-tls
  routes:
    - conditions:
      - prefix: /
      services:
        - name: kubernetes-dashboard
          protocol: https # <--- NEW FIELD
          port: 443
```

Thanks @mattmoor

### Support per-Split header manipulation

Adds support for adding and removing request or response headers for each service target in a Contour HTTPProxy resource.
Manipulating headers are also supported per-Service or per-Route. 
Headers can be set or removed from the request or response as follows:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: header-manipulation
  namespace: default
spec:
  virtualhost:
    fqdn: headers.bar.com
  routes:
    - services:
        - name: s1
          port: 80
          requestHeaderPolicy:
            set:
              - name: X-Foo
                value: bar
            remove:
              - X-Baz
          responseHeaderPolicy:
            set:
              - name: X-Service-Name
                value: s1
            remove:
              - X-Internal-Secret
```

Thanks @mattmoor

### Requests to external domains rewrite host via externalName service

To proxy to another resource outside the cluster (e.g. A hosted object store bucket for example), configure that external resource in a service type externalName.
Then define a headerRequestPolicy which replaces the Host header with the value of the external name service defined previously.

See the [externalName](https://projectcontour.io/docs/master/httpproxy/#externalname) section of HTTPProxy docs for more details.

_Note: The host rewrite only applied to services which target externalNames._

Thanks: @stevesloka

### Minor improvements

- Various documentation upgrades to projectcontour.io
- Contour uses SaveRegex now in Envoy configuration
- Contour is built with Go 1.13.5
- Add namespace env var to certgen job. Thanks @dhxgit

## Bug fixes

### Reject a TCPProxy HTTPProxy without Valid TLS Details 

To be a valid HTTPProxy, if the tcpproxy stanza is provided, the HTTPProxy must also feature a virtualhost.tls spec with either passthrough: true, or a valid secretName.

Fixes #1958

Thanks @davecheney

### 301 Upgrade Insecure Routes Irrespective of TCP Proxying

Clean the HTTPProxy spec.virtualhost.tls validation logic and fix the last issue with HTTPProxy TCPProxy logic.

If a HTTPProxy is using TCP proxying then its secure port is forwarded according to the spec.tcpproxy schema.
The insecure port, port 80 is not tcp forwarded and remains connected to a L7 http connection manager.
Because by definition a HTTPProxy using TCP proxying must supply a valid spec.virtualhost.tls block, our 301 upgrade logic applies.
Thus, after this change, if a route on the insecure listener is not using permitInsecure: true, it will by 301 upgraded.

Fixes #1952

Thanks @davecheney

### Reject Certificates without CN or SubjectAltName

Envoy crashes when processing a TLS certificate that does not have SubjectAltNames or a CN field in the Subject, so Contour now rejects any certificate which lacks a Subject CommonName (CN) or SubjectAltName extension.

Upstream Envoy issue: https://github.com/envoyproxy/envoy/issues/9182

Fixes #1965

Thanks @davecheney

### Run contour & cert-gen job as non-root

Adds `securityContext` to Contour & certgen jobs manifest examples to not run as root.

Thanks @surajssd

### Cert gen now accepts certificate lifetime argument
 
 A `certificate-lifetime` argument has been added to the Contour certgen job which allows for a duration in days the certificates used for Envoy<>Contour communication to be valid.
 
 Fixes #2017
 
 Thanks @tsaarni
 
### Other bug fixes

- Contour no longer generates ingress_https route for tcpproxy vhost. Fixes #1954. 
- Quickstart can be re-applied to an existing cluster 

## Upgrading

Please consult the [Upgrading](/docs/upgrading.md) document for further information on upgrading from Contour 1.0.1 to Contour 1.1.0.