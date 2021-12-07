## Contour leader election resource RBAC moved to namespaced Role

Previously, in our example deployment YAML, RBAC for Contour access to resources used for leader election was contained in a ClusterRole, meaning that Contour required cluster-wide access to ConfigMap resources.
This release also requires Contour access to Events and Leases which would require cluster-wide access (see [this PR](https://github.com/projectcontour/contour/pull/4202)).

In this release, we have moved the RBAC rules for leader election resources to a namespaced Role in the example Contour deployment.
This change should limit Contour's default required access footprint.
A corresponding namespaced RoleBinding has been added as well.

### Required actions

If you are using the example deployment YAML to deploy Contour, be sure to examine and re-apply the resources in `examples/contour/02-rbac.yaml` and `examples/contour/02-role-contour.yaml`.
If you have deployed Contour in a namespace other than the example `projectcontour`, be sure to modify the `contour` Role and `contour-rolebinding` RoleBinding resources accordingly.
Similarly, if you are using the `--leader-election-resource-namespace` flag to customize where Contour's leader election resources reside, you must customize the new Role and RoleBinding accordingly.
