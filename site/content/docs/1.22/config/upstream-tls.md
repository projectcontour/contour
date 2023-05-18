# Upstream TLS

A HTTPProxy can proxy to an upstream TLS backend by annotating the upstream Kubernetes Service or by specifying the upstream protocol in the HTTPProxy [services][2] field.
Applying the `projectcontour.io/upstream-protocol.tls` annotation to a Service object tells Contour that TLS should be enabled and which port should be used for the TLS connection.
The same configuration can be specified by setting the protocol name in the `spec.routes.services[].protocol` field on the HTTPProxy object.
If both the annotation and the protocol field are specified, the protocol field takes precedence.
By default, the upstream TLS server certificate will not be validated, but validation can be requested by setting the `spec.routes.services[].validation` field.
This field has mandatory `caSecret` and `subjectName` fields, which specify the trusted root certificates with which to validate the server certificate and the expected server name.
The `caSecret` can be a namespaced name of the form `<namespace>/<secret-name>`. If the CA secret's namespace is not the same namespace as the `HTTPProxy` resource, [TLS Certificate Delegation][4] must be used to allow the owner of the CA certificate secret to delegate, for the purposes of referencing the CA certificate in a different namespace, permission to Contour to read the Secret object from another namespace.

_**Note:**
If `spec.routes.services[].validation` is present, `spec.routes.services[].{name,port}` must point to a Service with a matching `projectcontour.io/upstream-protocol.tls` Service annotation._

In the example below, the upstream service is named `secure-backend` and uses port `8443`:

```yaml
# httpproxy-example.yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
spec:
  virtualhost:
    fqdn: www.example.com
  routes:
  - services:
    - name: secure-backend
      port: 8443
      validation:
        caSecret: my-certificate-authority
        subjectName: backend.example.com
```

```yaml
# service-secure-backend.yaml
apiVersion: v1
kind: Service
metadata:
  name: secure-backend
  annotations:
    projectcontour.io/upstream-protocol.tls: "8443"
spec:
  ports:
  - name: https
    port: 8443
  selector:
    app: secure-backend

```

If the `validation` spec is defined on a service, but the secret which it references does not exist, Contour will reject the update and set the status of the HTTPProxy object accordingly.
This helps prevent the case of proxying to an upstream where validation is requested, but not yet available.

```yaml
Status:
  Current Status:  invalid
  Description:     route "/": service "tls-nginx": upstreamValidation requested but secret not found or misconfigured
```

## Upstream Validation

When defining upstream services on a route, it's possible to configure the connection from Envoy to the backend endpoint to communicate over TLS.
Two configuration items are required, a CA certificate and a `SubjectName` which are both used to verify the backend endpoint's identity.

The CA certificate bundle for the backend service should be supplied in a Kubernetes Secret.
The referenced Secret must be of type "Opaque" and have a data key named `ca.crt`.
This data value must be a PEM-encoded certificate bundle.

In addition to the CA certificate and the subject name, the Kubernetes service must also be annotated with a Contour specific annotation: `projectcontour.io/upstream-protocol.tls: <port>` ([see annotations section][1]).

_**Note:** This annotation is applied to the Service not the Ingress or HTTPProxy object._

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: blog
  namespace: marketing
spec:
  routes:
    - services:
        - name: s2
          port: 80
          validation:
            caSecret: foo-ca-cert
            subjectName: foo.marketing
```

## Envoy Client Certificate

Contour can be configured with a `namespace/name` in the [Contour configuration file][3] of a Kubernetes secret which Envoy uses as a client certificate when upstream TLS is configured for the backend.
Envoy will send the certificate during TLS handshake when the backend applications request the client to present its certificate.
Backend applications can validate the certificate to ensure that the connection is coming from Envoy.

[1]: annotations.md
[2]: api/#projectcontour.io/v1.Service
[3]: ../configuration#fallback-certificate
[4]: tls-delegation.md
