# TLS backend verification

Status: Draft

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

3. The `validation` key will have a mandatory `caSecret` key which is the name of the secret where the ca to be validated is stored.
Certificate Delegation is not in scope.

4. The `validation` key will have an optional `subjectname` key which is expected to be present in the subjectAltName of the presented certificate.
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
            subjectname: backend.example.com 
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

	// SubjectName holds an optional subject name which Envoy will check against the
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

I have some concerns about the efficacy of this mechanism to deliver _verification_ not just _encryption_.
To explain my concern, the way that certificate validation occurs on the public internet is there are two third parties, the CA and the DNS host, both are known to the client and the server but neither are under the direct control of either.
The client looks up the DNS name that it intends to connect too, receives a set of IP addresses, connects to those and compares that the certificate presented is both signed by a CA that it trusts, and carries a subjectAltName that matches the DNS name it originally connected too.
There are a set of compensating controls at work

a. The DNS record may be hijacked, but without the matching certificate this is of little use.

b. The certificate may be stolen, but the attacker must also redirect the DNS entries

c. A certificate may be signed by an alternative CA, which is why pinning certificate fingerprints is a thing.

The probability of a & b happening at the same time is non zero, but represent at least some effort to compensate for the trust issues of each component, this is why c is a thing.
 
Let's compare this to how verification would work inside a Kubernetes cluster:
Envoy, acting as the client, is configured to talk to a set of IP addresses obtained from k8s Endpoint objects--there are no hostnames in play inside a cluster--representing pods for that service.
Envoy is configured by Contour to perform a TLS handshake when connecting to a pod.
This design will add the following constraints to Envoy's TLS handshake.

a. That the certificate presented is signed by the CA who's public key we present to Envoy via the IngressRoute `spec.routes[].backends[]verification.caSecret` key.

b. That the certificate presented contains a subjectAltName that is present in the IngressRoute `spec.routes[].backends[].verification.subjectName` key.

The problem is that both of these checks exist _inside the security boundary of the application which we are attempting to validate_.
Because the certificate is almost certainly going to be signed by a company CA, not one of the public CAs, checking the CA presented by the server matches the material supplied to the client, Envoy, is little more than a shared secret.
It does not assert that the certificate _is_ signed by a trusted CA, only that the certificate _is_ signed by a key to which the client has a matching public component.
This is further weakened by the observation that the service which we are connecting too, the ingressroute, and the caSecret are in the _same namespace_.
It is likely that the secret that the service we are connecting too is also in the same namespace.

SubjectAltName verification is similarly diluted.
There is no compensating control that the client will use the public DNS infrastructure to resolve the IP address to connect too as in the earlier example.
Instead Envoy will connect to a set of IP addresses controlled by the namespace--Ingressroute and endpoint documents live in the same namespace--and ask the endpoint that answers what it's name is.
SubjectAltName becomes just a shared secret, albeit weaker that the CA verification, because its just string matching, there's no crypto in there.

To be clear, verification does offer some improvements over simply doing a TLS handshake, but I cannot convince myself that it is as secure as the public TLS infrastructure.
The same CA, certificate, and subject name parameters can be reused across multiple clusters with exactly the same resultant validation as someone who went to the effort to issue a certificate per service.
