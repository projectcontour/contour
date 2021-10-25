### Performance improvement for processing configuration

The performance of Contour's configuration processing has been made more efficient, particularly for clusters with large numbers (i.e. >1k) of HTTPProxies and/or Ingresses.
This means that there should be less of a delay between creating/updating an HTTPProxy/Ingress in Kubernetes, and having it reflected in Envoy's configuration.
