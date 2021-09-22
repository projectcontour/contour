We are delighted to present version v1.18.0 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

A big thank you to everyone who contributed to the release.

# Major Changes

## ExternalName Services changes

Kubernetes Services of type `ExternalName` will now not be processed by Contour without active action by Contour's operator. This prevents a vulnerability where an attacker with access to create Service objects inside a cluster running Contour could force Contour's Envoy to expose its own admin interface to the internet with a Service with an ExternalName of `localhost` or similar.

With access to Envoy's admin page, an attacker can force Envoy to restart or drain connections (which will cause Kubernetes to restart it eventually). The attacker can also see the names and metadata of TLS Secrets, *but not their contents*.

This version includes two main changes:
- ExternalName Service processing is now disabled by default, and must be enabled.
- Even when processing is enabled, obvious attempts to expose `localhost` will be prevented. This is quite a porous control, and should not be relied on as your sole means of mitigation.

In short, we recommend migration away from ExternalName Services as soon as you can.

### I currently use ExternalName Services, what do I need to do?

If you are currently using ExternalName Services with your Contour installation, it's still recommended that you update to this version.

However, as part of updating, you will need to add the `enableExternalNameService: "true"` directive to your Contour configuration file. This is not recommended for general use, because of the concerns above, but we have but some mitigations in place to stop *obvious* misuse of ExternalName Services if you *must* have them enabled.

Note that because of the cross-namespace control issues documented at kubernetes/kubernetes#103675, you should definitely consider if you can move away from using ExternalName Services in any production clusters.

The ability to enable ExternalName Services for Contour is provided to help with migration, it is *not recommended* for use in production clusters.

## Gateway API Improvements

### GatewayClass and Gateway deletion

Contour 1.18.0 ensures when a GatewayClass or Gateway is deleted, it is removed from Contour's cache and DAG.
Routes programmed in Envoy are properly removed as well.

Relevant PRs:
- [#3923](https://github.com/projectcontour/contour/pull/3923) : internal/controller: fix Gateway/GatewayClass deletion bug

### GatewayClass with non-nil Spec.ParametersRef handling

Previously Contour would not admit GatewayClasses with a non-nil Spec.ParametersRef.
This release ensures these GatewayClasses are still reconciled (Spec.ParametersRef is ignored).

Relevant PRs:
- [#3876](https://github.com/projectcontour/contour/pull/3876) : internal/validation: allow GatewayClass.Spec.ParametersRef to be specified

## Secrets used for upstream TLS validation can be delegated across namespaces

Previously Contour would not allow a secret referenced in HTTPProxy upstream validation to reside in a different namespace from the HTTPProxy object itself.
This release includes a change to allow users to utilize TLS certificate delegation to relax this restriction.
Users will now be able to create a TLSCertificateDelegation object to allow the owner of the CA certificate secret to delegate, for the purposes of referencing the CA certificate in a different namespace, permission to Contour to read the Secret object from another namespace.
You may want to take advantage of this feature to consolidate secrets in a management namespace and delegate their usage to an app namespace containing the relevant HTTPProxy.

Thanks to @shashankram for implementing this feature!

Relevant PRs:
- [#3894](https://github.com/projectcontour/contour/pull/3894) : internal/dag: allow delegation of upstream validation CACertificate

## IngressClassName field added to HTTPProxy

To ensure forwards-compatibility with k8s standards around Ingress, Contour has added the Spec.IngressClassName field, to mimic the similar Ingress object field.
HTTPProxy objects could previously be filtered via [annotation](https://projectcontour.io/docs/v1.18.0/config/annotations/#ingress-class) but now will also be filtered using the same rules as Ingress objects, taking into account the Spec field.

Relevant PRs:
- [#3902](https://github.com/projectcontour/contour/pull/3902) : HTTPProxy: add Spec.IngressClassName

## Access Logging Enhancements

The Envoy text access log format string can now be customized via the `accesslog-format-string` field in the Contour config file.
In addition, the `REQ_WITHOUT_QUERY` access logging extension is enabled in Contour, which allows logging of a request path with any query string stripped.
*Note* that using this command operator extension requires Envoy 1.19.0.
See [this documentation page](https://projectcontour.io/docs/v1.18.0/config/access-logging/) for more details.

Thanks to @tsaarni for contributing this feature and documentation!

Relevant PRs:
- [#3694](https://github.com/projectcontour/contour/pull/3694) : internal/envoy: configurable access log format
- [#3849](https://github.com/projectcontour/contour/pull/3849) : site: Added versioned access logging document
- [#3921](https://github.com/projectcontour/contour/pull/3921) : site: update access logging links

## Envoy Updated to 1.19.0

Contour 1.18.0 is compatible with Envoy 1.19.0. For more information, see the Contour compatibility matrix.

Relevant PRs:
- [#3887](https://github.com/projectcontour/contour/pull/3887) : update Envoy to 1.19.0

## Documentation Working Group Contributions

Thanks to @gary-tai for helping clean up the project site.

Relevant PRs:
- [#3913](https://github.com/projectcontour/contour/pull/3913) : Kindly running post - fix broken links
- [#3919](https://github.com/projectcontour/contour/pull/3919) : Blog updates - fix 2 blog posts for broken image links

# Upgrading

Please consult the [upgrade documentation](https://projectcontour.io/resources/upgrading/).

## Compatible Kubernetes Versions

Contour v1.18.0 is tested against Kubernetes 1.19 through 1.21

# Community Thanks!

We’re immensely grateful for all the community contributions that help make Contour even better! For this release, special thanks go out to the following contributors:
- @tsaarni
- @shashankram
- @gary-tai

# Are you a Contour user? We would love to know!

If you're using Contour and want to add your organization to our adopters list, please visit this [page](https://github.com/projectcontour/contour/blob/master/ADOPTERS.md). If you prefer to keep your organization name anonymous but still give us feedback into your usage and scenarios for Contour, please post on this [GitHub thread](https://github.com/projectcontour/contour/issues/1269).
