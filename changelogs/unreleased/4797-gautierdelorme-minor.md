## Client Certificate Details Forwarding

HTTPProxy now supports passing certificate data through the `x-forwarded-client-cert` header to let applications use details from client certificates (e.g. Subject, SAN...).
Since the certificate (or the certificate chain) could exceed the web server header size limit, you have the ability to select what specific part of the certificate to expose in the header through the `httpproxy.spec.virtualhost.tls.clientValidation.forwardClientCertificate` field.
Read more about the supported values in the [Envoy documentation](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_conn_man/headers#x-forwarded-client-cert).
