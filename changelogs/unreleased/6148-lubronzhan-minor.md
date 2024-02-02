## Add anti-affinity rule for envoy deployed by provisioner

Now by default, the envoy deployment created by gateway provistioner will include an default anti-affinity rule. Also the anti-affinity rule in [default envoy deployment manifest](../../examples/deployment/03-envoy-deployment.yaml) is updated to `preferredDuringSchedulingIgnoredDuringExecution` to be consistent with contour deployment.
