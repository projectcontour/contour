# Troubleshooting

This document contains suggestions for debugging issues with your Contour installation.

## Envoy container not listening on port 8080 or 8443

Contour will not configure Envoy to listen on a port unless there is traffic to be served.
For example if you have not configured any TLS ingress objects then Contour will not command Envoy to open port 8443 (443 in the service object).
As both the HTTP and HTTPS listeners use the same code, if you have no ingress objects deployed in your cluster, or non are permitted to talk on HTTP then you will find that Envoy is not listening on port 8080 (80 in the service object).

To test Contour is correctly deployed you can deploy the KUARD example service
```
$ kubectl apply -f https://j.hept.io/contour-kuard-example
```

## Accessing the Envoy admin interface

By default the Envoy admin interface is bound to localhost.
To access it remotely you can pass set the flag `contour bootstrap --admin-address=0.0.0.0 /config/contour.yaml` in your deployment/daemonset's `envoy-initconfig` init container.
This will bind Envoy's admin interface to the Pod IP.
(You can also pass `--admin-port=....` if you wish to change the port the admin interface is bound to).

If you wish to access the admin interface via kubeproxy, add the following stanza to your `contour` service object.
```
    - name: admin
      port: 9001
      protocol: TCP
```
Then, after starting `kubectl proxy`, you can access the admin interface at this address

http://127.0.0.1:8001/api/v1/namespaces/heptio-contour/services/contour:9001/proxy/

**Warning** If you are using a service loadbalancer, this may cause your cloud provider to make the admin port accessible on your external load balancer IP.
**The admin interface has no authentication and may expose sensitive information**.

## I can't make kube-lego work with Contour

If you use [kube-lego][0] for Let's Encrypt SSL certificates there is a limitation that kube-lego appears to set the ingress class on the ingress record it uses for the acme-01 challenge to `nginx`.
This will cause Contour to ignore the record, and the challenge will fail.

At the moment the workaround is to manually edit the ingress object created by kube-lego to remove the `kubernetes.io/ingress.class` annotation, or set its value to `contour`.

[0]: https://github.com/jetstack/kube-lego
