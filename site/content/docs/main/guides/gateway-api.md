---
title: Using Gateway API with Contour
---

## Introduction

[Gateway API][1] is an open source project managed by the Kubernetes SIG-NETWORK community. The project's goal is to
evolve service networking APIs within the Kubernetes ecosystem. Gateway API consists of multiple resources that provide
user interfaces to expose Kubernetes applications- Services, Ingress, and more.

This guide covers using version **v1beta1** of the Gateway API, with Contour `v1.22.0` or higher.

### Background

Gateway API targets three personas:

- __Platform Provider__: The Platform Provider is responsible for the overall environment that the cluster runs in, i.e.
  the cloud provider. The Platform Provider will interact with `GatewayClass` resources.
- __Platform Operator__: The Platform Operator is responsible for overall cluster administration. They manage policies,
  network access, application permissions and will interact with `Gateway` resources.
- __Service Operator__: The Service Operator is responsible for defining application configuration and service
  composition. They will interact with `HTTPRoute` and `TLSRoute` resources and other typical Kubernetes resources.

Gateway API contains three primary resources:

- __GatewayClass__: Defines a set of gateways with a common configuration and behavior.
- __Gateway__: Requests a point where traffic can be translated to a Service within the cluster.
- __HTTPRoute/TLSRoute__: Describes how traffic coming via the Gateway maps to the Services.

Resources are meant to align with personas. For example, a platform operator will create a `Gateway`, so a developer can
expose an HTTP application using an `HTTPRoute` resource.

### Prerequisites
The following prerequisites must be met before using Gateway API with Contour:

- A working [Kubernetes][2] cluster. Refer to the [compatibility matrix][3] for cluster version requirements.
- The [kubectl][4] command-line tool, installed and configured to access your cluster.

## Deploying Contour with Gateway API

Contour supports two modes of provisioning for use with Gateway API: **static** and **dynamic**.

In **static** provisioning, the platform operator defines a `Gateway` resource, and then manually deploys a Contour instance corresponding to that `Gateway` resource.
It is up to the platform operator to ensure that all configuration matches between the `Gateway` and the Contour/Envoy resources.
With static provisioning, Contour can be configured with either a [controller name][8], or a specific gateway (see the [API documentation][7].)
If configured with a controller name, Contour will process the oldest `GatewayClass`, its oldest `Gateway`, and that `Gateway's` routes, for the given controller name.
If configured with a specific gateway, Contour will process that `Gateway` and its routes.

In **dynamic** provisioning, the platform operator first deploys Contour's Gateway provisioner. Then, the platform operator defines a `Gateway` resource, and the provisioner automatically deploys a Contour instance that corresponds to the `Gateway's` configuration and will process that `Gateway` and its routes.

Static provisioning may be more appropriate for users who prefer the traditional model of deploying Contour, have just a single Contour instance, or have highly customized YAML for deploying Contour.
Dynamic provisioning may be more appropriate for users who want a simple declarative API for provisioning Contour instances.

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

Update the Contour config map to enable Gateway API processing by specifying a gateway controller name, and restart Contour to pick up the config change:

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

See the next section ([Testing the Gateway API](#testing-the-gateway-api)) for how to deploy an application and route traffic to it using Gateway API!

## Testing the Gateway API

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

Test access to the kuard application:

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

## Next Steps

### Customizing your dynamically provisioned Contour instances

In the dynamic provisioning example, we used a default set of options for provisioning the Contour gateway.
However, Gateway API also [supports attaching parameters to a GatewayClass][5], which can customize the Gateways that are provisioned for that GatewayClass.

Contour defines a CRD called `ContourDeployment`, which can be used as `GatewayClass` parameters.

A simple example of a parameterized Contour GatewayClass that provisions Envoy as a Deployment instead of the default DaemonSet looks like:

```yaml
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour-with-envoy-deployment
spec:
  controllerName: projectcontour.io/gateway-controller
  parametersRef:
    kind: ContourDeployment
    group: projectcontour.io
    name: contour-with-envoy-deployment-params
    namespace: projectcontour
---
kind: ContourDeployment
apiVersion: projectcontour.io/v1alpha1
metadata:
  namespace: projectcontour
  name: contour-with-envoy-deployment-params
spec:
  envoy:
    workloadType: Deployment
```

All Gateways provisioned using the `contour-with-envoy-deployment` GatewayClass would get an Envoy Deployment.

See [the API documentation][6] for all `ContourDeployment` options.

### Further reading

This guide only scratches the surface of the Gateway API's capabilities. See the [Gateway API website][1] for more information.


[1]: https://gateway-api.sigs.k8s.io/
[2]: https://kubernetes.io/
[3]: https://projectcontour.io/resources/compatibility-matrix/
[4]: https://kubernetes.io/docs/tasks/tools/install-kubectl/
[5]: https://gateway-api.sigs.k8s.io/api-types/gatewayclass/#gatewayclass-parameters
[6]: https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1alpha1.ContourDeployment
[7]: https://projectcontour.io/docs/main/config/api/#projectcontour.io/v1alpha1.GatewayConfig
[8]: https://gateway-api.sigs.k8s.io/api-types/gatewayclass/#gatewayclass-controller-selection
