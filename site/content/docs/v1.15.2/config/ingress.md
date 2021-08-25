# k8s Ingress Resource Support in Contour

<!-- TODO: uncomment once we finish enabling Ingress conformance in CI -->
<!-- As of Contour version 1.X, Contour is validated to be conformant to the Ingress spec using the upstream [Ingress controller conformance tests][0]. -->
<!-- However, outside of those tests, the Ingress spec can be interpreted differently by various Ingress controller implementations. -->

This document describes Contour's implementation of specific Ingress resource fields and features.
As the Ingress specification has evolved between v1beta1 and v1, any differences between versions are highlighted to ensure clarity for Contour users.

## Kubernetes Versions

Contour is [validated against Kubernetes release versions N through N-2][1] (with N being the latest release).
As Ingress v1 was introduced in Kubernetes version 1.18, Contour running in an older Kubernetes version will watch Ingress v1beta1 objects and reconcile them.
For Kubernetes version 1.18+, the API serve translates any Ingress v1beta1 resources to Ingress v1 and Contour watches Ingress v1 resources.

## IngressClass and IngressClass Name

In order to support differentiating between Ingress controllers or multiple instances of a single Ingress controller, users can create an [IngressClass resource][2] and specify an IngressClass name on a Ingress to reference it.
The IngressClass resource can be used to provide configuration to an Ingress controller watching resources it governs.
Contour supports watching an IngressClass resource specified with the `--ingress-class-name` flag to the `contour serve` command.
Contour does not require an IngressClass resource with the name passed in the aforementioned flag to exist, the name can just be used as an identifier for filtering which Ingress resources Contour reconciles into actual route configuration.

Ingresses may specify an IngressClass name via the original annotation method or via the `ingressClassName` spec field.
As the `ingressClassName` field has been introduced on Ingress v1beta1, there should be no differences in IngressClass name filtering between the two available versions of the resource.
Contour uses its configured IngressClass name to filter Ingresses.
If the `--ingress-class-name` flag is provided, Contour will only accept Ingress resources that exactly match the specified IngressClass name via annotation or spec field, with the value in the annotation taking precedence.
If the flag is not passed to `contour serve` Contour will accept any Ingress resource that specifies the IngressClass name `contour` in annotation or spec fields or does not specify one at all.

## Default Backend

Contour supports the `defaultBackend` Ingress v1 spec field and equivalent `backend` v1beta1 version of the field.
See upstream [documentation][3] on this field.
Any requests that do not match an Ingress rule will be forwarded to this backend.
As TLS secrets on Ingresses are scoped to specific hosts, this default backend cannot serve TLS as it could match an unbounded set of hosts and configuring a matching set of TLS secrets would not be possible.
As is the case on Ingress rules, Contour only supports configuring a Service as a backend and does not support any other Kubernetes resource.

## Ingress Rules

See upstream [documentation][4] on Ingress rules.

As with default backends, Contour only supports configuring a Service as a backend and does not support any other Kubernetes resource.

Contour supports [wildcard hostnames][5] as documented by the upstream API as well as precise hostnames.
Wildcard hostnames are limited to the whole first DNS label of the hostname, e.g. `*.foo.com` is valid but `*foo.com`, `foo*.com`, `foo.*.com` are not.
`*` is also not a valid hostname.
The Ingress admission controller validation ensures valid hostnames are present when creating an Ingress resource.

Contour supports all of the various [path matching][6] types described by the Ingress spec.
Prior to Contour 1.14.0, path match types were ignored and path matching was performed with a Contour specific implementation.
Paths specified with any regex meta-characters (any of `^+*[]%`) were implemented as regex matches.
Any other paths were programmed in Envoy as "string prefix" matches.
This behavior is preserved in the `ImplementationSpecific` match type in Contour 1.14.0+ to ensure backwards compatibility.
`Exact` path matches will now result in matching requests to the given path exactly.
The `Prefix` patch match type will now result in matching requests with a "segment prefix" rather than a "string prefix" according to the spec (e.g. the prefix `/foo/bar` will match requests with paths `/foo/bar`, `/foo/bar/`, and `/foo/bar/baz`, but not `/foo/barbaz`).

## TLS

See upstream [documentation][7] on TLS configuration.

A secret specified in an Ingress TLS element will only be applied to Ingress rules with `Host` configuration that exactly matches an element of the TLS `Hosts` field. 
Any secrets that do not match an Ingress rule `Host` will be ignored.

Ingress v1 does not allow the `secretName` field to contain a string with a full `namespace/name` identifier.
This is a major change from Ingress v1beta1 and causes secrets referenced by v1 resources to be in the same namespace as the Ingress resource.
This also disables Contour's [TLS secret delegation][8] behavior across namespaces in Ingress v1.

## Status

In order to inform users of the address the Services their Ingress resources can be accessed at, Contour sets status on Ingress resources.
If `contour serve` is run with the `--ingress-status-address` flag, Contour will use the provided value to set the Ingress status address accordingly.
If not provided, Contour will use the address of the Envoy service using the passed in `--envoy-service-name` and `--envoy-service-namespace` flags.

[0]: https://github.com/kubernetes-sigs/ingress-controller-conformance
[1]: /resources/compatibility-matrix/
[2]: https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-class
[3]: https://kubernetes.io/docs/concepts/services-networking/ingress/#default-backend
[4]: https://kubernetes.io/docs/concepts/services-networking/ingress/#ingress-rules
[5]: https://kubernetes.io/docs/concepts/services-networking/ingress/#hostname-wildcards
[6]: https://kubernetes.io/docs/concepts/services-networking/ingress/#path-types
[7]: https://kubernetes.io/docs/concepts/services-networking/ingress/#tls
[8]: /docs/{{< param version >}}/config/tls-delegation/
