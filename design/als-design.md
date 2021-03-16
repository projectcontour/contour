# AccessLog Service (ALS) Support

Status: Draft

## Abstract

This document proposed a design for supporting Envoy's AccessLog Service capabilities in Contour.

## Background

Envoy has the ability to send rich access logs to access logging sinks.
With AccessLog Service (ALS) enabled, Envoy can send access log messages using gRPC streams to a gRPC access log service. 

This document focuses on configuring access log sinks and enabling Envoy to send access log messages via Contour.

## Goals
- Enable users to specify ALS configuration
- Enable users to send HTTP access logs for specific listeners 

## Non Goals
- TCP Access Log Support
- As part of first version, communicate between Envoy and ALS sinks will be insecure
- As part of first version, users can only specify a single ALS sink.

## High-Level Design
Contour will add support for Envoy's HTTP Access Log Service.
ALS information will be specified as a [static cluster for Envoy](https://github.com/envoyproxy/envoy/blob/main/source/common/grpc/async_client_manager_impl.cc#L44).
Contour controller will add details about ALS sink to `access_log` field for each HTTPConnectionManager filter.

## Detailed Design
To enable Envoy to send access logs to a sink; a static cluster needs to be set for Envoy that will have details about ALS sink.
There are two ways to add this configuration:
- ExtensionService
- Contour Configuration file
We have chosen to use ExtensionService to enable users to configure ALS sink details.

### ExtensionService
Contour administrator/operator can add an ExtensionService with ALS configuration details.
As part of `contour bootstrap`; the ExtensionService will be used to configure a static ALS cluster for Envoy.

An annotation `projectcontour.io/als-admitted: true` needs to be added to the particular ExtensionService.
This annotations hints `contour bootstrap` to use this ExtensionService for setting the ALS cluster. 
`contour bootstrap` will populate `Conditions` field to reflect if ALS sink could be correctly configured for Envoy.

Contour controller to will read ALS details from ExtensionService and Conditions field of ExtensionService.
If there are no configuration errors for ALS; then Contour will populate `access_log` field for each HTTP filter.
Required changes will be made to Kubernetes RBAC that would enable `contour bootstrap` to read from `ExtensionService`.
For example:
```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ExtensionService
metadata:
  namespace: projectcontour
  name: accesslog-service
  annotations:
    projectcontour.io/als-admitted: true
spec:
  protocol: h2
  services:
    - name: accesslog-service
      port: 8080
```
### Configuration File
New command line args will be added to `contour bootstrap` for passing ALS details. `contour bootstrap` will use this ALS details to configure a static ALS sink for Envoy. `contour bootstrap` will write out the ALS details to a Kubernetes Configmap. This needs to be done to ensure that users don't have to specify the same information 2 times; (once for `contour bootstrap` and then for `contour serve`).
`contour serve` can then read ALS information for this generated configmap and use this information to populate `access_log` field for each HTTP filter.

Required changes will be made to Kubernetes RBAC to enable required access for `contour bootstrap` and `contour serve`.