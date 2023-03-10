---
title: External Authorization Support
---

Starting in version 1.9, Contour supports routing client requests to an
external authorization server. This feature can be used to centralize
client authorization so that applications don't have to implement their
own authorization mechanisms.

## Authorization Architecture

An external authorization server is a server that implements the Envoy
external authorization [GRPC protocol][3]. Contour supports any server
that implements this protocol.

You can bind an authorization server to Contour by creating a
[`ExtensionService`][4] resource.
This resource tells Contour the service exists, and that it should
program Envoy with an upstream cluster directing traffic to it.
Note that the `ExtensionService` resource just binds the server; at this
point Contour doesn't assume that the server is an authorization server.

Once you have created `ExtensionService` resource, you can bind it to a
particular application by referencing it in a [`HTTPProxy`][5] resource.
In the `virtualhost` field, a new `authorization` field specifies the name
of an `ExtensionService` to bind for the virtual host.
When you specify a resource name here, Contour will program Envoy to
send authorization checks to the extension service cluster before routing
the request to the upstream application.

## Authorization Request Flow

It is helpful to have a mental model of how requests flow through the various
servers involved in authorizing HTTP requests.
The flow diagram below shows the actors that participate in the successful
authorization of an HTTP request.
Note that in some cases, these actors can be combined into a single
application server.
For example, there is no requirement for the external authorization server to
be a separate application from the authorization provider.


<p align="center">
<img src="/img/uml/client-auth-sequence-ext.png" alt="client authorization sequence diagram"/>
</p>

A HTTP Client generates an HTTP request and sends it to
an Envoy instance that Contour has programmed with an external
authorization configuration.
Envoy holds the HTTP request and sends an authorization check request
to the Authorization server that Contour has bound to the virtual host.
The Authorization server may be able to verify the request locally, but in
many cases it will need to make additional requests to an Authorization
Provider server to verify or obtain an authorization token.

In this flow, the ExtAuth server is able to authorize the request, and sends an
authorization response back to the Proxy.
The response includes the authorization status, and a set of HTTP headers
modifications to make to the HTTP request.
Since this authorization was successful, the Proxy modifies the request and
forwards it to the application.
If the authorization was not successful, the Proxy would have immediately
responded to the client with an HTTP error.

## Using the Contour Authorization Server

The Contour project has built a simple authorization server named
[`contour-authserver`][1]. `contour-authserver` supports an authorization
testing server, and an HTTP basic authorization server that accesses
credentials stored in [htpasswd][2] format.

To get started, ensure that Contour is deployed and that you have
[cert-manager][6] installed in your cluster so that you can easily issue
self-signed TLS certificates.

At this point, we should also create a cluster-wide self-signed certificate
issuer, just to make it easier to provision TLS certificates later:

```bash
$ kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned
spec:
  selfSigned: {}
EOF
clusterissuer.cert-manager.io/selfsigned created
```

### Deploying the Authorization Server

The first step is to deploy `contour-authserver` to the `projectcontour-auth`
namespace.
To do this, we will use [`kustomize`][8] to build a set of YAML object that we can
deploy using kubectl.
In a new directory, create the following `kustomization.yaml` file:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: projectcontour-auth

resources:
- github.com/projectcontour/contour-authserver/config/htpasswd

patchesJson6902:
- target:
    group: cert-manager.io
    version: v1
    kind: Certificate
    name: htpasswd
    namespace: projectcontour-auth
  patch: |- 
    - op: add
      path: /spec/issuerRef/kind
      value: ClusterIssuer

images:
- name: contour-authserver:latest
  newName: docker.io/projectcontour/contour-authserver
  newTag: v2
