# Interrogate Contour's xDS Resources

Sometimes it's helpful to be able to interrogate Contour to find out exactly what [xDS][1] resource data it is sending to Envoy.
Contour ships with a `contour cli` subcommand which can be used for this purpose.

Because Contour secures its communications with Envoy using Secrets in the cluster, the easiest way is to run `contour cli` commands _inside_ the pod.
Do this is via `kubectl exec`:

```bash
# Get one of the pods that matches the examples/daemonset
$ CONTOUR_POD=$(kubectl -n projectcontour get pod -l app=contour -o jsonpath='{.items[0].metadata.name}')
# Do the port forward to that pod
$ kubectl -n projectcontour exec $CONTOUR_POD -c contour -- contour cli lds --cafile=/certs/ca.crt --cert-file=/certs/tls.crt --key-file=/certs/tls.key
```

Which will stream changes to the LDS api endpoint to your terminal.
Replace `contour cli lds` with `contour cli rds` for route resources, `contour cli cds` for cluster resources, and `contour cli eds` for endpoints.

[1]: https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol
