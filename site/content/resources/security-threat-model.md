---
title: Contour Threat Model and Security Posture
layout: page
---

Contour is an ingress controller that works as an Envoy control plane, configuring the Envoy data plane, which actually carries traffic from outside to inside the cluster.

## Inputs

Contour's inputs are:
- configuration and command line flags set by its administrator
- Kubernetes objects that represent the desired load balancer/data plane configuration.

## Expected Users

The expected users of of Contour are:
- the Contour owner/administrator - installs and configures Contour
- application developers exposing their in-cluster apps - create ingress configuration objects in Kubernetes referencing their Deployments and Services
- external application consumers - their traffic passes through the Envoy data plane on the way to the hosted apps.

## Attack surface and mitigations

As you can see from the above, Contour does not have a web interface of any sort, and never directly participates in requests that transit the data plane, so the only way it is vulnerable to web attacks is via misconfiguring Envoy. As such, it is not directly susceptible to common web application security risks like the OWASP top ten. (*Envoy* is, but not Contour directly, and we rely on thye Envoy project's vigilance heavily.)

We anticpate that the most likely attacks are created by the relatively untrusted application developer users, whether they are malicious or not. We expect the most likely attacks to be:
- Confused deputy attacks - since Contour is trusted to build config and send to Envoy, that access can be misused to produce insecure Envoy configurations. [ExternalName Services can be used to gain access to Envoy's admin interface](https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc) was an example of this attack in action, and was specifically dealt with by disallowing ExternalName services by default, and by removing the Envoy admin interface from use across any network, even localhost.
- Insecure or conflicting configurations produced my manipulation of Kubernetes objects used for configuration.

 Our general method of mitigating both of these styles of attack is to be proscriptive about what configurations Contour will accept. Obviously, in cases like the ExternalName issue above, it's possible for a syntatically and allowed configuration to produce an insecure Envoy config, and this is therefore a primary focus of our thread model.
 
 In terms of the other users of Contour:
 - the external application consumers have no access to Contour, and relatively small access to Envoy. However, any vulnerabilities in Envoy itself also apply to Contour's installation of Envoy, so the Contour team issues Contour patch releases each time an Envoy security release is issued, in which we update the expected version of Envoy in our example deployment and make the community aware of the fix for the vulnerability.
 - the Contour/owner administrator: In general, we expect the operator of Contour to have full control over the installation, and so assume positive intent on their part, because a malicious administrator is not possible to deal with in this scope. What mitigations we place around Contour's per-installation configuration is geared towards preventing accidental insecure configuration rather than deliberate malfeasance, alongside preventing Kubernetes privilege escalation by specifying the minimum Kubernetes access required by each component.  

The last remaining areas to discuss are:
- Bounds checking and input validation: The Kubernetes apiserver is very good at bounds checking and input validation for fields in Kubernetes objects, so we delegate a lot of that there, and assume generally that the values for any specific object are unlikely to be malicious in themselves. In short, we don't need to worry too much about length checking for fields in objects, removing invalid characters, etc, as the apiserver does that for us.
- RBAC and privilege escalation prevention: We can't control what owners of Contour do in their own clusters, but we do provide an example installation that codifies what we believe to be best practice in terms of Kubernetes privilege limitations. We provide limited roles that only grant access to the things required for the component (Contour or Envoy) to run, and ensure that the deployments use those roles. In addition, we've attempted to ensure that both the Contour and Envoy containers can safely be run as nonroot.
- Static checking and code quality: We maintain a CI pipeline that runs golangci-lint including the usual set of Go static security checking, and enforce PR review for all merges to our main branch. Substantial changes are also subject to design review via a formal design document process.

## Reporting Security issues
For reporting security issues, please see the [reporting process documentation][1].
 

[1]: /resources/security-process