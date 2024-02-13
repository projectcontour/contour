---
title: Getting Started with Contour
description: Getting Started with Contour
id: getting-started
---

# Getting Started with Contour

This guide shows how to install Contour in three different ways:
- using Contour's example YAML
- using the Helm chart for Contour
- using the Contour gateway provisioner

It then shows how to deploy a sample workload and route traffic to it via Contour.

This guide uses all default settings. No additional configuration is required.

## Validate Kubernetes environment

This guide is designed to work with:

- a Kubernetes cluster with support for services of type `LoadBalancer` (GKE, AKS, EKS, etc.); or
- a locally-running [kind cluster][27] with port mappings configured

If you already have access to one of these Kubernetes environments, you're ready to move on to installing Contour.
If not, you can [set up a local kind cluster][28] for testing purposes.

## Install Contour and Envoy

### Option 1: YAML
Run the following to install Contour:

```bash
$ kubectl apply -f https://projectcontour.io/quickstart/contour.yaml
```

Verify the Contour pods are ready by running the following:

```bash
$ kubectl get pods -n projectcontour -o wide
```

You should see the following:
- 2 Contour pods each with status **Running** and 1/1 **Ready**
- 1+ Envoy pod(s), each with the status **Running** and 2/2 **Ready**

### Option 2: Helm
This option requires [Helm to be installed locally][29].

Add the bitnami chart repository (which contains the Contour chart) by running the following:

```bash
$ helm repo add bitnami https://charts.bitnami.com/bitnami
```

Install the Contour chart by running the following:

```bash
$ helm install my-release bitnami/contour --namespace projectcontour --create-namespace
```

Verify Contour is ready by running:

```bash
$ kubectl -n projectcontour get po,svc
```

You should see the following:
- 1 instance of pod/my-release-contour-contour with status **Running** and 1/1 **Ready**
- 1+ instance(s) of pod/my-release-contour-envoy with each status **Running** and 2/2 **Ready**
- 1 instance of service/my-release-contour
- 1 instance of service/my-release-contour-envoy


### Option 3: Contour Gateway Provisioner

The Gateway provisioner watches for the creation of [Gateway API][31] `Gateway` resources, and dynamically provisions Contour+Envoy instances based on the `Gateway's` spec.
Note that although the provisioning request itself is made via a Gateway API resource (`Gateway`), this method of installation still allows you to use *any* of the supported APIs for defining virtual hosts and routes: `Ingress`, `HTTPProxy`, or Gateway API's `HTTPRoute` and `TLSRoute`.
In fact, this guide will use an `Ingress` resource to define routing rules, even when using the Gateway provisioner for installation.


Deploy the Gateway provisioner:
```bash
$ kubectl apply -f https://projectcontour.io/quickstart/contour-gateway-provisioner.yaml
```

Verify the Gateway provisioner deployment is available:

```bash
$ kubectl -n projectcontour get deployments
NAME                          READY   UP-TO-DATE   AVAILABLE   AGE
contour-gateway-provisioner   1/1     1            1           1m
```

Create a GatewayClass:

```shell
kubectl apply -f - <<EOF
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1
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
apiVersion: gateway.networking.k8s.io/v1
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

Verify the `Gateway` is available (it may take up to a minute to become available):

```bash
$ kubectl -n projectcontour get gateways
NAME        CLASS     ADDRESS         READY   AGE
contour     contour                   True    27s
```

Verify the Contour pods are ready by running the following:

```bash
$ kubectl -n projectcontour get pods
```

You should see the following:
- 2 Contour pods each with status **Running** and 1/1 **Ready**
- 1+ Envoy pod(s), each with the status **Running** and 2/2 **Ready**

## Test it out!

Congratulations, you have installed Contour and Envoy! Let's install a web application workload and get some traffic flowing to the backend.

To install [httpbin][9], run the following:

```bash
kubectl apply -f https://projectcontour.io/examples/httpbin.yaml
```

Verify the pods and service are ready by running:

```bash
kubectl get po,svc,ing -l app=httpbin
```

You should see the following:
- 3 instances of pods/httpbin, each with status **Running** and 1/1 **Ready**
- 1 service/httpbin CLUSTER-IP listed on port 80
- 1 Ingress on port 80

**NOTE**: the Helm install configures Contour to filter Ingress and HTTPProxy objects based on the `contour` IngressClass name.
If using Helm, ensure the Ingress has an ingress class of `contour` with the following:

```bash
kubectl patch ingress httpbin -p '{"spec":{"ingressClassName": "contour"}}'
```

Now we're ready to send some traffic to our sample application, via Contour & Envoy.

_Note, for simplicity and compatibility across all platforms we'll use `kubectl port-forward` to get traffic to Envoy, but in a production environment you would typically use the Envoy service's address._

Port-forward from your local machine to the Envoy service:
```shell
# If using YAML
$ kubectl -n projectcontour port-forward service/envoy 8888:80

