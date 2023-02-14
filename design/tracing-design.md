# Design for Supporting Tracing in Contour

Status: Accepted

## Abstract
Envoy has rich support for [distributed tracing][1].
Exporting trace data to OpenTelemetry has been supported since envoy v1.23.
However, Contour does not currently offer a way to configure Envoy for tracing.
This design document provides the implementation of exporting tracing data to [OpenTelemetry][2] in Contour.

## Background
Envoy's [documentation on tracing][1] says:
> Distributed tracing allows developers to obtain visualizations of call flows in large service oriented architectures. It can be invaluable in understanding serialization, parallelism, and sources of latency.
 
Envoy supports:
- generating traces for incoming requests
- joining existing traces when a client sets the `x-client-trace-id` header
- exporting trace data to various third-party providers (Zipkin, Jaeger, Datadog, etc.)

The tracing ecosystem is complex right now.
However, a few observations can be made:
- [OpenTelemetry][2] is a CNCF project which is working to become a standard in the space. It was formed as a merger of the OpenTracing and OpenCensus projects. It supports ingesting trace data in a variety of formats, transforming it, and exporting it to a variety of backends.
- Envoy has had support for exporting tracing data to OpenTelemetry since v1.23.

## Goals
- **Support a single global trace configuration** that applies to all requests.
- **Support OpenTelemetry formats**, Because OpenTelemetry is an observability standard. Data can be ingested, transformed, and sent to an observability backend.
- **Support custom tags to send additional span data**, including getting data from request headers, etc.

## Non Goals
- **Per-HTTPProxy trace configuration.** There is a technical limitation to enabling per-proxy configuration. Trace settings are configured on the HTTP connection manager (HCM). Each TLS-enabled virtual host uses its own HCM, but *all* non-TLS virtual hosts share a single HCM. As such, while it's possible to have unique trace settings for each TLS virtual host, it's not possible to do the same for non-TLS virtual hosts.  Note that this same limitation has come up when designing external authorization and global rate limiting as well.
- **Trace formats other than OpenTelemetry.** While Contour *may* choose to add direct support for other trace formats in the future, our hope is that by supporting the OpenTelemetry collector, users are able to export traces to the backend of their choice without having to directly configure it in Contour/Envoy.

## High-Level Design
The Contour configuration file and ContourConfiguration CRD will be extended with a new optional `tracing` section. This configuration block, if present, will enable tracing and will define the trace format and other properties needed to generate and export trace data.

The `OpenTelemetry` trace format will be supported.

The parameters defined in the configuration file will be used to configure each [HTTP connection manager's `tracing` section][3].

Since the tracing backend is pluggable, Contour will not package any particular backend.
The user is responsible for deploying and operating the tracing backend of their choice, and configuring Contour to make use of it.

## Detailed Design

### otel-collector ExtensionService

When using tracing, the user must first define an ExtensionService with the cluster-level details for the otel-collector itself.

For example:

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ExtensionService
metadata:
  namespace: projectcontour
  name: otel-collector
spec:
  protocol: h2c
  services:
    - name: otel-collector
      port: 4317
  timeoutPolicy:
    response: 50ms
```

### Contour Configuration

The `tracing` section of the Contour config file will look like:

```yaml
tracing:
  # includePodDetail defines a flag. If it is true, contour will add the pod name and namespace to the span of the trace. the default is true
  includePodDetail: true
  # serviceName defines a configurable service name.the default is contour
  serviceName: contour
  # overallSampling defines the sampling rate of trace data.
  overallSampling: 100
  # maxPathTagLength defines maximum length of the request path to extract and include in the HttpUrl tag.
  maxPathTagLength: 256
  # customTags defines a list of custom tags with unique tag name.
  # A customTag can only set one of literal and requestHeaderName
  customTags:
    # tagName used to populate the tag name
    - tagName: literal
      # literal is a static custom tag value.
      literal: 'this is literal'
    - tagName: requestHeaderName
      # requestHeaderName indicates which request header the label value is obtained from.
      requestHeaderName: ':path'
  # extensionService Identifies the extension service defining the openTelemetry collector service.
  # formatted as <namespace>/<name>.    
  extensionService: projectcontour/otel-collector
```

## Alternatives Considered
If there are alternative high level or detailed designs that were not pursued they should be called out here with a brief explanation of why they were not pursued.

## Security Considerations
Similar to other extensionservices, Envoy's connection with the tracing provider server can be secured over TLS using existing mechanisms.

## Compatibility
A discussion of any compatibility issues that need to be considered

## Implementation

### HCM Configuration

In the presence of the `tracing` block above, all HTTP connection managers (HCM) will be configured for tracing.

A Tracing HCM will include the following as an example:

```go
...
Tracing: &http.HttpConnectionManager_Tracing{
            OverallSampling: &envoy_type.Percent{
                Value: 100,
            },
            MaxPathTagLength: wrapperspb.UInt32(256),
            CustomTags:       []*envoy_trace_v3.CustomTag{
                {
                    Tag: "literal",
                    Type: &envoy_trace_v3.CustomTag_Literal_{
                        Literal: &envoy_trace_v3.CustomTag_Literal{
                            Value: "this is literal",
                        },
                    },
                },
                {
                    Tag: "podName",
                    Type: &envoy_trace_v3.CustomTag_Environment_{
                        Environment: &envoy_trace_v3.CustomTag_Environment{
                            Name: "HOSTNAME",
                        },
                    },
                },
				{
                    Tag: "podNamespaceName",
                    Type: &envoy_trace_v3.CustomTag_Environment_{
                        Environment: &envoy_trace_v3.CustomTag_Environment{
                            Name: "CONTOUR_NAMESPACE",
                        },
                    },
                },
                {
                    Tag: "requestHeaderName",
                    Type: &envoy_trace_v3.CustomTag_RequestHeader{
                        RequestHeader: &envoy_trace_v3.CustomTag_Header{
                        Name:         ":path",
                        },
                    },
                },
            },
            Provider: &envoy_config_trace_v3.Tracing_Http{
                Name: "envoy.tracers.opentelemetry",
                ConfigType: &envoy_config_trace_v3.Tracing_Http_TypedConfig{
                    TypedConfig: protobuf.MustMarshalAny(&envoy_config_trace_v3.OpenTelemetryConfig{
                        GrpcService: &envoy_core_v3.GrpcService{
                            TargetSpecifier: &envoy_core_v3.GrpcService_EnvoyGrpc_{
                                EnvoyGrpc: &envoy_core_v3.GrpcService_EnvoyGrpc{
                                    ClusterName: "extension/projectcontour/otel-collector",
                                    Authority:   "extension/projectcontour/otel-collector",
                                },
                            },
                            Timeout: durationpb.New(time.Millisecond * 50),
                        },
                        ServiceName: "contour",
                    }),
                },
            },
        },
...
```

## Open Issues
- Envoy supports custom tracing sampling rate and custom label override hcm configuration in route - should contour be supported in the future?


[1]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/observability/tracing
[2]: https://opentelemetry.io/
[3]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/network/http_connection_manager/v3/http_connection_manager.proto#envoy-v3-api-msg-extensions-filters-network-http-connection-manager-v3-httpconnectionmanager-tracing
