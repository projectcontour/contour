## Gateway provisioner: generate xDS TLS certs directly

The Gateway provisioner now generates xDS TLS certificates directly, rather than using a "certgen" job to trigger certificate generation.
This simplifies operations and reduces the RBAC permissions that the provisioner requires.
Certificates will still be rotated each time the provisioner is upgraded to a new version.