```

Note that the kustomization patches the `Certificate` resource to use the
"selfsigned" `ClusterIssuer` that we created earlier.
This is required because the `contour-authserver` deployment includes a
request for a self-signed TLS server certificate.
In a real deployment, this certificate should be requested from a real trusted
certificate issuer.

Now create the `projectcontour-auth` namespace, build the deployment YAML,
and apply to your cluster:

```bash
$ kubectl create namespace projectcontour-auth
namespace/projectcontour-auth created
$ kubectl apply -f <(kustomize build .)
serviceaccount/htpasswd created
clusterrole.rbac.authorization.k8s.io/contour:authserver:htpasswd created
clusterrolebinding.rbac.authorization.k8s.io/contour:authserver:htpasswd created
service/htpasswd created
deployment.apps/htpasswd created
certificate.cert-manager.io/htpasswd created
```

At this point, `contour-authserver` is deployed and is exposed to
the cluster as the Service `projectcontour-auth/htpasswd`.
It has a self-signed TLS certificate and is accepting secure connections
on port 9443.

In the default configuration, `contour-authserver` will accept htpasswd data
from secrets with the `projectcontour.io/auth-type: basic` annotation.
Most systems install the Apache [`htpasswd`][7] tool, which we can use
to generate the password file:

```bash
$ touch auth
$ htpasswd -b auth user1 password1
Adding password for user user1
$ htpasswd -b auth user2 password2
Adding password for user user2
$ htpasswd -b auth user3 password3
Adding password for user user3
```

Once we have some password data, we can populate a Kubernetes secret with it.
Note that the password data must be in the `auth` key in the secret, and that
the secret must be annotated with the `projectcontour.io/auth-type` key.

```bash
$ kubectl create secret generic -n projectcontour-auth passwords --from-file=auth
secret/passwords created
$ kubectl annotate secret -n projectcontour-auth passwords projectcontour.io/auth-type=basic
secret/passwords annotated
```

### Creating an Extension Service

Now that `contour-authserver` is deployed, the next step is to create a
`ExtensionService` resource.

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ExtensionService
metadata:
  name: htpasswd
  namespace: projectcontour-auth
spec:
  protocol: h2
  services:
  - name: htpasswd
    port: 9443
```

The `ExtensionService` resource must be created in the same namespace
as the services that it binds.
This policy ensures that the creator of the `ExtensionService` also has
the authority over those services.

```bash
$ kubectl apply -f htpasswd.yaml
extensionservice.projectcontour.io/htpasswd created
```

### Deploying a Sample Application

To demonstrate how to use the authorization server in a `HTTPProxy` resource,
we first need to deploy a simple echo application.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ingress-conformance-echo
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: ingress-conformance-echo
  template:
    metadata:
      labels:
        app.kubernetes.io/name: ingress-conformance-echo
    spec:
      containers:
      - name: conformance-echo
        image: agervais/ingress-conformance-echo:latest
        ports:
        - name: http-api
          containerPort: 3000
        readinessProbe:
          httpGet:
            path: /health
            port: 3000
---
apiVersion: v1
kind: Service
metadata:
  name: ingress-conformance-echo
spec:
  ports:
  - name: http
    port: 80
    targetPort: http-api
  selector:
    app.kubernetes.io/name: ingress-conformance-echo
```

This echo server will respond with a JSON object that reports information about
the HTTP request it received, including the request headers.

```bash
$ kubectl apply -f echo.yaml
deployment.apps/ingress-conformance-echo created
service/ingress-conformance-echo created
```

Once the application is running, we can expose it to Contour with a `HTTPProxy`
resource.

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ingress-conformance-echo
spec:
  dnsNames:
  - local.projectcontour.io
  secretName: ingress-conformance-echo
  issuerRef:
    name: selfsigned
    kind: ClusterIssuer
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    tls:
      secretName: ingress-conformance-echo
  routes:
  - services:
    - name: ingress-conformance-echo
      port: 80
```

_Note that we created a TLS secret and exposed the application over HTTPS._

```bash
$ kubectl apply -f echo-proxy.yaml
certificate.cert-manager.io/ingress-conformance-echo created
httpproxy.projectcontour.io/echo created
$ kubectl get proxies echo
NAME   FQDN                      TLS SECRET                 STATUS   STATUS DESCRIPTION
echo   local.projectcontour.io   ingress-conformance-echo   valid    valid HTTPProxy
```

We can verify that the application is working by requesting any path:

```bash
$ curl -k https://local.projectcontour.io/test/$((RANDOM))
{"TestId":"","Path":"/test/12707","Host":"local.projectcontour.io","Method":"GET","Proto":"HTTP/1.1","Headers":{"Accept":["*/*"],"Content-Length":["0"],"User-Agent":["curl/7.64.1"],"X-Envoy-Expected-Rq-Timeout-Ms":["15000"],"X-Envoy-Internal":["true"],"X-Forwarded-For":["172.18.0.1"],"X-Forwarded-Proto":["https"],"X-Request-Id":["7b87d5d1-8ee8-40e3-81ac-7d74dfd4d50b"],"X-Request-Start":["t=1601596511.489"]}}
```

### Using the Authorization Server

Now that we have a working application exposed by a `HTTPProxy` resource, we
can add HTTP basic authorization by binding to the `ExtensionService` that we
created earlier.
The simplest configuration is to add an `authorization` field that names the
authorization server `ExtensionService` resource that we created earlier.

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    tls:
      secretName: ingress-conformance-echo
    authorization:
      extensionRef:
        name: htpasswd
        namespace: projectcontour-auth
  routes:
  - services:
    - name: ingress-conformance-echo
      port: 80
