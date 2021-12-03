### Transition to controller-runtime managed leader election

Contour now utilizes [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) Manager based leader election and coordination of subroutines.
With this change, Contour is also transitioning away from using a ConfigMap for leader election.
In this release, Contour now uses a combination of ConfigMap and Lease object.
A future release will remove usage of the ConfigMap resource for leader election.

This change should be a no-op for most users, however be sure to re-apply the relevant parts of your deployment for RBAC to ensure Contour has access to Lease and Event objects (this would be the ClusterRole in the provided example YAML).
