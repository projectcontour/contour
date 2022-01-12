### Add Envoy Deployment Example

The examples now include a way to deploy Envoy as a Deployment vs a Daemonset.
This can assist in allowing Envoy to drain connections cleanly when the Kubernetes cluster size is scaled down.