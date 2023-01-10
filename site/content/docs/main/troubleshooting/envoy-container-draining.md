# Envoy container stuck in unready/draining state

It's possible for the Envoy containers to become stuck in an unready/draining state.
This is an unintended side effect of the shutdown-manager sidecar container being restarted by the kubelet.
For more details on exactly how this happens, see [this issue][1].

If you observe Envoy containers in this state, you should `kubectl delete` them to allow new Pods to be created to replace them.

To make this issue less likely to occur, you should:
- ensure you have [resource requests][2] on all your containers
- ensure you do **not** have a liveness probe on the shutdown-manager sidecar container in the envoy daemonset (this was removed from the example YAML in Contour 1.24.0).

If the above are not sufficient for preventing the issue, you may also add a liveness probe to the envoy container itself, like the following:

```yaml
livenessProbe:
  httpGet:
    path: /ready
    port: 8002
  initialDelaySeconds: 15
  periodSeconds: 5
  failureThreshold: 6
```

This will cause the kubelet to restart the envoy container if it does get stuck in this state, resulting in a return to normal operations load balancing traffic.
Note that in this case, it's possible that a graceful drain of connections may or may not occur, depending on the exact sequence of operations that preceded the envoy container failing the liveness probe.

[1]: https://github.com/projectcontour/contour/issues/4851
[2]: /docs/{{< param latest_version >}}/deploy-options/#setting-resource-requests-and-limits