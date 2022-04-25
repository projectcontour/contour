# External client certificate validation

Status: Draft

This document outlines how to add support for authentication of external clients by validating their client certificates.

## Goals

- Allow Contour to be used to protect the backend services from access of unauthorized external clients.
- Allow authentication of external clients by having Envoy validate that client certificates are signed by trusted CA (Certificate Authority).
- Allow configuration of trusted CA certificate(s) for validating the client certificates.

## Non Goals

- Fine grained authorization on level of individual request (example of non-goal: only client X can access resource Y on backend service Z).

## Background

In TLS (and HTTPS) often only the client authenticates the server.
TLS supports optional authentication of client.
Client certificate based authentication is typically used in machine-to-machine (M2M) communication e.g. to protect sensitive REST APIs.
The application acting as TLS client authenticates itself towards the TLS server by using x509 certificate and by providing proof of possession of the corresponding private key.

## High-Level Design

Envoy supports following options for certificate based client authentication:

1. Verify that a chain of trust can be established from the presented client certificate to the configured trusted root CA certificate (validation of certificate)
2. In addition to 1, verify that the subject alternative name of the client is one of the names listed in the configuration (validation of client identity)
3. Verify that the client certificate hash or subject public key hash matches with configured hash (hash pinning)

Option 1) is proposed to be implemented in this document.
It is sufficient for the very simplest authentication use cases.

At a high level following CRD change is proposed:

- A new record `clientValidation` is added in `spec.virtualhost.tls` in the `HTTPProxy`.
- The `clientValidation` contains parameter `caSecret` which is a reference to a secret containing trusted CA certificate for validating client certificates.

Sample YAML

```
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: client-validation-example
  namespace: default
spec:
  virtualhost:
    fqdn: example.com
    tls:
      secretName: server-credentials
      clientValidation:
        caSecret: ca-cert-for-client-validation
  routes:
    - services:
        - name: service
          port: 80
```

### Trusted CAs in Secrets

The same approach shall be followed for configuring trusted CA certificates as is used currently to store the CA certificates for backend (Envoy upstream) validation:

The CA certificate is stored in an opaque Kubernetes secret.
The secret will be stored in the same namespace as the corresponding `HTTPProxy` object.
The secret object shall contain entry named `ca.crt`.
The contents shall be the CA certificate in PEM format.
The file may contain "PEM bundle", that is, a list of CA certificates concatenated in single file.

Example:
```
% kubectl create secret generic ca-cert-for-client-validation --from-file=./ca.crt
```

TLS certificate delegation is not in scope for CA certificates.

## Detailed Design

The new configuration item `spec.virtualhost.tls.clientValidation` must be parsed and the CRD's must be updated to validate the new item.

Client certificate validation is enabled in Envoy by setting `auth.DownstreamTlsContext.RequireClientCertificate` value to `true` and by adding trusted CA certificates to `auth.CommonTlsContext.ValidationContextType`.

## Alternatives Considered

To use annotation in the `Ingress` object was considered to clumsy.
Annotation must on `Ingress` level and refer to an individual virtual host.

## Security Considerations

This proposal assumes that the API server is secure.
If secret or CA data stored in the API server is modified, verification will be ineffective.

This proposal also assumes that RBAC is in place and only the owners of the Service, Secret, HTTPProxy documents in a namespace can modify them.
