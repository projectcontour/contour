# How-To: Deploy web applications on Kubernetes with Heptio Contour and Let's Encrypt

This is a technical how-to that walks you through deploying a web application securely on a Kubernetes Cluster using HTTPS.
The technologies we're going to use in this how-to are:

- Kubernetes (obviously :))
- Contour as the ingress controller
- JetStack's cert-manager to provision TLS certificates from the Let's Encrypt project

## Prerequisites

Before you begin you'll need to have a few pieces already in place:

- A Kubernetes cluster deployed either in a data centre or using a cloud provider with a Kubernetes as a service offering. For this tutorial we're on a GKE cluster running Kubernetes 1.8.
- RBAC enabled on your cluster.
- Your cluster must be able to request a public IP address from your cloud provider, using a load balancer. If you're on AWS or GKE this is automatic if you deploy a Kubernetes service object of type: LoadBalancer. If you're on your own datacenter you must set it up yourself.
- A DNS domain that you control, where you will host your web application.
- Administrator permissions for all deployment steps

_NOTE:_ Using a minikube cluster is out of scope because of the complexities of NAT and port forwarding.

## Ready

This tutorial walks you through deploying:

- [Heptio Contour][0]
- [Jetstack cert-manager][1]
- Your web application

_NOTE:_ If you encounter failures related to permissions, make sure the user you are operating as has administrator permissions.

After you've been through the steps the first time, you don't need to repeat deploying Contour and cert-manager for subsequent application deployments, you can skip to step three.

## 1. Deploy Contour

First, run:
```
kubectl apply -f https://j.hept.io/contour-deployment-rbac
```
This sets up Contour as a deployment in its own namespace, heptio-contour, and tells the cloud provider to provision an external IP that is forwarded to the Contour pods.

You can check the progress of the deployment with this command:
```
% kubectl -n heptio-contour get po
NAME                      READY     STATUS    RESTARTS   AGE
contour-f9f68994f-kzjdz   2/2       Running   0          6d
contour-f9f68994f-t7h8n   2/2       Running   0          6d
```
After all the `contour` pods reach `Running` status, move on to the next step.

### Access your cluster
Now you need to retrieve the external address of the load balancer assigned to Contour by your cloud provider. Run this command:
```
% kubectl get -n heptio-contour service contour -o wide
NAME      TYPE           CLUSTER-IP     EXTERNAL-IP    PORT(S)                      AGE       SELECTOR
contour   LoadBalancer   10.51.245.99   35.189.26.87   80:30111/TCP,443:30933/TCP   38d       app=contour
```
The value of `EXTERNAL-IP` varies by cloud provider. In this example GKE gives a bare IP address; AWS gives you a long DNS name.

To make it easier to work with the external load balancer I added a DNS record to a domain I control that points to this load balancer's IP address:
```
% host gke.davecheney.com
gke.davecheney.com has address 35.189.26.87
```
If you're using AWS, the you would use a `CNAME`, not an `A` record, and it would look something like this:
```
% host aws.davecheney.com
aws.davecheney.com is an alias for a4d1766f6ce1611e7b27f023b7e83d33–1465548734.ap-southeast-2.elb.amazonaws.com.
a4d1766f6ce1611e7b27f023b7e83d33–1465548734.ap-southeast-2.elb.amazonaws.com has address 52.63.20.117
a4d1766f6ce1611e7b27f023b7e83d33–1465548734.ap-southeast-2.elb.amazonaws.com has address 52.64.233.204
```
If you're using your own data centre, you need to arrange for traffic from a public IP to be forwarded to the cluster IP of the Contour service. This is beyond the scope of this tutorial.

### Testing connectivity

Next you need to deploy at least one Ingress object before Contour can serve traffic. Note that as a security feature, Contour does not expose a port to the internet unless there's a reason it should. A great way to test your Contour installation is to deploy the Kubernetes Up And Running demonstration application (KUARD).

To deploy KUARD to your cluster, run this command:
```
kubectl apply -f https://j.hept.io/contour-kuard-example
```
Check that the pod is running:
```
% kubectl get po -l app=kuard
NAME                       READY     STATUS    RESTARTS   AGE
kuard-67ff6dd458-sfxkb     1/1       Running   0          19d
```
Then type the DNS name you set up in the previous step into a web browser, for example http://gke.davecheney.com/. You should see something like:
![KAURD screenshot][kuard]
You can delete the KUARD service now, or at any time, by running:
```
kubectl delete -f https://j.hept.io/contour-kuard-example
```
## 2. Deploy jetstack/cert-manager

