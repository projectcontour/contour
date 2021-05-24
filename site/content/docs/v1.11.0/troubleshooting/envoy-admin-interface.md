# Accessing the Envoy Administration Interface

Getting access to the Envoy [administration interface][1] can be useful for diagnosing issues with routing or cluster health.

The Envoy administration interface is bound by default to `http://127.0.0.1:9001`.
To access it from your workstation use `kubectl port-forward` like so:

```sh
# Get one of the pods that matches the Envoy daemonset
ENVOY_POD=$(kubectl -n projectcontour get pod -l app=envoy -o name | head -1)
# Do the port forward to that pod
kubectl -n projectcontour port-forward $ENVOY_POD 9001
```

Then navigate to `http://127.0.0.1:9001/` to access the administration interface for the Envoy container running on that pod.

[1]: https://www.envoyproxy.io/docs/envoy/latest/operations/admin
