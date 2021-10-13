Today we present version v1.17.1 of Contour, our layer 7 HTTP reverse proxy for Kubernetes clusters.

This is a security release, which fixes the advisory at [GHSA-5ph6-qq5x-7jwc](https://github.com/projectcontour/contour/security/advisories/GHSA-5ph6-qq5x-7jwc).

Thanks to @josh-ferrell for the initial report.

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