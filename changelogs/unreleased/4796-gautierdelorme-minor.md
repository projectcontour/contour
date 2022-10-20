## Optional Client Certificate Validation

By default, client certificates are required but some applications might support different authentication schemes.
You can now set the `httpproxy.spec.virtualhost.tls.clientValidation.optionalClientCertificate` field to `true`. A client certificate will be requested, but the connection is allowed to continue if the client does not provide one.
If a client certificate is sent, it will be verified according to the other properties, which includes disabling validations if `httpproxy.spec.virtualhost.tls.clientValidation.skipClientCertValidation` is set.
