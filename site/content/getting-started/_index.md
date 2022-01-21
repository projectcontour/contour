---
title: Getting Started with Contour
description: Getting Started with Contour
id: getting-started
---

# Getting Started with Contour

This guide shows how to install Contour in three different ways:
- using Contour's example YAML
- using the Helm chart for Contour
- using the Contour operator (exerimental)

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
- 2 Contour pods with each status **Running** and 1/1 **Ready**  
- 1+ Envoy pod(s), with each the status **Running** and 2/2 **Ready**  

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


### Option 3: Contour Operator (experimental)

**NOTE**: If you are using a kind cluster, the Contour Operator requires a different kind configuration file than [what is described in the guide][30].
Recreate your kind cluster, following the guide but using the following Operator-compatible config file:
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
  extraPortMappings:
  - containerPort: 30080
    hostPort: 80
    listenAddress: "0.0.0.0"
  - containerPort: 30443
    hostPort: 443
    listenAddress: "0.0.0.0"
```

Install the Contour Operator & Contour CRDs:

```bash
$ kubectl apply -f https://projectcontour.io/quickstart/operator.yaml
```

Verify the Operator deployment is available:

```bash
$ kubectl get deploy -n contour-operator
NAME               READY   UP-TO-DATE   AVAILABLE   AGE
contour-operator   1/1     1            1           1m
```

Install an instance of the `Contour` custom resource:

```bash
# If using a kind cluster
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour-operator/{{< param latest_version >}}/examples/contour/contour-nodeport.yaml

# If using a cluster with support for services of type LoadBalancer
kubectl apply -f https://raw.githubusercontent.com/projectcontour/contour-operator/{{< param latest_version >}}/examples/contour/contour.yaml
```

Verify the `Contour` custom resource is available (it may take up to several minutes to become available):

```bash
$ kubectl get contour/contour-sample
NAME             READY   REASON
contour-sample   True    ContourAvailable
```

Verify the Contour pods are ready by running the following: 

```bash
$ kubectl get pods -n projectcontour -o wide
```

You should see the following:
- 2 Contour pods with each status **Running** and 1/1 **Ready**  
- 1+ Envoy pod(s), each with the status **Running** and 1/1 **Ready**  
 
## Test it out!

Congratulations, you have installed Contour and Envoy! Let's install a web application workload and get some traffic flowing to the backend.

TODO is there a different sample workload then?
Note: It is not recommended to expose kuard to the public.

To install kuard, run the following:

```bash
kubectl apply -f https://projectcontour.io/examples/kuard.yaml
```

Verify the pods and service are ready by running:

```bash
kubectl get po,svc,ing -l app=kuard
```  

You should see the following:
- 3 instances of pods/kuard, each with status **Running** and 1/1 **Ready**
- 1 service/kuard CLUSTER-IP listed on port 80
- 1 Ingress on port 80

**NOTE**: the Helm install configures Contour to filter Ingress and HTTPProxy objects based on the `contour` IngressClass name.
If using Helm, ensure the Ingress has an ingress class of `contour` with the following:

```bash
kubectl patch ingress kuard -p '{"spec":{"ingressClassName": "contour"}}'
```

Now we're ready to send some traffic to our sample application, via Contour & Envoy.

If you're using a local KinD cluster, the address you'll use is `127.0.0.1`.

If you're using a cluster with support for services of type `LoadBalancer`, you'll use the external IP address of the `envoy` service:

```bash
# If using YAML or the Operator
kubectl -n projectcontour get svc/envoy -o jsonpath='{.status.loadBalancer.ingress[0].ip}'

# If using Helm
kubectl -n projectcontour get svc/my-release-contour-envoy -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

Verify access to the sample application via Contour/Envoy by browsing to the above address. You can refresh multiple times to cycle through each pod replica.  

Congratulations, you have installed Contour, deployed a backend application, created an `Ingress` to route traffic to the application, and successfully accessed the app with Contour!

## Next Steps  
Now that you have a basic Contour installation, where to go from here?

- Explore [HTTPProxy][2], a cluster-wide reverse proxy
- Explore [contour-operator][14] (experimental) to manage multiple instances of contour
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
[9]: https://github.com/kubernetes-up-and-running/kuard
[10]: {{< relref "community.md" >}}
[11]: https://github.com/projectcontour/community/wiki/Office-Hours
[12]: {{< param slack_url >}}
[13]: https://projectcontour.io/resources/deprecation-policy/
[14]: https://github.com/projectcontour/contour-operator/blob/main/README.md
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
[28]: /guides/kind
[29]: https://helm.sh/docs/intro/install/
[30]: /guides/kind/#kind-configuration-file
