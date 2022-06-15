## Gateway API: ReferencePolicy is deprecated, will be removed next release

Gateway API has renamed ReferencePolicy to ReferenceGrant in the v0.5.0 release, while retaining the former for one release to ease migration.
Contour currently supports both, but will drop support for ReferencePolicy in the next release.
Users of ReferencePolicies must migrate their resources to ReferenceGrants ahead of the next Contour release.
