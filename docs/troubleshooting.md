# Troubleshooting

This document contains suggestions for debugging issues with your Contour installation.

## Envoy container not listening on port 8080 or 8443

Contour does not configure Envoy to listen on a port unless there is traffic to be served.
For example, if you have not configured any TLS ingress objects then Contour does not command Envoy to open port 8443 (443 in the service object).
Because the HTTP and HTTPS listeners both use the same code, if you have no ingress objects deployed in your cluster, or if no ingress objects are permitted to talk on HTTP, then Envoy does not listen on port 8080 (80 in the service object).

To test whether Contour is correctly deployed you can deploy the kuard example service:
```
$ kubectl apply -f https://j.hept.io/contour-kuard-example
```

## Access the Envoy admin interface remotely

By default the Envoy admin interface is bound to localhost.
To access it remotely you can pass the flag `contour bootstrap --admin-address=0.0.0.0 /config/contour.yaml` in the `envoy-initconfig` init container for your Deployment or DaemonSet.
This binds Envoy's admin interface to the Pod IP.
(You can also pass `--admin-port=....` to change the port the admin interface is bound to).

If you wish to access the admin interface with kubeproxy, add the following stanza to your `contour` service object.
```
    - name: admin
      port: 9001
      protocol: TCP
```
After you start `kubectl proxy`, you can access the admin interface at this address:

http://127.0.0.1:8001/api/v1/namespaces/heptio-contour/services/contour:9001/proxy/

**Warning** If you are using a service load balancer, this may cause your cloud provider to make the admin port accessible on your external load balancer IP.
**The admin interface has no authentication and may expose sensitive information**.

## Can't make kube-lego work with Contour

If you use [kube-lego][0] for Let's Encrypt SSL certificates, kube-lego appears to set the ingress class on the ingress record it uses for the acme-01 challenge to `nginx`.
This setting causes Contour to ignore the record, and the challenge fails.

The current workaround is to manually edit the ingress object created by kube-lego to remove the `kubernetes.io/ingress.class` annotation. Or you can set the value of the annotation to `contour`.

[0]: https://github.com/jetstack/kube-lego
