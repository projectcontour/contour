# Client certificate validation (mTLS)

Status: Draft

## Goals

- Allow client certificate validation (mTLS) on contour /performed by Envoy)
- Allow various ways for client certificate validation; Spki, Hash and CA
- Forward client certificate details to the backend (in the XFCC header)
- Document mTLS configuration and client certificate detail forwarding.

## Non Goals

- Support for k8s ingress documents.
- Repeat configuration details described in Envoy documentation (make references)

## Background

Client certificate validation (mTLS) is supported by Envoy. It should
be possible for `contour` users to utilize this feature.


## Implementation phases

The most urgent and also the most straight-forward feature is to
provide client certificate validation. For some backend applications
(but likely not for all) the client cerificate details are needed. So
the proposal is to divide the mTLS implementation in two phases;

1. Add client certificate validation but with hard-coded client
   certification detail forwarding (XFCC header)

2. Add configuration of what is to be forwarded in the XFCC header

Configuration of the XFCC headed can be added later without make any
backward-incompatible changes.

The main problem with XFCC is that it is not really part of the tls
configuration, but rather a backend http connection setting. This
makes me believe that some discussions must be conducted before the
configuration is settled. But in the meanwhile implementation the more
urgent mTLS validation must not be stalled.

The only one of the "other" ingress controllers that mention client
certificate details forwarding I have found is `traefik`;

https://docs.traefik.io/v2.0/middlewares/passtlsclientcert/

For some reason they do not use the "standard" XFCC header. Never the
less is may be wise to mimic this approach in `contour`, for instance
herd-code forwaring of the entire certificate rather than the hash in
phase 1.


## High-Level Design

At a high level I propose the following:

1. A new record "clientValidation" is added in spec.virtualhost.tls
   in the IngressRoute.

2. The `clientValidation` contains configuration for client
   certificate validation. Many ways of client certificate validation
   may be specified (all ways supported by Envoy).

3. For phase 1 the configuration of what to forward in the XFCC header is
   hard-coded to SANITIZE_SET and certificate hash.

The design of phase 2, the XFCC configuration, is dependent on the
decided configuration so design proposal is postponed.


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



## Phase 2 considerations

I think the XFCC settings can be done "per route" which would mean
that configuration would go into `spec.routes`.

As mentioned above it might be better to hard-code forwarding of the
entire certificate in phase 1. I would stick with `SANITIZE_SET`
though, the "append" settings seems appropriate if you have a chain of
Envoy's like in `istio` for intance.

It is *really* hard to find actual use cases where XFCC is used by
some backend application. BTW I have not received in "internal"
requirements on this, only for mTLS validation (phase 1).

Finally a comment from our security expert;

> Actually this makes me wonder, what is the convention of using XFCC –
> do you known in which scenario would the Hash be useful for the
> upstream server?  Using hash would really require that upstream server
> knows about the client certificate(s) in advance, and if client
> certificate is renewed, this information would need to be updated to
> the upstream server (since hash would change).

And my reply;

I have chosen the Envoy defaults for XFCC because I don't know
anything about the use in applications. My guess is that the
client-certificate hash will is used something like this;

 1. Envoy validates the (individual) client-cerificate, so the
    backend can be sure that the certificate is valid.

 2. The backend app will need client data for this individual client
    and makes a DB-query with the certificate-hash as key.

I don't think the backend needs (or wants) the certificate itself,
just a validated way to get client-data.


References;

* Envoy API documentation;
  https://protect2.fireeye.com/url?k=68a52038-342f02f7-68a560a3-0cc47ad93ea4-f01aad0c9bf4d1c4&q=1&u=https%3A%2F%2Fwww.envoyproxy.io%2Fdocs%2Fenvoy%2Flatest%2Fapi-v2%2Fconfig%2Ffilter%2Fnetwork%2Fhttp_connection_manager%2Fv2%2Fhttp_connection_manager.proto%23envoy-api-msg-config-filter-network-http-connection-manager-v2-httpconnectionmanager-setcurrentclientcertdetails

* Envoy config doc;
  https://protect2.fireeye.com/url?k=794671c7-25cc5308-7946315c-0cc47ad93ea4-ee50bfb2576ceb84&q=1&u=https%3A%2F%2Fwww.envoyproxy.io%2Fdocs%2Fenvoy%2Flatest%2Fconfiguration%2Fhttp_conn_man%2Fheaders%23config-http-conn-man-headers-x-forwarded-client-cert

* Traefic XFCC; https://docs.traefik.io/v2.0/middlewares/passtlsclientcert/
