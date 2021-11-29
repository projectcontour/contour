## TLS Certificate validation updates

Contour now allows non-server certificates that do not have a CN or SAN set, which mostly fixes
[#2372](https://github.com/projectcontour/contour/issues/2372) and [#3889](https://github.com/projectcontour/contour/issues/3889).

TLS documentation has been updated to make the rules for Secrets holding TLS information clearer.

Those rules are:

For certificates that identify a server, they must:
- be `kubernetes.io/tls` type
- contain `tls.crt`, and `tls.key` keys with the server certificate and key respectively.
- have the first certificate in the `tls.crt` bundle have a CN or SAN field set.

They may:
- have the `tls.crt` key contain a certificate chain, as long as the first certificate in the chain is the server certificate.
- add a `ca.crt` key that contains a Certificate Authority (CA) certificate or certificates.

Certificates in the certificate chain that are not server certificates do not need to have a CN or SAN.

For CA secrets, they must:
- be `Opaque` type
- contain only a `ca.crt` key, not `tls.crt` or `tls.key`

The `ca.crt` key may contain one or more CA certificates, that do not need to have a CN or SAN.

