# TLS backend verification

Status: Approved

Contour 0.11 added the ability to communicate via TLS between Envoy and pods in the backend service.
This proposal describes the facility for Envoy to verify the backend service's certificate.

## Goals

- Allow IngressRoute authors to assert that the backend service Envoy is communicating with is using a certificate signed by a certificate authority of their choice.
- Allow IngressRoute authors to assert that the backend service Envoy is communicating with presents a subjectAltName of their choice.

## Non Goals

- Authentication via client side certificates is not in scope.
- Support for k8s ingress documents.

## Background

Contour 0.11 added the `contour.heptio.com/upstream-protocol.tls` Service annotation which told Contour to use TLS when communicating with the members of the backend cluster, the Service's pods.
However, this connection is not validated, Envoy will accept any certificate the client presents.

This proposal seeks to improve the validation of TLS connections .

## High-Level Design

At a high level I propose the following:

1. The CA to be used to validate the certificate presented by the backend should be packaged in a Secret.
The secret will be placed in the namespace of the IngressRoute.

2. IngressRoute's `spec.routes.services.service` entry will grow a new subkey, `validation`.

3. The `validation` key will have a required `caSecret` key which is the name of the secret where the ca to be validated is stored.
Certificate Delegation is not in scope.

4. The `validation` key will have a required `subjectname` key which is expected to be present in the subjectAltName of the presented certificate.
If `subjectname` is not present, any certificate with a valid chain to the supplied CA is considered valid.

5. If `spec.routes.services[].validation` is present, `spec.routes.services[].{name,port}` must point to a service with a matching `contour.heptio.com/upstream-protocol.tls` Service annotation.
If the Service annotation is not present or incorrect, the route is rejected with an appropriate status message.

### Sample YAML

```
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: secure-backend
spec:
  virtualhost: 
    fqdn: www.example.com  
  routes:
    - match: /
      services:
        - name: service
          port: 8443
          validation:
            caSecret: my-certificate-authority
            subjectName: backend.example.com 
```

## Detailed Design

Implementation will consist of these steps:

### CAs in Secrets

The store of CA information is an opaque kubernetes secret.
The secret will be stored in the same namespace as the corresponding IngressRoute.
TLS certificate delegation is not in scope for this proposal.

The secret object should contain one entry named `ca.key`, the constents will be the CA public key material.

Example:
```
% kubectl create secret generic my-certificate-authority --from-file=./ca.key
```

Contour already subscribes to Secrets in all namespaces so Secrets will be piped through to the `dag.KubernetsCache` automatically.

### Changes to the DAG

A new typed will be added to the `dag` package, `UpstreamValidation` to capture the validation parameters.
The name upstream matches Envoy's nomenclature; Cluster members are _upstream_ of Envoy.

```go
package dag

type UpstreamValidation struct {

	// Certificate holds a reference to the Secret containing the CA to be used to
	// verify the upstream connection.
	Certificate *Secret 	

	// SubjectName holds the subject name which Envoy will check against the
	// certificate presented by the upstream.
	SubjectName string
}
```
`UpstreamValidation` will be stored on the `dag.Cluster` object and populated from the YAML described in the previous section.

### Changes to internal/envoy

`envoy.Cluster` already accepts a `dag.Cluster` which gives us a path to pass the `UpstreamValidation` parameters.

`envoy.Clustername` will have to change to reflect `UpstreamValidation` parameters, if present, in the cluster's hash name.
Test cases will need to be updated.

`envoy.UpstreamTLSContext` will have to be refactored to take the `UpstreamValidation` parameters if provided.
Test cases will need to be updated.

### Changes to internal/contour

No changes will be required to the code in `cluster.go`.
Test cases will need to be updated.

### Changes to internal/e2e

Test cases will need to be updated.

## Alternatives Considered

An alternative to storing CA information in Secrets is to store it in ConfigMaps.
This was rejected for two reasons

1. Contour already watches Secret objects, so we get this for free without having to watch a new set of objects.
2. Using ConfigMaps creates a precident for storing other information in ConfigMaps. As ConfigMaps are just homeless annotations, their potential for misuse is endless.

## Security Considerations

This proposal assumes that the API server is secure.
If secret or CA data stored in the API server is modified, verification will be ineffective.

This proposal also assumes that RBAC is in place and only the owners of the Service, Secret, IngressRoute documents in a namespace can modify them.
