# External Service Routing

HTTPProxy supports routing traffic to `ExternalName` service types.
Contour looks at the `spec.externalName` field of the service and configures the route to use that DNS name instead of utilizing EDS.

There's nothing specific in the HTTPProxy object that needs to be configured other than referencing a service of type `ExternalName`.
HTTPProxy supports the `requestHeadersPolicy` field to rewrite the `Host` header after first handling a request and before proxying to an upstream service.
This field can be used to ensure that the forwarded HTTP request contains the hostname that the external resource is expecting.

_**Note:** The ports are required to be specified._

```yaml
# httpproxy-externalname.yaml
apiVersion: v1
kind: Service
metadata:
  labels:
    run: externaldns
  name: externaldns
  namespace: default
spec:
  externalName: foo-basic.bar.com
  ports:
  - name: http
    port: 80
    protocol: TCP
    targetPort: 80
  type: ExternalName
```

To proxy to another resource outside the cluster (e.g. A hosted object store bucket for example), configure that external resource in a service type `externalName`.
Then define a `requestHeadersPolicy` which replaces the `Host` header with the value of the external name service defined previously.
Finally, if the upstream service is served over TLS, set the `protocol` field on the service to `tls` or annotate the external name service with: `projectcontour.io/upstream-protocol.tls: 443,https`, assuming your service had a port 443 and name `https`.
