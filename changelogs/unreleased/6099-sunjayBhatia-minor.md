## Remove Contour container readiness probe initial delay

The Contour Deployment Contour server container previously had its readiness probe `initialDelaySeconds` field set to 15.
This has been removed from the example YAML manifests and Gateway Provisioner generated Contour Deployment since as of [PR #5672](https://github.com/projectcontour/contour/pull/5672) Contour's xDS server will not start or serve any configuration (and the readiness probe will not succeed) until the existing state of the cluster is synced.
In clusters with few resources this will improve the Contour Deployment's update/rollout time as initial startup time should be low.