Let's Encrypt provides an automated service to request short lived SSL certificates (around 90 days).
In this tutorial we deploy JetStack's cert-manager, which manages requesting and renewing certificates for us.

_NOTE:_ cert-manager is a powerful product and supports a lot more functionality that we're going to use today.
The example in this tutorial is an opinionated deployment using Let's Encrypt as the certificate issuer.
There are plenty of other ways to deploy cert-manager, but they are out of scope.

### Fetch the source manager deployment manifest
To keep things simple, we skip cert-manager's Helm installation, and use the supplied YAML manifests.
Clone the github.com/jetstack/cert-manager repo:
```
git clone https://github.com/jetstack/cert-manager
cd cert-manager
```
Then deploy cert-manager with this command:
```
kubectl -n cert-manager apply -f docs/deploy/rbac/
```
When cert-manager is up and running you should see something like:
```
% kubectl -n cert-manager get all
NAME                  DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deploy/cert-manager   1         1         1            1           1h
NAME                         DESIRED   CURRENT   READY     AGE
rs/cert-manager-5886f49b8c   1         1         1         1h
NAME                  DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deploy/cert-manager   1         1         1            1           1h
NAME                         DESIRED   CURRENT   READY     AGE
rs/cert-manager-5886f49b8c   1         1         1         1h
NAME                               READY     STATUS    RESTARTS   AGE
po/cert-manager-5886f49b8c-49tq8   2/2       Running   0          1h
```

### Deploy the Let's Encrypt Cluster Issuer
With cert-manager running, next you need to configure it.
cert-manager supports two different CRDs for configuration, an `Issuer`, which is scoped to a single namespace, and a `ClusterIssuer`, which is cluster-wide.
Because we want Contour to be able to serve HTTPS traffic for an Ingress in any namespace, we use the latter, ClusterIssuer.
Create a file called `letsencrypt-staging.yaml` with the following contents:
```
apiVersion: certmanager.k8s.io/v1alpha1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
  namespace: cert-manager
spec:
  acme:
    email: user@example.com
    http01: {}
    privateKeySecretRef:
      name: letsencrypt-staging
    server: https://acme-staging.api.letsencrypt.org/directory
```
replacing `user@example.com` with your email address.
This is the email address that Let's Encrypt uses to communicate with you about certificates you have requested.

We use the staging Let's Encrypt server initially because the production server has strict API limits and quickly bans any IP address and the domains that are being registered that exceeds those limits.
If you're interested you can [read about Let's Encrypt's limits here][2].

After we get everything set up, we'll come back and configure a production Let's Encrypt certificate issuer.

After you edit and save the file, deploy it:
```
% kubectl apply -f letsencrypt-staging.yaml
clusterissuer "letsencrypt-staging" created
```
You should see several lines in the output of `kubectl -n cert-manager logs -l app=cert-manager -c cert-manager` informing you that the `ClusterIssuer` is properly registered:
```
I0220 02:32:50.614141 1 controller.go:138] clusterissuers controller: syncing item 'letsencrypt-staging'
I0220 02:32:52.552107 1 helpers.go:122] Setting lastTransitionTime for ClusterIssuer "letsencrypt-staging" condition "Ready" to 2018–02–20 02:32:52.552092474 +0000 UTC m=+10215.147984505
I0220 02:32:52.560665 1 controller.go:152] clusterissuers controller: Finished processing work item "letsencrypt-staging"
```
Now you're ready to deploy a Kubernetes service and serve traffic for it securely over HTTPS.

