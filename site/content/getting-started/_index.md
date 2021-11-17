
---
title: Getting Started with Contour
description: Getting Started with Contour
id: getting-started
---

This guide installs Contour with a sample workload in a test environment in two ways:  

- Quickstart: installs and demonstrates how Contour works with a workload application
- Quickstart using Helm: installs contour using the helm package manager  

The quickstart guide uses all default settings. No additional configuration is required.

# What is Contour?  

Contour is an open source Kubernetes ingress controller providing the control plane for the Envoy edge and service proxy.​ Contour supports dynamic configuration updates and multi-team ingress delegation out of the box while maintaining a lightweight profile.  

# Philosophy
- Follow an opinionated approach which allows us to better serve most users
- Design Contour to serve both the cluster administrator and the application developer
- Use our experience with ingress to define reasonable defaults for both cluster administrators and application developers.
- Meet users where they are by understanding and adapting Contour to their use cases  

See the full [Contour Philosophy][5] page.

# Why Contour?
Contour bridges other solution gaps in several ways:
- Dynamically update the ingress configuration with minimal dropped connections
- Safely support multiple types of ingress config in multi-team Kubernetes clusters
- Improve on core ingress configuration methods using our HTTPProxy custom resource
- Cleanly integrate with the Kubernetes object model  

# Quickstart
This quickstart guide installs Contour on a kubernetes cluster with a web application workload.
1. Set up a kubernetes environment
1. Install a Contour service
1. Install a kuard workload


## 1. Set up a kubernetes environment
This procedure uses Docker and kind to deploy a kubernetes cluster. If you already have a cluster available, skip to step 2  – Install a Contour service.  


### Install kind:

See the download and install instructions for kind [here][22].

Verify kind is installed by running:

```yaml
$ kind
```
You should see a list of kind commands.  

### Install Docker:  

You can find Docker installation information [here][23].  

Verify Docker is installed by running:

```yaml
$ docker
```
You should see a list of docker commands.

### Create a kind configuration file:  

Create yaml file on your local system to allow port 80 and 443. Copy the text below into the local yaml file **kind.config.yaml**.
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    listenAddress: "0.0.0.0"  
  - containerPort: 443
    hostPort: 443
    listenAddress: "0.0.0.0"
``` 
	    
### Create a Kubernetes cluster using kind:  

Create a cluster using kind.config.yaml file  

```yaml
$ kind create cluster --config=kind.config.yaml
```

Verify the nodes are ready by running:  

```yaml
$ kubectl get no
```  

You should see a 2 nodes listed with status **Ready**:  
- kind-control-plane
- kind-worker

Congratulations, you have created your cluster environment. You're ready to install Contour.  
 
## 2. Install Contour and Envoy
Run the following to install Contour:

```yaml
$ kubectl apply -f https://projectcontour.io/quickstart/contour.yaml
```

Verify the Contour pods are ready by running the following: 

```yaml
$ kubectl get pods -n projectcontour -o wide
```  
You should see the following:
- 2 Contour pods with each status **Running** and 1/1 **Ready**  
- 1 Envoy pod with the status **Running** and 1/1 **Ready**  

Congratulations, you have installed Contour and Envoy! Let's install a web application workload and get some traffic flowing to the backend.
 
## 3. Install a kuard workload (simple application)  
This section installs [kuard][9] to verify web traffic is flowing to the pods through envoy.

Note: It is not recommended to expose kuard to the public.

Install kuard by running the following:  

```yaml
$ kubectl apply -f https://projectcontour.io/examples/kuard.yaml
```  
Verify the pods and service are ready by running:
```yaml
$ kubectl get po,svc,ing -l app=kuard
```  
You should see the following:
- 3 instances of pods/kuard, each with status **Running** and 1/1 **Ready**
- 1 service/kuard CLUSTER-IP on port 80
- 1 ingress on port 80


Verify web access by browsing to [http://127.0.0.1.](http://127.0.0.1.) You can refresh multiple times to cycle through each pod workload.  
 
Congratulations, you have installed Contour with a backend web application on a kubernetes cluster! This installation has created the following:

- Namespace <code>projectcontour</code>
- Two instances of Contour in the namespace
- A Kubernetes Daemonset running Envoy on each node in the cluster listening on host ports 80/443
- A Service of <code>type: LoadBalancer</code> that points to the Contour’s Envoy instances

Note: When you are done with the cluster, you can delete it by running:  
```yaml
$ kind delete cluster
```  
---
# Quickstart using Helm
Prerequisites: 
- kubernetes cluster environment.  

See Quickstart (above) to install a kubernetes cluster using kind and Docker.  

This guide installs Contour using Helm and a simple web application workload.

1. Install Helm  
1. Add bitnami Helm repo  
1. Install Contour  
1. Install a kuard workload 

## 1. Install Helm  

You can find instructions to install Helm [here][24]. 
  
## 2. Add bitnami Helm repo  
Add the bitnami repository by running the following:  

```yaml  
$ helm repo add bitnami https://charts.bitnami.com/bitnami  
```
Note: you may need to run the following to update your repo:
``` yaml
helm repo update
```
## 3. Install Contour  
Install Contour by running the following:
```yaml 
$ helm install my-release bitnami/contour
```  
Verify Contour is ready by running:
```yaml
$ kubectl get po,svc

```  
You should see the following:
- 2 instances of pod/my-release-contour-contour
- 1 instance of pod/my-release-contour-envoy
- 1 instance of service/my-release-contour 
- 1 instance of service/my-release-contour-envoy

## 4. Install a kuard workload (simple application)  
Install kuard web application workload to have traffic flowing to the backend.

Note: It is not recommended to expose kuard to the public.

To install kuard, run the following:
```yaml
kubectl apply -f https://projectcontour.io/examples/kuard.yaml
```
Verify the pods and service are ready by running:
```yaml
$ kubectl get po,svc,ing -l app=kuard
```  
You should see the following:
- 3 instances of pods/kuard, each with status **Running** and 1/1 **Ready**
- 1 service/kuard CLUSTER-IP listed on port 80
- 1 Ingress on port 80

The Helm install configures Contour to filter Ingress and HTTPProxy objects based on the `contour` IngressClass name.
To ensure Contour reconciles the created Ingress, edit the `spec` to add an `ingressClassName` field as below:
```yaml
spec:
  ingressClassName: contour
```

Verify web access by browsing to [127.0.0.1](http://127.0.0.1). You can refresh multiple times to cycle through each pod workload.  

Congratulations, you have installed Contour with a backend web application workload using Helm .

Note: When you are done with the cluster, you can delete it by running:  
```yaml
$ kind delete cluster
```  
---
# Next Steps  
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
  
# Connect with the Team
Have questions? Send a Slack message on the Contour channel, an email on the mailing list, or join a Contour meeting.
- Slack: kubernetes.slack.com [#contour][12]
- Join us in a [User Group][10] or [Office Hours][11] meeting 
- Join the [mailing list][25] for the latest information


# Troubleshooting

If you encounter issues, review the [troubleshooting][17] page, [file an issue][6], or talk to us on the [#contour channel][12] on Kubernetes Slack.

[1]: /docs/{{< param latest_version >}}/deploy-options
[2]: /docs/{{< param latest_version >}}/config/fundamentals
[3]: /docs/{{< param latest_version >}}
[4]: {{< ref "resources/faq.md" >}}
[5]: {{< relref "resources/philosophy.md" >}}
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
[24]: https://helm.sh/docs/intro/install/
[25]: https://lists.cncf.io/g/cncf-contour-users/