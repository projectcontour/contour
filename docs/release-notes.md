**note: do not commit this file, copy and paste the text into the release page**

VMware is enraptured to present version 1.0.0-rc.2 of Contour, our layer 7 HTTP reverse proxy for Kuberentes clusters. As always, without the help of the many community contributors this release would not have been possible. Thank you!

Contour 1.0.0-rc.2 is the second, and hopefully last, release candidate on the path to Contour 1.0.

The current stable release at this time remains Contour 0.15.2.

## New and improved 

Contour 1.0.0-rc.2 contains many bug fixes and improvements over rc.1.

### Website improvements

As part of the continued preparations for the 1.0 release Contour's documentation has been relocated to the https://projectcontour.io website. Specifically;

* The Getting Started documentation has moved to [projectcontour.io/getting-started](https://projectcontour.io/getting-started/)
* Guides and How-to's have moved to [projectcontour.io/guides](https://projectcontour.io/guides)
* Versioned release documetaion has moved to [projectcontour.io/docs](https://projectcontour.io/docs)
* Project related and non-versioned documentation has moved to [projectcontour.io/resources](https://projectcontour.io/resources/)  

### IngressRoute and HTTPProxy status update improvements

IngressRoute and HTTPProxy status updates are now performed by the lead Contour in the deployment. The lead Contour is determined via Kubernetes' standard leader election mechanisms.

If leader election is disabled, all Contours will write status back to the Kubernetes API.

Fixes #1425, #1385, and many more issues with status loops over the years.

### HTTPProxy and IngressRoute OpenAPIv3 schema validation

Contour 1.0.0-rc.2 includes updates the the OpenAPIv3 schema validations. These schemas are automatically generated from the CRD documents themselves and should be more complete and consistent than the previous hand rolled versions.

Fixes #513, #1414. Thanks @youngnick

### TCPProxy delegation

Contour 1.0.0-rc.2 now supports TCPProxy delegation. See the [relevant section](https://projectcontour.io/docs/1.0/httpproxy) in the HTTPProxy documentation.

Fixes #1655.

### Envoy keepalive tuning

Contour 1.0.0-rc.2 addresses an issue where connections between Contour and Envoy could become stuck half-open (one side thinks the connection is open, the other side doesn't) or half-closed (one side closes the connection, the other side never gets the message).

The common theme was the cluster was using an overlay network which suggested the overlay was timing out long running TCP connections. Contour 1.0.0-rc.2 configures various keep alive mechanisms to detect networking issues between Envoy and Contour. 

This fix is also included in Contour 0.15.3 and later.

Fixes #1744. Thanks @youngnick, @bgagnon, and @ravilr.

### Minor improvements

- The ability to write the bootstrap configuration to standard out via `contour bootstrap -- -` has been added. Thanks @jpeach.
- Contour now validates that TLS certificates either bare the type `kubernetes.io/tls` or, in the case of upstream validation certificates, contain a non empty `ca.crt` key. Fixes #1697. Thanks @jpeach.
- `x_trace_id` has been added to the set of JSON loggable fields. Fixes #1734. Thanks @cw-sakamoto!
- Obsolute Heptio branding has been removed from `contour cli`. Thanks @jpeach.
- Contour is built with Go 1.13.3.

## Bug fixes

### TLS certificate validation improvements

Contour 1.0.0-rc.2 improves the TLS certificate validation added in rc.1. Contour is now less likely to reject valid certificates that contain unexpected elliptic curve parameters.

This fix is also included in Contour 0.15.2 and later.

Fixes #1702. With many thanks to @mattalberts for the report and the fix.

### Minor bug fixes

- _Many_ documentation updates and improvements. Thanks @stevesloka, @youngnick, @jpeach.
- Ingress, IngressRoute, and HTTPProxy route conditions are now properly ordered. Fixes #1579. Thanks @jpeach.

## Upgrading

Please consult the [Upgrading](/docs/upgrading.md) document for further information on upgrading from Contour 1.0.0-rc.1 to Contour 1.0.0-rc.2.