## 3. Deploy your first HTTPS site
For this tutorial we deploy a version of Kenneth Reitz's [httpbin.org service][3].
We start with the deployment.
Copy the following to a file called `deployment.yaml`:
```
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    app: httpbin
  name: httpbin
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin
  strategy:
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 1
    type: RollingUpdate
  template:
    metadata:
      labels:
        app: httpbin
    spec:
      containers:
      - image: docker.io/kennethreitz/httpbin
        name: httpbin
        ports:
        - containerPort: 8080
          name: http
        command: ["gunicorn"]
        args: ["-b", "0.0.0.0:8080", "httpbin:app"]
      dnsPolicy: ClusterFirst
      restartPolicy: Always
      schedulerName: default-scheduler
      securityContext: {}
      terminationGracePeriodSeconds: 30
```
Then deploy to your cluster:
```
% kubectl apply -f deployment.yaml 
deployment "httpbin" created
% kubectl get po -l app=httpbin
NAME                       READY     STATUS    RESTARTS   AGE
httpbin-67fd96d97c-8j2rr   1/1       Running   0          56m
```
Now it's time to expose the deployment to the world.
We do this with a Service:
```
apiVersion: v1
kind: Service
metadata:
  name: httpbin
spec:
  ports:
  - port: 8080
    protocol: TCP
    targetPort: 8080
  selector:
    app: httpbin
```
which we deploy with the following command:
```
% kubectl apply -f service.yaml 
service "httpbin" created
% kubectl get svc httpbin
NAME      TYPE       CLUSTER-IP      EXTERNAL-IP   PORT(S)          AGE
httpbin   NodePort   10.51.250.182   <none>        8080:31205/TCP   57m
```
Now it's time to expose the service to the world with Contour.
We do this with an Ingress object:
```
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: httpbin
spec:
  rules:
  - host: httpbin.davecheney.com
    http:
      paths:
      - backend:
          serviceName: httpbin
          servicePort: 8080
```
Note that the host name I specified, `httpbin.davecheney.com`, is part of a domain that I control.
`httpbin.davecheney.com` is a `CNAME` to the `gke.davecheney.com` record that we created in the first section.
This lets requests to `httpbin.davecheney.com` resolve to the external IP address of the Contour service.
They are then forwarded to the Contour pods running in my cluster:
```
% host httpbin.davecheney.com
httpbin.davecheney.com is an alias for gke.davecheney.com.
gke.davecheney.com has address 35.189.26.87
```
Change the value of `spec.rules.host` to something that you control, and deploy the Ingress to your cluster:
```
% kubectl apply -f ingress.yaml
ingress "httpbin" created
% kubectl get ing httpbin
NAME      HOSTS                    ADDRESS   PORTS     AGE
httpbin   httpbin.davecheney.com             80        58m
```
Now you can type the host name of the service into a browser, or use curl, to verify it's deployed and everything is working:
```
% curl http://httpbin.davecheney.com/get
{
  "args": {}, 
  "headers": {
    "Accept": "*/*", 
    "Content-Length": "0", 
    "Host": "htpbin.davecheney.com", 
    "User-Agent": "curl/7.58.0", 
    "X-Envoy-Expected-Rq-Timeout-Ms": "15000", 
    "X-Envoy-Internal": "true"
  }, 
  "origin": "10.152.0.2", 
  "url": "http://httpbin.davecheney.com/get"
}
```
Excellent, it looks like everything is up and running serving traffic over HTTP.

### Request a TLS certificate from Let's Encrypt
Now it's time to use cert-manager to request a TLS certificate from Let's Encrypt.
We do this by adding two annotations and a `tls:` section to our Ingress' spec.
```
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: httpbin
  annotations:
    kubernetes.io/tls-acme: "true"
    certmanager.k8s.io/cluster-issuer: "letsencrypt-staging"
spec:
  tls:
  - secretName: httpbin
    hosts: 
    - httpbin.davecheney.com
  rules:
  - host: httpbin.davecheney.com
    http:
      paths:
      - backend:
          serviceName: httpbin
          servicePort: 8080
```
The `kubernetes.io/tls-acme: "true"` annotation tells cert-manager to use the `letsencrypt-staging` cluster-wide issuer we created earlier to request a certificate from Let's Encrypt's staging servers.

The certificate will be issued in the name of the hosts listed in the `tls:` section, `httpbin.davecheney.com` and the stored in the secret `httpbin`.
Behind the scenes cert-manager creates a certificate CRD to manage the lifecycle of the certificate.
You can watch the progress of the certificate being issued:
```
% kubectl describe certificate httpbin | tail -n 6
  Normal   PresentChallenge       1m               cert-manager-controller  Presenting http-01 challenge for domain httpbin.davecheney.com
  Normal   SelfCheck              1m               cert-manager-controller  Performing self-check for domain httpbin.davecheney.com
  Normal   ObtainAuthorization    1m               cert-manager-controller  Obtained authorization for domain httpbin.davecheney.com
  Normal   IssueCertificate       1m               cert-manager-controller  Issuing certificate...
  Normal   CeritifcateIssued      1m               cert-manager-controller  Certificated issued successfully
  Normal   RenewalScheduled       1m (x3 over 1m)  cert-manager-controller  Certificate scheduled for renewal in 1438 hours
```
Wait for the certificate to be issued:
```
% kubectl describe certificate httpbin | grep -C3 CertIssueSuccess
  Conditions:
    Last Transition Time:  2018-02-26T01:26:30Z
    Message:               Certificate issued successfully
    Reason:                CertIssueSuccess
    Status:                True
    Type:                  Ready
```
A `kubernetes.io/tls` secret is created with the `secretName` you specified in your Ingress' `tls:` section.
```
% kubectl get secret httpbin
NAME      TYPE                DATA      AGE
httpbin   kubernetes.io/tls   2         3m
```
cert-manager manages the contents of the secret as long as the Ingress is present in your cluster.

