# TLS Certificate Delegation

_Status_: Accepted

This document outlines a specification to allow an Ingress Controller to reference a TLS certificate stored in one namespace from an Ingress or IngressRoute document stored in another namespace.

## Goals

- Permit wildcard TLS certificates to be referenced by Ingress/IngressRoute objects in other namespaces.

## Non-goals

- To implement a notion of a default, or fallback, TLS certificate available to all Ingress/IngressRoute objects.
- To permit Secrets to be referenced from any Kubernetes object.
- To permit other classes of Kubernetes objects to be referenced across namespaces.

## Background

Currently the Secret containing the TLS certificate must be co-located in the same namespace as the Ingress or root IngressRoute object referencing that secret.
This requirement complicates deployment patterns where wildcard TLS certificates are used, specifically the use of a wildcard certificate to secure a number of subdomains where the Ingress/IngressRoute records for those subdomains do not share the same namespace as the wildcard TLS certificate.
For example, presenting foo.example.com using the certifiate for \*.example.com when the Ingress/IngressRoute object for foo.example.com does not share the same namespace as the Secret containing \*.example.com.

This proposal introduces a specification, in the form of a simple CRD, whereby the permission to access the contents of a secret containing a TLS certificate is delegated from the owning namespace to one or more other namespaces.

## High-Level Design

- The definition of a Secret's name is extended to recognize a namespace prefix, ie. `secretName: kube-system/wildcard-tls` means the secret `wildcard-tls` in the `kube-system` namespace.
- A `TLSCertificateDelegation` CRD grants the permission to reference the contents of a Secret in the owner's namespace to an Ingress controller operating in the context of an Ingress or IngressRoute object from a different namespace for the purpose of retrieving the TLS certicate.

## Detailed Design

The implementation of this design is in three parts; the addition of a TLSCertificateDelegation CRD, and the modifications to the interpretation the tls stanza in Ingress and IngressRoute objects.

### TLSCertificateDelegation CRD

The TLSCertificateDelegation object records the permission to reference a Secret object from the namespace of the TLSCertificateDelegation object to Ingress or IngressRoute objects in the target namespaces.
This permission is managed by the Ingress controller which has the RBAC permissions to read all the relevant Secrets but currently only allows an Ingress or IngressRoute object to reference secrets from its own namespace.

```
apiVersion: contour.vmware.com/v1
kind: TLSCertificateDelegation
metadata:
  name: wildcards
  namespace: kube-system
spec:
  delegations:
  - secretName: example-com-wildcard
    targetNamespaces:
    - dev-example
    - www-example
  - secretName: google-com
    targetNamespaces: ["finance"]
  - secretName: dev-wildcard
    targetNamespaces: ["*"]
```

In this example permission to reference `kube-system/example-com-wildcard` is delegated to Ingress/IngressRoute objects in the `dev-example` and `www-example` namespaces, `kube-system/google-com` is delegated to Ingress/IngressRoute's in the `finance` namespace, and `kube-system/dev-wildcard` is delegated to _all_ namespaces.  

### Ingress extensions

To support this feature an extension to the `spec.tls.secretName` key will be recognized by Contour.
If the `spec.tls.secretName` field contains a value with a forward slash, ie `namespace1/wildcard` the Secret object referenced will be `wildcard` in the namespace `namespace1`.

If the appropriate secret delegation is in place Contour will use the fully qualified secret name as if it were in the same namespace as the Ingress object.

_Note_: `kubectl` currently permits `spec.tls.secretName` to contain a forward slash (`/`) but it is currently interpreted by Contour as part of the Secret object's name, not a separator.

### IngressRoute extensions

To support this feature an extension to the `spec.virtualhost.tls.secretName` key will be recognized by Contour.
If the `spec.virtualhost.tls.secretName` field contains a value with a forward slash, ie `namespace1/wildcard` the Secret object referenced will be `wildcard` in the namespace `namespace1`.

If the appropriate secret delegation is in place Contour will use the fully qualified secret name as if it were in the same namespace as the IngressRoute root object.

## Alternatives Considered

Alternative designs that extended the IngressRoute specification to allow referencing Secrets by name _and_ namespace were rejected because there was no way to effectively prevent anyone with the permission to construct an IngressRoute object in their own namespace from utilizing the TLS certificate from another namespace.
While it would not be possible for the author to read the contents of the other namespace's secret--only Contour would have that permission--this would allow an attacker to present a certificate from a namespace they do not have permission to read as their own.
In the case of a wildcard certificate this is benficial--it's actually what we want--but also opens up the possibility, when combined with DNS spoofing, of presenting an alternate site using the _real_ SSL certificate, leading to cookie hijacking and MITM attacks.
	
## Security Considerations

Delegation is a necessary security measure because it allows namespace owners to explicitly delegate the permission to reference secrets in their namespace without granting permission to actually _read_ the contents of the certificate.

Permission to use secret delegation is restricted via RBAC and by default is not enabled.
To create a secret delegation CRD the author must have permission to create the secret delegation object in the source Namespace.
