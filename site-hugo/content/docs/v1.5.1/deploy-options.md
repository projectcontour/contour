# Deployment Options

The [Getting Started][8] guide shows you a simple way to get started with Contour on your cluster.
This topic explains the details and shows you additional options.
Most of this covers running Contour using a Kubernetes Service of `Type: LoadBalancer`.
If you don't have a cluster with that capability see the [Running without a Kubernetes LoadBalancer][1] section.

## Installation

### Recommended installation details

The recommended installation of Contour is Contour running in a Deployment and Envoy in a Daemonset with TLS securing the gRPC communication between them.
The [`contour` example][2] will install this for you.
A Service of `type: LoadBalancer` is also set up to forward traffic to the Envoy instances.

If you wish to use Host Networking, please see the [appropriate section][3] for the details.

## Testing your installation

### Get your hostname or IP address

To retrieve the IP address or DNS name assigned to your Contour deployment, run:

```bash
$ kubectl get -n projectcontour service envoy -o wide
```

On AWS, for example, the response looks like:

```
NAME      CLUSTER-IP     EXTERNAL-IP                                                                    PORT(S)        AGE       SELECTOR
contour   10.106.53.14   a47761ccbb9ce11e7b27f023b7e83d33-2036788482.ap-southeast-2.elb.amazonaws.com   80:30274/TCP   3h        app=contour
```

Depending on your cloud provider, the `EXTERNAL-IP` value is an IP address, or, in the case of Amazon AWS, the DNS name of the ELB created for Contour. Keep a record of this value.

Note that if you are running an Elastic Load Balancer (ELB) on AWS, you must add more details to your configuration to get the remote address of your incoming connections.
See the [instructions for enabling the PROXY protocol.][9].

#### Minikube

On Minikube, to get the IP address of the Contour service run:

```bash
$ minikube service -n projectcontour envoy --url
```

The response is always an IP address, for example `http://192.168.99.100:30588`. This is used as CONTOUR_IP in the rest of the documentation.

#### kind

When creating the cluster on Kind, pass a custom configuration to allow Kind to expose port 80/443 to your local host:

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

Then run the create cluster command passing the config file as a parameter.
This file is in the `examples/kind` directory:

```bash
$ kind create cluster --config examples/kind/kind-expose-port.yaml
```

Then, your CONTOUR_IP (as used below) will just be `localhost:80`.

_Note: We've created a public DNS record (`local.projectcontour.io`) which is configured to resolve to `127.0.0.1``. This allows you to use a real domain name in your kind cluster._

### Test with Ingress

The Contour repository contains an example deployment of the Kubernetes Up and Running demo application, [kuard][5].
To test your Contour deployment, deploy `kuard` with the following command:

```bash
$ kubectl apply -f https://projectcontour.io/examples/kuard.yaml
```

Then monitor the progress of the deployment with:

```bash
$ kubectl get po,svc,ing -l app=kuard
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

### Test with IngressRoute

To test your Contour deployment with [IngressRoutes][6], run the following command:

```sh
$ kubectl apply -f https://projectcontour.io/examples/kuard-ingressroute.yaml
```

Then monitor the progress of the deployment with:

```sh
$ kubectl get po,svc,ingressroute -l app=kuard
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
$ curl -H 'Host: kuard.local' ${CONTOUR_IP}
```
### Test with HTTPProxy

To test your Contour deployment with [HTTPProxy][9], run the following command:

```sh
$ kubectl apply -f https://projectcontour.io/examples/kuard-httpproxy.yaml
```

Then monitor the progress of the deployment with:

```sh
$ kubectl get po,svc,httpproxy -l app=kuard
```

You should see something like:

```sh
NAME                        READY     STATUS    RESTARTS   AGE
pod/kuard-bcc7bf7df-9hj8d   1/1       Running   0          1h
pod/kuard-bcc7bf7df-bkbr5   1/1       Running   0          1h
pod/kuard-bcc7bf7df-vkbtl   1/1       Running   0          1h

NAME            TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)   AGE
service/kuard   ClusterIP   10.102.239.168   <none>        80/TCP    1h

NAME                                    FQDN                TLS SECRET                  FIRST ROUTE  STATUS  STATUS DESCRIPT
httpproxy.projectcontour.io/kuard      kuard.local         <SECRET NAME IF TLS USED>                valid   valid HTTPProxy
```

... showing that there are three Pods, one Service, and one HTTPProxy .

In your terminal, use curl with the IP or DNS address of the Contour Service to send a request to the demo application:

```sh
$ curl -H 'Host: kuard.local' ${CONTOUR_IP}
```

## Running without a Kubernetes LoadBalancer

If you can't or don't want to use a Service of `type: LoadBalancer` there are other ways to run Contour.

### NodePort Service

If your cluster doesn't have the capability to configure a Kubernetes LoadBalancer,
or if you want to configure the load balancer outside Kubernetes,
you can change the Envoy Service in the [`02-service-envoy.yaml`][7] file and set `type` to `NodePort`.

This will have every node in your cluster listen on the resultant port and forward traffic to Contour.
That port can be discovered by taking the second number listed in the `PORT` column when listing the service, for example `30274` in `80:30274/TCP`.

Now you can point your browser at the specified port on any node in your cluster to communicate with Contour.

### Host Networking

You can run Contour without a Kubernetes Service at all.
This is done by having the Envoy pod run with host networking.
Contour's examples utilize this model in the `/examples` directory.
To configure, set: `hostNetwork: true` and `dnsPolicy: ClusterFirstWithHostNet` on your Envoy pod definition.
Next, pass `--envoy-service-http-port=80 --envoy-service-https-port=443` to the contour `serve` command which instructs Envoy to listen directly on port 80/443 on each host that it is running.
This is best paired with a DaemonSet (perhaps paired with Node affinity) to ensure that a single instance of Contour runs on each Node.
See the [AWS NLB tutorial][10] as an example.

### Upgrading Contour/Envoy

At times it's needed to upgrade Contour, the version of Envoy, or both.
The included `shutdown-manager` can assist with watching Envoy for open connections while draining and give signal back to Kubernetes as to when it's fine to delete Envoy pods during this process.

See the [redeploy envoy][11] docs for more information.

## Running Contour in tandem with another ingress controller

If you're running multiple ingress controllers, or running on a cloudprovider that natively handles ingress,
you can specify the annotation `kubernetes.io/ingress.class: "contour"` on all ingresses that you would like Contour to claim.
You can customize the class name with the `--ingress-class-name` flag at runtime.
If the `kubernetes.io/ingress.class` annotation is present with a value other than `"contour"`, Contour will ignore that ingress.

## Uninstall Contour

To remove Contour from your cluster, delete the namespace:

```bash
$ kubectl delete ns projectcontour
```

[1]: #running-without-a-kubernetes-loadbalancer
[2]: {{site.github.repository_url}}/tree/{{page.version}}/examples/contour/README.md
[3]: #host-networking
[4]: {% link _guides/proxy-proto.md %}
[5]: https://github.com/kubernetes-up-and-running/kuard
[6]: /docs/{{page.version}}/ingressroute
[7]: {{site.github.repository_url}}/tree/{{page.version}}/examples/contour/02-service-envoy.yaml
[8]: {% link getting-started.md %}
[9]: httpproxy.md
[10]: {% link _guides/deploy-aws-nlb.md %}
[11]: redeploy-envoy.md
