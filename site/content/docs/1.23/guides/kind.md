---
title: Creating a Contour-compatible kind cluster
---

This guide walks through creating a kind (Kubernetes in Docker) cluster on your local machine that can be used for developing and testing Contour.

# Prerequisites

Download & install Docker and kind:

- Docker [installation information](https://docs.docker.com/desktop/#download-and-install)  
- kind [download and install instructions](https://kind.sigs.k8s.io/docs/user/quick-start/)

# Kind configuration file  

Create a kind configuration file locally.
This file will instruct kind to create a cluster with one control plane node and one worker node, and to map ports 80 and 443 on your local machine to ports 80 and 443 on the worker node container.
This will allow us to easily get traffic to Contour/Envoy running inside the kind cluster from our local machine.

Copy the text below into the local yaml file `kind-config.yaml`:

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
	    
# Kubernetes cluster using kind  

Create a kind cluster using the config file from above:

```yaml
$ kind create cluster --config kind-config.yaml
```

Verify the nodes are ready by running:  

```yaml
$ kubectl get nodes
```  

You should see 2 nodes listed with status **Ready**:  
- kind-control-plane
- kind-worker

Congratulations, you have created your cluster environment. You're ready to install Contour.  

_Note:_ When you are done with the cluster, you can delete it by running:
```yaml
$ kind delete cluster
```

# Next Steps
See https://projectcontour.io/getting-started/ for how to install Contour into your kind cluster.
