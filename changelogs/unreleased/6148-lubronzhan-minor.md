## Add anti-affinity rule for envoy deployed by provisioner

The envoy deployment created by the gateway provisioner now includes a default anti-affinity rule. The anti-affinity rule in the [example envoy deployment manifest](https://github.com/projectcontour/contour/blob/main/examples/deployment/03-envoy-deployment.yaml) is also updated to `preferredDuringSchedulingIgnoredDuringExecution` to be consistent with the contour deployment and the gateway provisioner anti-affinity rule.
