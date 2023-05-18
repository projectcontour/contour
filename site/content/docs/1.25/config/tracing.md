# Tracing Support

- [Overview](#overview)
- [Tracing-config](#tracing-config)

## Overview

Envoy has rich support for [distributed tracing][1]，and supports exporting data to third-party providers (Zipkin, Jaeger, Datadog, etc.)

[OpenTelemetry][2] is a CNCF project which is working to become a standard in the space. It was formed as a merger of the OpenTracing and OpenCensus projects.

Contour supports configuring envoy to export data to OpenTelemetry, and allows users to customize some configurations.

- Custom service name, the default is `contour`.
- Custom sampling rate, the default is `100`.
- Custom the maximum length of the request path, the default is `256`.
- Customize span tags from literal or request headers.
- Customize whether to include the pod's hostname and namespace.

## Tracing-config

In order to use this feature, you must first select and deploy an opentelemetry-collector to receive the tracing data exported by envoy. 

First we should deploy an opentelemetry-collector to receive the tracing data exported by envoy
```bash
# install operator
kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml
```

Install an otel collector instance, with verbose logging exporter enabled:
```shell
kubectl apply -f - <<EOF
apiVersion: opentelemetry.io/v1alpha1
kind: OpenTelemetryCollector
metadata:
  name: simplest
  namespace: projectcontour
spec:
  config: |
    receivers:
      otlp:
        protocols:
          grpc:
          http:
    exporters:
      logging:
        verbosity: detailed

    service:
      pipelines:
        traces:
          receivers: [otlp]
          exporters: [logging]
EOF
```

Define extension service:
```shell
kubectl apply -f - <<EOF
apiVersion: projectcontour.io/v1alpha1
kind: ExtensionService
metadata:
  name: otel-collector
  namespace: projectcontour
spec:
  protocol: h2c
  services:
    - name: simplest-collector
      port: 4317
EOF
```

Update Contour config (needs a Contour deployment restart afterwards):
```shell
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    tracing:
      # Whether to send the namespace and instance where envoy is located to open, the default is true.
      includePodDetail: true
      # The extensionService and namespace and name defined above in the format of namespace/name.
      extensionService: projectcontour/otel-collector
      # The service name that envoy sends to openTelemetry-collector, the default is contour.
      serviceName: some-service-name
      # A custom set of tags.
      customTags:
      # envoy will send the tagName to the collector.
      - tagName: custom-tag
        # fixed tag value.
        literal: foo
      - tagName: header-tag
        # The tag value obtained from the request header, 
        # if the request header does not exist, this tag will not be sent.
        requestHeaderName: X-Custom-Header        
EOF
```

Install httpbin and test it:
```bash
kubectl apply -f https://projectcontour.io/examples/httpbin.yaml
```

Access the api and view the logs of simplest:
```bash
kubectl logs deploy/simplest-collector -n projectcontour
```

Now you should be able to see traces in the logs of the otel collector.

[1]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/observability/tracing
[2]: https://opentelemetry.io/

