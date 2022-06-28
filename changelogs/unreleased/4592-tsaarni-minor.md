## Validating revocation status of client certificates

It is now possible to enable revocation check for client certificates validation.
The CRL files must be provided in advance and configured as opaque Secret.
To enable the feature, `httpproxy.spec.virtualhost.tls.clientValidation.crlSecret` is set with the secret name.
