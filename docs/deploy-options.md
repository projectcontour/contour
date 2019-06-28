# Deployment options

The [README][0] shows you a simple way to get started with Contour on your cluster. This topic explains the details and shows you additional options.  Most of this covers running Contour using a Kubernetes Service of `Type: LoadBalancer`. If you don't have a cluster with that capability or if you don't want to use it see the [Running without a Kubernetes LoadBalancer](#running-without-a-kubernetes-loadbalancer) section.

## Deployment or DaemonSet?

We provide example deployment manifests for setting up Contour by creating either a DaemonSet or a Deployment.

- The DaemonSet creates an instance of Contour that runs on each node in your cluster.
- The Deployment creates two instances of Contour that run on the cluster, on two arbitrary nodes.

In either case, a Service of `type: LoadBalancer` is set up to forward to the Contour instances.

## Install

- Clone or fork the repository.
- To install the DaemonSet, navigate to the `examples/ds-grpc-v2` directory. OR
- To install the Deployment, navigate to the `examples/deployment-grpc-v2`.

Then run:

```
kubectl apply -f .
```

Contour is now deployed. Depending on your cloud provider, it may take some time to configure the load balancer.

### Under the hood

Each directory contains four files:

* `01-common.yaml`: Creates the `heptio-contour` Namespace and a ServiceAccount.
* `02-rbac.yaml`: Creates the RBAC rules for Contour. The Contour RBAC permissions are the minimum required for Contour to operate.
* `02-contour.yaml`: Runs the Contour pods with either the DaemonSet or the Deployment. See [Architecture][1] for pod details.
* `02-service.yaml`: Creates the Service object so that Contour can be reached from outside the cluster.

## Get your hostname or IP address

To retrieve the IP address or DNS name assigned to your Contour deployment, run:

```
kubectl get -n heptio-contour service contour -o wide
```

On AWS, for example, the response looks like:

```
NAME      CLUSTER-IP     EXTERNAL-IP                                                                    PORT(S)        AGE       SELECTOR
contour   10.106.53.14   a47761ccbb9ce11e7b27f023b7e83d33-2036788482.ap-southeast-2.elb.amazonaws.com   80:30274/TCP   3h        app=contour
```

Depending on your cloud provider, the `EXTERNAL-IP` value is an IP address, or, in the case of Amazon AWS, the DNS name of the ELB created for Contour. Keep a record of this value.

Note that if you are running an Elastic Load Balancer (ELB) on AWS, you must add more details to your configuration to get the remote address of your incoming connections. See [Recovering the remote IP address with an ELB](tls.md/#recovering-the-remote-IP-address-with-an-ELB).

### Minikube

On Minikube, to get the IP address of the Contour service run:

```
minikube service -n heptio-contour contour --url
```

The response is always an IP address, for example `http://192.168.99.100:30588`. This is used as CONTOUR_IP in the rest of the documentation.

### kind
When creating the cluster on Kind, pass a custom configuration to allow Kind to expose port 8080 to your local host:

```yaml
kind: Cluster
apiVersion: kind.sigs.k8s.io/v1alpha3
nodes:
- role: control-plane
- role: worker
  extraPortMappings:
  - containerPort: 8080
    hostPort: 8080
    listenAddress: "0.0.0.0"
```

Then run the create cluster command passing the config file as a parameter.
This file is in the `examples/kind` directory:

```bash
$ kind create cluster --config examples/kind/kind-expose-port.yaml
```

Then, your CONTOUR_IP (as used below) will just be `localhost:8080`.

_Note: If you change Envoy's ports to bind to 80/443 then it's possible to add entried to your local `/etc/hosts` file and make requests like `http://kuard.local` which matches how it might work on a production installation._

## Test with Ingress

The Contour repository contains an example deployment of the Kubernetes Up and Running demo application, [kuard][2].
To test your Contour deployment, deploy `kuard` with the following command:

```
kubectl apply -f examples/example-workload/kuard.yaml
```

Then monitor the progress of the deployment with:

```
kubectl get po,svc,ing -l app=kuard
```

You should see something like:

```
NAME                       READY     STATUS    RESTARTS   AGE
po/kuard-370091993-ps2gf   1/1       Running   0          4m
po/kuard-370091993-r63cm   1/1       Running   0          4m
po/kuard-370091993-t4dqk   1/1       Running   0          4m

NAME        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE
svc/kuard   10.110.67.121   <none>        80/TCP    4m

NAME        HOSTS     ADDRESS     PORTS     AGE
ing/kuard   *         10.0.0.47   80        4m
```

... showing that there are three Pods, one Service, and one Ingress that is bound to all virtual hosts (`*`).

In your browser, navigate your browser to the IP or DNS address of the Contour Service to interact with the demo application.

## Test with IngressRoute

To test your Contour deployment with [IngressRoutes][4], run the following command:

```sh
kubectl apply -f examples/example-workload/kuard-ingressroute.yaml
```

Then monitor the progress of the deployment with:

```sh
kubectl get po,svc,ingressroute -l app=kuard
```

You should see something like:

```sh
NAME                        READY     STATUS    RESTARTS   AGE
pod/kuard-bcc7bf7df-9hj8d   1/1       Running   0          1h
pod/kuard-bcc7bf7df-bkbr5   1/1       Running   0          1h
pod/kuard-bcc7bf7df-vkbtl   1/1       Running   0          1h

NAME            TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)   AGE
service/kuard   ClusterIP   10.102.239.168   <none>        80/TCP    1h

NAME                                    CREATED AT
ingressroute.contour.heptio.com/kuard   1h
```

... showing that there are three Pods, one Service, and one IngressRoute.

In your terminal, use curl with the IP or DNS address of the Contour Service to send a request to the demo application:

```sh
curl -H 'Host: kuard.local' ${CONTOUR_IP}
```

## Running without a Kubernetes LoadBalancer

If you can't or don't want to use a Service of `type: LoadBalancer` there are two alternate ways to run Contour.

### Local development

As mentioned above, when you are doing local development, you can run a local cluster with [minikube](https://kubernetes.io/docs/setup/minikube/) or [kind](https://github.com/kubernetes-sigs/kind). Follow the instructions under [Get your hostname or IP address][5] to get access, then use your user agent of choice.

### NodePort Service

If your cluster doesn't have the capability to configure a Kubernetes LoadBalancer, or if you want to configure the load balancer outside Kubernetes, you can change the `02-service.yaml` file to set `type` to `NodePort`.  This will have every node in your cluster listen on the resultant port and forward traffic to Contour.  That port can be discovered by taking the second number listed in the `PORT` column when listing the service, for example `30274` in `80:30274/TCP`.

Now you can point your browser at the specified port on any node in your cluster to communicate with Contour.

### Host Networking

You can run Contour without a Kubernetes Service at all.
This is done by having the Contour pod run with host networking.
Do this with `hostNetwork: true` on your pod definition.
Envoy will listen directly on port 8080 on each host that it is running.
This is best paired with a DaemonSet (perhaps paired with Node affinity) to ensure that a single instance of Contour runs on each Node.
See the [AWS NLB tutorial][3] as an example.

## Running Contour in tandem with another ingress controller

If you're running multiple ingress controllers, or running on a cloudprovider that natively handles ingress, you can specify the annotation `kubernetes.io/ingress.class: "contour"` on all ingresses that you would like Contour to claim. You can customize the class name with the `--ingress-class-name` flag at runtime.
If the `kubernetes.io/ingress.class` annotation is present with a value other than `"contour"`, Contour will ignore that ingress.

## Uninstall Contour

To remove Contour from your cluster, delete the namespace:

```
% kubectl delete ns heptio-contour
```


[0]: ../README.md#get-started
[1]: architecture.md
[2]: https://github.com/kubernetes-up-and-running/kuard
[3]: deploy-aws-nlb.md
[4]: ingressroute.md
[5]: #get-your-hostname-or-ip-address