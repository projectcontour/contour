# Minikube

Minikube is not recommended as a platform to test or develop Contour due to its network limitations.
You can, however, run Contour on Minikube to explore how it works.
This should be a preliminary exploration only.

## Access your cluster

On Minikube you can retrieve the external address of Contour's load balancer:
```
$ minikube service -n heptio-contour contour --url
http://192.168.99.100:30588
```

## Configuring DNS

Because of known issues with how Minikube manages DNS, we recommend that you run the provided example so that you do not have to modify DNS on your local machine.
Otherwise, you must fake DNS by editing `/etc/hosts`.

## Deploy KUARD example application.

```
$ kubectl apply -f https://j.hept.io/contour-kuard-ninikube-example
```

This example specifies `kuard.192.168.99.100.nip.io` as a specific ingress backend for kuard.
It uses nip.io and the Minikube IP address so that kuard responds only at http://kuard.192.168.99.100.nip.io.
Once that is applied you can visit http://kuard.192.168.99.100.nip.io and see the kuard example application.

## Troubleshooting

Minikube remaps Contour's service load balancer from ports 80 and 443 to a random high port as the Ingress.
This is problematic because this port is not a _well known_ (see RFC 2616) port so `curl(1)` or browsers will include the port number in the `Host:` header.
This causes Envoy to misroute the request because the domain name in the RDS virtualhost entry does not contain the `:port` suffix assigned by Minikube.

The problem is the port Minikube chooses is not easily predictable, so it is not simply a matter of including various permutations of hostname:port in the virtualhost.domains array.

### Workarounds
You can either force the `Host:` header with something like:
```
curl -H "Host: example.com" -v http://example.com:31847
```
Or run curl with an OSX-specific extension:
```
curl -v --connect-to example.com:80:example.com:31847 http://example.com/
```

This is tracked as issue [#210][1], which is blocked on [envoyproxy/envoy#1269][2].
At the moment there is no ETA when these issues will be resolved.

[1]: https://github.com/heptio/contour/issues/210
[2]: https://github.com/envoyproxy/envoy/issues/1269
