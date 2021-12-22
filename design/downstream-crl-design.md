# Downstream Certificate Revocation List Proposal

Status: Draft

## Abstract
This proposal covers the implementation of CRLs (Certificate Revocation List) in Contours DownstreamValidation, using the `crl` field in Envoy's [extensions.transport_sockets.tls.v3.CertificateValidationContext](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/transport_sockets/tls/v3/common.proto#extensions-transport-sockets-tls-v3-certificatevalidationcontext) field.

## Background

Envoy supports specifying a PEM encoded CRL for use when validating a peer certificate.
Currently Contour allows specifying a CA Certificate bundle that is used to validate a client.
This is done using Envoy's Common TLS Context inside of a Downstream TLS context.
Envoy's Common TLS Context also supports a CRL data source, which is a set of CRL's that are used when validating a clients certificate.
This CRL data source can be exposed the same way that we expose the CA Certificate data source, through Contour's `DownstreamValidation` structure.

## Goals
- Allow a Kubernetes secret containing a PEM encoded CRL to be used when validating client connections
- Perform simple validation of the K8s secret
  - Validate that it is indeed a PEM CRL object

## Non Goals
- Upstream Validation
- Support for `only_verify_leaf_cert_crl`
  - Not in a released version of envoy yet, still in development


## High-Level Design
The design would for the most part mirror the implementation of `DownstreamValidation.CACertificate` and `DownstreamValidation.SkipClientCertValidation`.

Sample YAML

```yaml
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
        caCrlSecret: crl-for-client-validation
  routes:
    - services:
        - name: service
          port: 80
```

## Detailed Design

### CRL Secret

The same approach shall be followed for configuring revocation lists as is used currently to store the CA certificates for client validation

The CRL is stored in an opaque Kubernetes secret.
The secret will be stored in the same namespace as the corresponding `HTTPProxy` object.
The secret object shall contain entry named `crl.pem`.
The constents shall be the CRL in PEM format.
The file may contain "PEM bundle", that is, a list of CRLs concatenated in single file.

Example:

```bash
kubectl create secret generic crl-for-client-validation --from-file=./crl.pem
```

### httpproxy.DownstreamValidation additions

```go
// DownstreamValidation defines how to verify the client certificate.
type DownstreamValidation struct {
	// Name of a Kubernetes secret that contains a CA certificate bundle.
	// The client certificate must validate against the certificates in the bundle.
	// If specified and SkipClientCertValidation is true, client certificates will
	// be required on requests.
	// +optional
	// +kubebuilder:validation:MinLength=1
	CACertificate string `json:"caSecret,omitempty"`

	// SkipClientCertValidation disables downstream client certificate
	// validation. Defaults to false. This field is intended to be used in
	// conjunction with external authorization in order to enable the external
	// authorization server to validate client certificates. When this field
	// is set to true, client certificates are requested but not verified by
	// Envoy. If CACertificate is specified, client certificates are required on
	// requests, but not verified. If external authorization is in use, they are
	// presented to the external authorization server.
	// +optional
	SkipClientCertValidation bool `json:"skipClientCertValidation"`

	// Name of a Kubernetes Opaque secret that contains a concatenated list of
  // pem encoded crls.
	// This will be used to verify that a client certificate has not been revoked
	// +optional
	// +kubebuilder:validation:MinLength=1
	CACertificateRevocationList string `json:"caCrlSecret,omitempty"`
}
```

### Envoy Configuration

The new fields from `spec.virtualhost.tls.clientValidation` must be parsed and mapped to `auth.CommonTlsContext.ValidationContextType`

- `spec.virtualhost.tls.clientValidation.caCrlSecret` -> `envoy_v3_tls.CommonTlsContext_ValidationContext.Crl`
  - `internal.envoy.v3.auth.validationContext()` will be updated to accept a `crl` the same way it accepts a `ca`
  - If empty it will not set the `crl` field, maintaining backward compatibility

### Secret validation

Currently basic validation is done on Opaque secrets with a `ca.crt` key, just to make sure that the length is non-zero.
This validation would also be performed for the CRL.

Validation is also performed to make sure that the CA Bundle has the correct PEM header and is of type `CERTIFICATE`.
The same validation would be performed, but checking for type `CRL`

## Alternatives Considered
N/A

## Security Considerations
N/A

## Compatibility
This change should be additive, so there should be no compatibility issues

## Implementation
N/A

## Open Issues

- How to handle an error where a user supplies a CRL for one certificate in a chain, but not all. In which case Envoy will fail to verify
  - Short of decoding both the certificate list and CRL list, and making sure one exists for both, I don't have another answer. This seems out of scope for Contour here.
