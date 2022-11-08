# ContourDeployment.Spec.Contour.Replicas and ContourDeployment.Spec.Envoy.Replicas are deprecated

- `ContourDeployment.Spec.Contour.Replicas` is deprecated and has been replaced by `ContourDeployment.Spec.Contour.Deployment.Replicas`. Users should switch to using the new field. The deprecated field will be removed in a future release. See #4713 for additional details.

- `ContourDeployment.Spec.Envoy.Replicas` is deprecated and has been replaced by `ContourDeployment.Spec.Envoy.Deployment.Replicas`. Users should switch to using the new field. The deprecated field will be removed in a future release. See #4713 for additional details.