```

```bash
$ kubectl apply -f echo-auth.yaml
httpproxy.projectcontour.io/echo configured
```

Now, when we make the same HTTP request, we find the response requests
authorization:

```bash
$ curl -k -I https://local.projectcontour.io/test/$((RANDOM))
HTTP/2 401
www-authenticate: Basic realm="default", charset="UTF-8"
date: Fri, 02 Oct 2020 00:27:49 GMT
server: envoy
```

Providing a user credential from the password file that we created
earlier allows the request to succeed. Note that `contour-authserver`
has injected a number of headers (prefixed with `Auth-`) to let the
application know how the request has been authorized.

```bash
$ curl -k --user user1:password1 https://local.projectcontour.io/test/$((RANDOM))
{"TestId":"","Path":"/test/27132","Host":"local.projectcontour.io","Method":"GET","Proto":"HTTP/1.1","Headers":{"Accept":["*/*"],"Auth-Handler":["htpasswd"],"Auth-Realm":["default"],"Auth-Username":["user1"],"Authorization":["Basic dXNlcjE6cGFzc3dvcmQx"],"Content-Length":["0"],"User-Agent":["curl/7.64.1"],"X-Envoy-Expected-Rq-Timeout-Ms":["15000"],"X-Envoy-Internal":["true"],"X-Forwarded-For":["172.18.0.1"],"X-Forwarded-Proto":["https"],"X-Request-Id":["2c0ae102-4cf6-400e-a38f-5f0b844364cc"],"X-Request-Start":["t=1601601826.102"]}}
```

## Global External Authorization

Starting from version 1.25, Contour supports global external authorization. This allows you to setup a single external authorization configuration for all your virtual hosts (HTTP and HTTPS).

To get started, ensure you have `contour-authserver` and the `ExtensionService` deployed as described above. 

### Global Configuration

Define the global external authorization configuration in your contour config. 

```yaml
globalExtAuth:
  extensionService: projectcontour-auth/htpasswd
  failOpen: false
  authPolicy:
    context:
      header1: value1
      header2: value2
  responseTimeout: 1s
```

Setup a HTTPProxy without TLS
```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
  routes:
  - services:
    - name: ingress-conformance-echo
      port: 80
```

```
$ kubectl apply -f echo-proxy.yaml 
httpproxy.projectcontour.io/echo created
```

When we make a HTTP request without authentication details, we can see that the endpoint is secured and returns a 401. 

```
$ curl -k -I http://local.projectcontour.io/test/$((RANDOM))
HTTP/1.1 401 Unauthorized
www-authenticate: Basic realm="default", charset="UTF-8"
vary: Accept-Encoding
date: Mon, 20 Feb 2023 13:45:31 GMT
```

If you add the username and password to the same request you can verify that the request succeeds. 
```
$ curl -k --user user1:password1 http://local.projectcontour.io/test/$((RANDOM))
{"TestId":"","Path":"/test/27748","Host":"local.projectcontour.io","Method":"GET","Proto":"HTTP/1.1","Headers":{"Accept":["*/*"],"Auth-Context-Header1":["value1"],"Auth-Context-Header2":["value2"],"Auth-Context-Routq":["global"],"Auth-Handler":["htpasswd"],"Auth-Realm":["default"],"Auth-Username":["user1"],"Authorization":["Basic dXNlcjE6cGFzc3dvcmQx"],"User-Agent":["curl/7.86.0"],"X-Envoy-Expected-Rq-Timeout-Ms":["15000"],"X-Envoy-Internal":["true"],"X-Forwarded-For":["172.18.0.1"],"X-Forwarded-Proto":["http"],"X-Request-Id":["b6bb7036-8408-4b03-9ce5-7011d89799b4"],"X-Request-Start":["t=1676900780.118"]}}
```

Global external authorization can also be configured with TLS virtual hosts. Update your HTTPProxy by adding `tls` and `secretName` to it. 

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    tls:
      secretName: ingress-conformance-echo
  routes:
  - services:
    - name: ingress-conformance-echo
      port: 80
```

```
$ kubectl apply -f echo-proxy.yaml
httpproxy.projectcontour.io/echo configured
```

