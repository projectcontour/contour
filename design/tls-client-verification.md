# Client certificate validation (mTLS)

Status: Draft

## Goals

- Allow client certificate validation (mTLS) on contour (performed by Envoy)
- Allow various ways for client certificate validation; Spki, Hash and CA
- Document mTLS configuration and client certificate detail forwarding.

## Non Goals

- Support for k8s ingress documents.
- Repeat configuration details described in Envoy documentation (make references)

## Background

Client certificate validation (mTLS) is supported by Envoy. It should
be possible for `contour` users to utilize this feature.


## High-Level Design

At a high level I propose the following:

1. A new record "clientValidation" is added in spec.virtualhost.tls
   in the IngressRoute.

2. The `clientValidation` contains configuration for client
   certificate validation. Many ways of client certificate validation
   may be specified (all ways supported by Envoy).


### Sample YAML

```
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: kahttp
  namespace: default
spec:
  virtualhost:
    fqdn: kahttp.com
    tls:
      secretName: contour-secret
      clientValidation:
        secretName: clientsecret
        spkis:
          - 2IEpPESU/mmC30tPsnOfbGKdwKdQfN/wZw1QWpjGlmk=
  routes:
    - match: /
      services:
        - name: kahttp
          port: 80
```


## Detailed Design

Since this design proposal required some "learning-by-doing" most of
phase 1 is implemented by https://github.com/heptio/contour/pull/1226

Unit-tests must be added on all appropriate places.


### CAs in Secrets

Same as for [TLS backend verification](tls-backend-verification.md).


### Changes in APIs

The new configuration item `clientValidation` must be parsed and the
CRD's must be updated to validate the new item.


### Changes to the DAG

A new typed will be added to the `dag` package, `ClientValidation`
to capture the validation parameters. It will be added to `SecureVirtualHost`.

```go
package dag

type ClientValidation struct {
	// The CA for client validation.
	*Secret
	// SPKIs used to validate the client certificate
	Spkis []string
	// Hashes used to validate the client certificate
	Hashes []string
}
```

### Changes to internal/envoy

`DownstreamTLSContext()` is extended to take a `clientValidation` this
is a pointer to a structure and may be `nil`. A structure is preferred
before adding a whole bunch of parameters.

```go
type ClientValidation struct {
	Secret *auth.Secret
	Spkis  []string
	Hashes []string
}

func DownstreamTLSContext(secretName string, clientValidation *ClientValidation, tlsMinProtoVersion
  auth.TlsParameters_TlsProtocol, alpnProtos ...string) *auth.DownstreamTlsContext {
  // ...
}
```


### Changes to internal/e2e

Test cases will need to be updated.

### Changes to internal/contour

The `listener.go` will pass ClientValidation data to envoy and
ForwardClientCertDetails is set to SANITIZE_SET.


## Alternatives Considered

To use annotation in the `Ingress` object was considered to
clumsy. Annotation must on `Ingress` level and refere to an individual
virtual host.


## Security Considerations

This proposal assumes that the API server is secure.  If secret or CA
data stored in the API server is modified, verification will be
ineffective.

This proposal also assumes that RBAC is in place and only the owners
of the Service, Secret, IngressRoute documents in a namespace can
modify them.



