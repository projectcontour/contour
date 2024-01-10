---
title: Using Gateway API with Contour
---

This tutorial walks through an example of using [Gateway API][1] with Contour.
See the [Contour reference documentation][5] for more information on Contour's Gateway API support.

### Prerequisites
The following prerequisites must be met before following this guide:

- A working [Kubernetes][2] cluster. Refer to the [compatibility matrix][3] for cluster version requirements.
- The [kubectl][4] command-line tool, installed and configured to access your cluster.

## Deploy Contour with Gateway API enabled

First, deploy Contour with Gateway API enabled.
This can be done using either [static or dynamic provisioning][6].

### Option #1: Statically provisioned

Create Gateway API CRDs:
```shell
$ kubectl apply -f {{< param github_raw_url>}}/{{< param branch >}}/examples/gateway/00-crds.yaml
```

Create a GatewayClass:
```shell
kubectl apply -f - <<EOF
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
spec:
  controllerName: projectcontour.io/gateway-controller
EOF
```

Create a Gateway in the `projectcontour` namespace:
```shell
kubectl apply -f - <<EOF
kind: Namespace
apiVersion: v1
metadata:
  name: projectcontour
---
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
  namespace: projectcontour
spec:
  gatewayClassName: contour
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
EOF
```

Deploy Contour:
```shell
$ kubectl apply -f {{< param base_url >}}/quickstart/contour.yaml
```
This command creates:

- Namespace `projectcontour` to run Contour
- Contour CRDs
- Contour RBAC resources
- Contour Deployment / Service
- Envoy DaemonSet / Service
- Contour ConfigMap

Update the Contour configmap to enable Gateway API processing by specifying a gateway controller name, and restart Contour to pick up the config change:

```shell
kubectl apply -f - <<EOF
kind: ConfigMap
apiVersion: v1
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    gateway:
      controllerName: projectcontour.io/gateway-controller
EOF

kubectl -n projectcontour rollout restart deployment/contour
```

See the next section ([Testing the Gateway API](#testing-the-gateway-api)) for how to deploy an application and route traffic to it using Gateway API!

### Option #2: Dynamically provisioned

Deploy the Gateway provisioner:
```shell
$ kubectl apply -f {{< param base_url >}}/quickstart/contour-gateway-provisioner.yaml
```

This command creates:

- Namespace `projectcontour` to run the Gateway provisioner
- Contour CRDs
- Gateway API CRDs
- Gateway provisioner RBAC resources
- Gateway provisioner Deployment

Create a GatewayClass:

```shell
kubectl apply -f - <<EOF
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
spec:
  controllerName: projectcontour.io/gateway-controller
EOF
```

Create a Gateway:

```shell
kubectl apply -f - <<EOF
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
  namespace: projectcontour
spec:
  gatewayClassName: contour
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
EOF
```

The above creates:
- A `GatewayClass` named `contour` controlled by the Gateway provisioner (via the `projectcontour.io/gateway-controller` string)
- A `Gateway` resource named `contour` in the `projectcontour` namespace, using the `contour` GatewayClass
- Contour and Envoy resources in the `projectcontour` namespace to implement the `Gateway`, i.e. a Contour deployment, an Envoy daemonset, an Envoy service, etc.

See the next section ([Testing the Gateway API](#test-routing)) for how to deploy an application and route traffic to it using Gateway API!

## Configure an HTTPRoute

Deploy the test application:
```shell
$ kubectl apply -f {{< param github_raw_url>}}/{{< param branch >}}/examples/example-workload/gatewayapi/kuard/kuard.yaml
```
This command creates:

- A Deployment named `kuard` in the default namespace to run kuard as the test application.
- A Service named `kuard` in the default namespace to expose the kuard application on TCP port 80.
- An HTTPRoute named `kuard` in the default namespace, attached to the `contour` Gateway, to route requests for `local.projectcontour.io` to the kuard service.

Verify the kuard resources are available:
```shell
$ kubectl get po,svc,httproute -l app=kuard
NAME                         READY   STATUS    RESTARTS   AGE
pod/kuard-798585497b-78x6x   1/1     Running   0          21s
pod/kuard-798585497b-7gktg   1/1     Running   0          21s
pod/kuard-798585497b-zw42m   1/1     Running   0          21s

NAME            TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)   AGE
service/kuard   ClusterIP   172.30.168.168   <none>        80/TCP    21s

NAME                                        HOSTNAMES
httproute.gateway.networking.k8s.io/kuard   ["local.projectcontour.io"]
```

## Test Routing

_Note, for simplicity and compatibility across all platforms we'll use `kubectl port-forward` to get traffic to Envoy, but in a production environment you would typically use the Envoy service's address._

Port-forward from your local machine to the Envoy service:
```shell
# If using static provisioning
$ kubectl -n projectcontour port-forward service/envoy 8888:80

# If using dynamic provisioning
$ kubectl -n projectcontour port-forward service/envoy-contour 8888:80
```

In another terminal, make a request to the application via the forwarded port (note, `local.projectcontour.io` is a public DNS record resolving to 127.0.0.1 to make use of the forwarded port):
```shell
$ curl -i http://local.projectcontour.io:8888
```
You should receive a 200 response code along with the HTML body of the main `kuard` page.

You can also open http://local.projectcontour.io:8888/ in a browser.

### Further reading

This guide only scratches the surface of the Gateway API's capabilities. See the [Gateway API website][1] for more information.


[1]: https://gateway-api.sigs.k8s.io/
[2]: https://kubernetes.io/
[3]: https://projectcontour.io/resources/compatibility-matrix/
[4]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[5]: /docs/{{< param version >}}/config/gateway-api
[6]: /docs/{{< param version >}}/config/gateway-api#enabling-gateway-api-in-contour