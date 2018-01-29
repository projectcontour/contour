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

Getting access to the Envoy admin interface can be useful for diagnosing issues with routing or cluster health.

### Enabling the admin interface

To access Envoy's admin interface add the flag `--admin-address=0.0.0.0` in the `envoy-initconfig` init container for your Deployment or DaemonSet. 
This binds Envoy's admin interface to the Pod IP on port 9001.
If you are using Contour 0.4 or later, these options are the default.

### Deploying the debug service.

Deploy the [admin interface service][1], which looks like this:
```
apiVersion: v1
kind: Service
metadata:
 name: contour-admin
 namespace: heptio-contour
spec:
 ports:
 - port: 9001
   name: admin
   protocol: TCP
 selector:
   app: contour
 type: ClusterIP
```
This will create a new service called `admin` on port `9001` bound to Contour's cluster IP. 

_Note_: It is not advisable to make the admin interface accessible to the internet by editing the `contour` service.
**The admin interface has no authentication and may expose sensitive information**.
Always deploy a new service for port 9001.

### Access the interface via kubectl proxy.

If your cluster IP is not routable from your workstation (which is unlikely), the easiest way to access the admin service is via `kubectl proxy`.
Start `kubectl proxy` in a terminal session:
```
% kubectl proxy
Starting to serve on 127.0.0.1:8001
```
Once `kubectl proxy` is running, you can access the admin interface at this address:

http://127.0.0.1:8001/api/v1/namespaces/heptio-contour/services/contour-admin:9001/proxy/

## Can't make kube-lego work with Contour

If you use [kube-lego][0] for Let's Encrypt SSL certificates, kube-lego appears to set the ingress class on the ingress record it uses for the acme-01 challenge to `nginx`.
This setting causes Contour to ignore the record, and the challenge fails.

The current workaround is to manually edit the ingress object created by kube-lego to remove the `kubernetes.io/ingress.class` annotation. Or you can set the value of the annotation to `contour`.

[0]: https://github.com/jetstack/kube-lego
[1]: ../deployment/debug/debug-service.yaml
