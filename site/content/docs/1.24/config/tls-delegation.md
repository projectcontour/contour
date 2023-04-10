# TLS Certificate Delegation

In order to support wildcard certificates, TLS certificates for a `*.somedomain.com`, which are stored in a namespace controlled by the cluster administrator, Contour supports a facility known as TLS Certificate Delegation.
This facility allows the owner of a TLS certificate to delegate, for the purposes of referencing the TLS certificate, permission to Contour to read the Secret object from another namespace.
Delegation works for both HTTPProxy and Ingress resources, however it needs an annotation to work with Ingress v1.

The [`TLSCertificateDelegation`][1] resource defines a set of `delegations` in the `spec`.
Each delegation references a `secretName` from the namespace where the `TLSCertificateDelegation` is created as well as describing a set of `targetNamespaces` in which the certificate can be referenced.
If all namespaces should be able to reference the secret, then set `"*"` as the value of `targetNamespaces` (see example below).

```yaml
apiVersion: projectcontour.io/v1
kind: TLSCertificateDelegation
metadata:
  name: example-com-wildcard
  namespace: www-admin
spec:
  delegations:
    - secretName: example-com-wildcard
      targetNamespaces:
      - example-com
    - secretName: another-com-wildcard
      targetNamespaces:
      - "*"
```

In this example, the permission for Contour to reference the Secret `example-com-wildcard` in the `admin` namespace has been delegated to HTTPProxy and Ingress objects in the `example-com` namespace.
Also, the permission for Contour to reference the Secret `another-com-wildcard` from all namespaces has been delegated to all HTTPProxy and Ingress objects in the cluster.

To reference the secret from an HTTPProxy or Ingress v1beta1 you must use the slash syntax in the `secretName`:
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: www
  namespace: example-com
spec:
  virtualhost:
    fqdn: foo2.bar.com
    tls:
      secretName: www-admin/example-com-wildcard
  routes:
    - services:
        - name: s1
          port: 80
```

To reference the secret from an Ingress v1 you must use the `projectcontour.io/tls-cert-namespace` annotation:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    projectcontour.io/tls-cert-namespace: www-admin
  name: www
  namespace: example-com
spec:
  rules:
  - host: foo2.bar.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: s1
            port:
              number: 80
  tls:
  - hosts:
    - foo2.bar.com
    secretName: example-com-wildcard
```


[0]: https://github.com/projectcontour/contour/issues/3544
[1]: /docs/{{< param version >}}/config/api/#projectcontour.io/v1.TLSCertificateDelegation