After you add the `tls:` stanza to the Ingress spec, you can visit your site, replacing `http://` with `https://` — and you get a huge security warning!
This is because the certificate was issued by the staging Let's Encrypt servers and has a fake CA.
This is so you can't accidentally use the staging servers to serve real certificates.
```
% curl https://httpbin.davecheney.com/get
curl: (60) SSL certificate problem: unable to get local issuer certificate
More details here: https://curl.haxx.se/docs/sslcerts.html

curl failed to verify the legitimacy of the server and therefore could not
establish a secure connection to it. To learn more about this situation and
how to fix it, please visit the web page mentioned above.
```
### Switch to Let's Encrypt Production
Now it's time to switch to the Let's Encrypt production servers to request a certificate signed with the Let's Encrypt trusted root CA.

To do this we create a new `ClusterIssuer`, as before but with some modifications.
Create a file called letsencrypt-prod.yaml with the following contents:
```
apiVersion: certmanager.k8s.io/v1alpha1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
  namespace: cert-manager
spec:
  acme:
    email: user@example.com
    http01: {}
    privateKeySecretRef:
      name: letsencrypt-prod
    server: https://acme-v01.api.letsencrypt.org/directory
```
again replacing user@example.com with your email address, then deploy it:
```
% kubectl apply -f letsencrypt-prod.yaml
clusterissuer "letsencrypt-prod" created
```
Now we use `kubectl edit ing httpbin` to edit our Ingress to ask for a real certificate from `letsencrypt-prod`:
```
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: httpbin
  annotations:
    certmanager.k8s.io/cluster-issuer: letsencrypt-prod
spec:
  ...
```
Next, delete the existing certificate CRD and the secret that contains the untrusted staging certificate.
This triggers cert-manager to request the certificate again from the production Let's Encrypt server.
```
% kubectl delete certificate httpbin
certificate "httpbin" deleted
% kubectl delete secret httpbin
secret "httpbin" deleted
```
Check that the `httpbin` Secret is recreated, to make sure that the certificate is issued again.
Now revisiting our `https://httpbin.davecheney.com` site should show a valid, trusted, HTTPS certificate.
```
% curl https://httpbin.davecheney.com/get
{
  "args": {}, 
  "headers": {
    "Accept": "*/*", 
    "Content-Length": "0", 
    "Host": "httpbin.davecheney.com", 
    "User-Agent": "curl/7.58.0", 
    "X-Envoy-Expected-Rq-Timeout-Ms": "15000", 
    "X-Envoy-Internal": "true"
  }, 
  "origin": "10.152.0.2", 
  "url": "https://httpbin.davecheney.com/get"
}
```
![httpbin.davecheney.com screenshot][httpbin]
## Wrapping up
Now that you've deployed your first HTTPS site using Contour and Let's Encrypt, deploying additional TLS enabled services is much simpler.
Just remember that for each HTTPS website you deploy, you create a Certificate CRD that provides the domain name and the name of the target secret.
After the secret is created, you add a tls section to your site's Ingress object.

## Bonus points
For bonus points, it's 2018 and you probably shouldn't be serving traffic over insecure HTTP any more.
Now we have TLS configured for our web service, we can use a feature of Contour to automatically upgrade any HTTP request to the corresponding HTTPS site.

To enable the automatic redirect from HTTP to HTTPS, add this annotation to your Ingress object.
```
metadata:
 annotations:
   ingress.kubernetes.io/force-ssl-redirect: "true"
```
Now any requests to the insecure HTTP version of your site get an unconditional 301 redirect to the HTTPS version:
```
% curl -v http://httpbin.davecheney.com/get
* Trying 35.189.26.87…
* TCP_NODELAY set
* Connected to httpbin.davecheney.com (35.189.26.87) port 80 (#0)
> GET /get HTTP/1.1
> Host: httpbin.davecheney.com
> User-Agent: curl/7.58.0
> Accept: */*
> 
< HTTP/1.1 301 Moved Permanently
< location: https://httpbin.davecheney.com/get
< date: Tue, 20 Feb 2018 04:11:46 GMT
< server: envoy
< content-length: 0
< 
* Connection #0 to host httpbin.davecheney.com left intact
```

[0]: https://github.com/heptio/contour/
[1]: https://github.com/jetstack/cert-manager
[2]: https://letsencrypt.org/docs/rate-limits/
[3]: http://httpbin.org/
[kuard]: cert-manager/kuard.png
[httpbin]: cert-manager/httpbin.png