# If using Helm
$ kubectl -n projectcontour port-forward service/my-release-contour-envoy 8888:80

# If using the Gateway provisioner
$ kubectl -n projectcontour port-forward service/envoy-contour 8888:80
```

In a browser or via `curl`, make a request to http://local.projectcontour.io:8888 (note, `local.projectcontour.io` is a public DNS record resolving to 127.0.0.1 to make use of the forwarded port).
You should see the `httpbin` home page.

Congratulations, you have installed Contour, deployed a backend application, created an `Ingress` to route traffic to the application, and successfully accessed the app with Contour!

## Next Steps
Now that you have a basic Contour installation, where to go from here?

- Explore [HTTPProxy][2], a cluster-wide reverse proxy
- Explore the [Gateway API documentation][32] and [Gateway API guide][14]
- Explore other [deployment options][1]

Check out the following demo videos:
- [Contour 101 - Kubernetes Ingress and Blue/Green Deployments][20]
- [HTTPProxy in Action][19]
- [Contour Demos and Deep Dives videos][21]

Explore the documentation:
- [FAQ][4]
- [Contour Architecture][18]
- [Contour Configuration Reference][7]

## Connect with the Team
Have questions? Send a Slack message on the Contour channel, an email on the mailing list, or join a Contour meeting.
- Slack: kubernetes.slack.com [#contour][12]
- Join us in a [User Group][10] or [Office Hours][11] meeting
- Join the [mailing list][25] for the latest information

## Troubleshooting

If you encounter issues, review the [troubleshooting][17] page, [file an issue][6], or talk to us on the [#contour channel][12] on Kubernetes Slack.

[1]: /docs/{{< param latest_version >}}/deploy-options
[2]: /docs/{{< param latest_version >}}/config/fundamentals
[3]: /docs/{{< param latest_version >}}
[4]: {{< ref "resources/faq.md" >}}
[6]: {{< param github_url >}}/issues
[7]: /docs/{{< param latest_version >}}/configuration/
[9]: https://httpbin.org/
[10]: {{< relref "community.md" >}}
[11]: https://github.com/projectcontour/community/wiki/Office-Hours
[12]: {{< param slack_url >}}
[13]: https://projectcontour.io/resources/deprecation-policy/
[14]: /docs/{{< param latest_version >}}/guides/gateway-api
[15]: https://github.com/bitnami/charts/tree/master/bitnami/contour
[16]: https://github.com/helm/charts#%EF%B8%8F-deprecation-and-archive-notice
[17]: /docs/{{< param latest_version >}}/troubleshooting
[18]: /docs/{{< param latest_version >}}/architecture
[19]: https://youtu.be/YA82A4Rcs_A
[20]: https://www.youtube.com/watch?v=xUJbTnN3Dmw
[21]: https://www.youtube.com/playlist?list=PL7bmigfV0EqRTmmjwWm4SxuCZwNvze7se
[22]: https://kind.sigs.k8s.io/docs/user/quick-start/
[23]: https://docs.docker.com/desktop/#download-and-install
[25]: https://lists.cncf.io/g/cncf-contour-users/
[26]: https://www.envoyproxy.io/
[27]: https://kind.sigs.k8s.io/
[28]: /docs/{{< param latest_version >}}/guides/kind
[29]: https://helm.sh/docs/intro/install/
[30]: /docs/{{< param latest_version >}}/guides/kind/#kind-configuration-file
[31]: https://gateway-api.sigs.k8s.io/
[32]: /docs/{{< param latest_version >}}/config/gateway-api