# Kubernetes Ingress

Contour supports Kubernetes Ingress and implements many advanced features via [annotations][0].

More information about the Ingress spec can be found in the Kubernetes docs: [https://kubernetes.io/docs/concepts/services-networking/ingress/][1]

## Status

Contour updates the `status` field on all Ingress objects if the `Host` or `IP` is available through the Envoy service's (`type=LoadBalancer`) [status][2] information.
The default service which Contour looks for this information is the namespace in which Contour is deployed and the service named `envoy`, however this is
configurable by setting the following flags on Contour:

- `--envoy-service-name`: Defines the name of the Envoy service which this instance of Contour sends configuration (Defaults to `envoy`)
- `--envoy-service-namespace`: Defines the namespace of the Envoy service which this instance of Contour sends configuration (Defaults to namespace of where Contour is deployed)

[0]: annotations.md
[1]: https://kubernetes.io/docs/concepts/services-networking/ingress/
[2]: https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer
