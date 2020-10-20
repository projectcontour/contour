# Envoy container not listening

Contour does not configure Envoy to listen on a port unless there is traffic to be served.
For example, if you have not configured any TLS Ingress objects then Contour does not command Envoy to open the secure listener (port 443 in the example deployment).
Because the HTTP and HTTPS listeners both use the same code, if you have no Ingress objects deployed in your cluster, or if no Ingress objects are permitted to talk on HTTP, then Envoy does not listen on the insecure port (port 80 in the example deploymen).

To test whether Contour is correctly deployed you can deploy the kuard example service:

```sh
$ kubectl apply -f https://projectcontour.io/examples/kuard.yaml
```
