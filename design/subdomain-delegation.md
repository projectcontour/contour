# Subdomain Delegation

_Status_: Draft, in review.

This document outlines an addition to the [IngressRoute][0] specification to permit subdomains to inherit domain wide parameters such as wildcard TLS certificates without exposing that configuration to the specific subdomain.

## Goals

- Permit a number of subdomains to be served using a wildcard TLS certificates without requiring the IngressRoute record for that subdomain to be a part of the namespace holding the TLS wildcard certificate's Secret.

## Non-goals

- To support hosting a Kubernetes service for a wildcard domain (eg, \*.whatever.net) itself. 

## Background

Currently the Secret containing the TLS certificate must be co-located in the same namespace as the Ingress or root IngressRoute object referencing that secret.
This requirement complicates deployment patterns where wildcard TLS certificates are used, specifically the use of a wildcard certificate to secure a number of subdomains where the Ingress/IngressRoute records for those subdomains do not share the same namespace as the wildcard TLS certificate.
For example, presenting foo.example.com using the certificate for \*.example.com when the Ingress/IngressRoute object for foo.example.com does not share the same namespace as the Secret containing \*.example.com.

This proposal introduces a new type of delegation, subdomain delegation, which permits the root IngressRoute object to define a TLS certificate to be used for a set of IngressRoute resources denoted as subdomains.

## High-Level Design

- `spec.virtualhost.fqdn` can be treated as a domain _suffix_, if the `spec.delegate` key is present.
- `spec.virtualhost.tls.secretName` indicates the certificate to be applied to all subdomains delegated from this domain root.

## Detailed Design

This proposal extends the IngressRoute specification to permit delegation of subdomains.

```
apiVersion: contour.heptio.com/v1beta1
kind: IngressRoute
metadata:
  name: domain-root
  namespace: mycompany
spec:
  virtualhost:
    fqdn: .mycompany.com
    tls:
      secretName: mycompany-wildcard-tls
  subdomains:
  - subdomain: mail.mycompany.com
    delegate:
      name: mail
      namespace: msexchange
  - subdomain: www.mycompany.com
    delegate:
      name: wordpress
```

In this example the configuration for `mail.mycompany.com` is found in the IngressRoute object `msexchange/mail`, `www.mycompany.com` in `mycompany/wordpress`.
The TLS certificate `mycompany/mycompany-wildcard-tls` will be used to present for the domains `mail.mycompany.com` and `www.mycompany.com` unless those IngressRoute objects have their own `spec.virtualhost.tls` stanza.

## Alternatives Considered

This is an alternative proposal to [TLS Certificate Delegation][1]

## Security Considerations

Subdomain delegation allows the administrator of a domain suffix, eg, `.mycompany.com` to define the TLS parameters covering the set of subdomains that a wildcard certificate will be applied too.

[0]: https://github.com/heptio/contour/blob/master/docs/ingressroute.md
[1]: https://github.com/heptio/contour/pull/889