you can verify the HTTPS requests succeeds
```
$ curl -k --user user1:password1 https://local.projectcontour.io/test/$((RANDOM))
{"TestId":"","Path":"/test/13499","Host":"local.projectcontour.io","Method":"GET","Proto":"HTTP/1.1","Headers":{"Accept":["*/*"],"Auth-Context-Header1":["value1"],"Auth-Context-Header2":["value2"],"Auth-Context-Routq":["global"],"Auth-Handler":["htpasswd"],"Auth-Realm":["default"],"Auth-Username":["user1"],"Authorization":["Basic dXNlcjE6cGFzc3dvcmQx"],"User-Agent":["curl/7.86.0"],"X-Envoy-Expected-Rq-Timeout-Ms":["15000"],"X-Envoy-Internal":["true"],"X-Forwarded-For":["172.18.0.1"],"X-Forwarded-Proto":["https"],"X-Request-Id":["2b3edbed-3c68-44ef-a659-2e1245d7fe13"],"X-Request-Start":["t=1676901557.918"]}}
```

### Excluding a virtual host from global external authorization

You can exclude a virtual host from the global external authorization policy by setting the `disabled` flag to true under `authPolicy`. 

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    tls:
      secretName: ingress-conformance-echo
    authorization:
      authPolicy:
        disabled: true
  routes:
  - services:
    - name: ingress-conformance-echo
      port: 80
```

```
$ kubectl apply -f echo-proxy.yaml
httpproxy.projectcontour.io/echo configured
```

You can verify that an insecure request succeeds without being authorized. 

```
$ curl -k https://local.projectcontour.io/test/$((RANDOM))
{"TestId":"","Path":"/test/51","Host":"local.projectcontour.io","Method":"GET","Proto":"HTTP/1.1","Headers":{"Accept":["*/*"],"User-Agent":["curl/7.86.0"],"X-Envoy-Expected-Rq-Timeout-Ms":["15000"],"X-Envoy-Internal":["true"],"X-Forwarded-For":["172.18.0.1"],"X-Forwarded-Proto":["https"],"X-Request-Id":["18716e12-dcce-45ba-a3bb-bc26af3775d2"],"X-Request-Start":["t=1676901847.802"]}}
```

### Overriding global external authorization for a HTTPS virtual host

You may want a different configuration than what is defined globally. To override the global external authorization, add the `authorization` block to your TLS enabled HTTPProxy as shown below

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: echo
spec:
  virtualhost:
    fqdn: local.projectcontour.io
    tls:
      secretName: ingress-conformance-echo
    authorization:
      extensionRef:
        name: htpasswd
        namespace: projectcontour-auth
  routes:
  - services:
    - name: ingress-conformance-echo
      port: 80
```

```
$ kubectl apply -f echo-proxy.yaml
httpproxy.projectcontour.io/echo configured
```

You can verify that the endpoint has applied the overridden external authorization configuration.

```
$ curl -k --user user1:password1 https://local.projectcontour.io/test/$((RANDOM))
{"TestId":"","Path":"/test/4514","Host":"local.projectcontour.io","Method":"GET","Proto":"HTTP/1.1","Headers":{"Accept":["*/*"],"Auth-Context-Overriden_message":["overriden_value"],"Auth-Handler":["htpasswd"],"Auth-Realm":["default"],"Auth-Username":["user1"],"Authorization":["Basic dXNlcjE6cGFzc3dvcmQx"],"User-Agent":["curl/7.86.0"],"X-Envoy-Expected-Rq-Timeout-Ms":["15000"],"X-Envoy-Internal":["true"],"X-Forwarded-For":["172.18.0.1"],"X-Forwarded-Proto":["https"],"X-Request-Id":["8a02d6ce-8be0-4e87-8ed8-cca7e239e986"],"X-Request-Start":["t=1676902237.111"]}}
```

NOTE: You can only override the global external configuration on a HTTPS virtual host.

## Caveats

There are a few caveats to consider when deploying external
authorization:

1. Only one external authorization server can be configured on a virtual host
1. HTTP hosts are only supported with global external authorization.
1. External authorization cannot be used with the TLS fallback certificate (i.e. client SNI support is required)

[1]: https://github.com/projectcontour/contour-authserver
[2]: https://httpd.apache.org/docs/current/misc/password_encryptions.html
[3]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/service/auth/v3/external_auth.proto
[4]: ../config/api/#projectcontour.io/v1alpha1.ExtensionService
[5]: ../config/api/#projectcontour.io/v1.HTTPProxy
[6]: https://cert-manager.io/
[7]: https://httpd.apache.org/docs/current/programs/htpasswd.html
[8]: https://kubernetes-sigs.github.io/kustomize/
