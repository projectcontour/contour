# Design for Supporting Outlier detection in Contour

Status: Draft

## Abstract
This document recommends adding passive health checks to services to enhance routing reliability.

## Background

Envoy supports two types of health checks, namely active health check and [passive health check][1]. Passive and active health checks can be enabled simultaneously or independently, and form the basis of the overall upstream health check solution. Contour has implemented active health checks. If Passive health checks are configured to better monitor service status and route traffic to healthy instances.

## Goals
- Add outlier detection related configuration detection for services.
- Support global configuration of OutlierDetection, service level can be overridden or disabled.
- Support configuration based on route and TCPRoute.
- Support configuration for consecutive-5 xx and consecutive-local-origin-failure.

## Non-Goals
- Not supported Outlier Detection configuration for consecutive-gateway-failure, success-rate, and failure-percentage.

## Detailed Design

### Global Configuration
The `OutlierDetection` section of the Contour config file will look like:

```yaml
outlierDetection:
  # ConsecutiveServerErrors defines The number of consecutive server-side error responses before a consecutive 5xx ejection occurs.
  # When the backend host encounters consecutive
  # errors greater than or equal to ConsecutiveServerErrors, it will be
  # ejected from the load balancing pool.
  # for HTTP services, a 5xx counts as an error and for TCP services
  # connection failures and connection timeouts count as an error.
  # It can be disabled by setting the value to 0.
  # Defaults to 5.
  consecutiveServerErrors: 5
  # Interval is the interval at which host status is evaluated.
  # Defaults to 10s.
  interval: 10s
  # BaseEjectionTime is the base time that a host is ejected for.
  # A host will remain ejected for a period of time equal to the
  # product of the ejection base duration and the number of times the host has been ejected.
  # Defaults to 30s.
  baseEjectionTime: 30s
  # MaxEjectionTime is the maximum time a host will be ejected for.
  # After this amount of time, a host will be returned to normal operation.
  # If not specified, the default value (300s) or BaseEjectionTime value is applied, whatever is larger.
  # Defaults to 300s.
  maxEjectionTime: 300s
  # SplitExternalLocalOriginErrors defines whether to split the local origin errors from the external origin errors.
  # Defaults to false.
  splitExternalLocalOriginErrors: false
  # ConsecutiveLocalOriginFailure defines the number of consecutive local origin failures before a consecutive local origin ejection occurs.
  # Parameters take effect only when SplitExternalLocalOriginErrors is true.
  # Defaults to 5.
  consecutiveLocalOriginFailure: 5
  # MaxEjectionPercent is the max percentage of hosts in the load balancing pool for the upstream service that can be ejected.
  # But will eject at least one host regardless of the value here.
  # Defaults to 10%.
  maxEjectionPercent: 10
  # MaxEjectionTimeJitter is The maximum amount of jitter to add to the ejection time, 
  # in order to prevent a ‘thundering herd’ effect where all proxies try to reconnect to host at the same time.
  # Defaults to 0s.
  maxEjectionTimeJitter: 0s
```


### Httpproxy Configuration
Add a new field in the CRD of httpproxy for configuring Outlier Detection of the service. The field is defined as follows:
```go
// OutlierDetection defines the configuration for outlier detection on a service.
// If not specified, global configuration will be used.
// If specified, it will override the global configuration.
type OutlierDetection struct {
	
	// Disabled defines whether to disable outlier detection for the service.
	// Defaults to false. 
	Disabled *bool `json:"disabled,omitempty"`
    // ConsecutiveServerErrors defines The number of consecutive server-side error responses before a consecutive 5xx ejection occurs.
    // When the backend host encounters consecutive
    // errors greater than or equal to ConsecutiveServerErrors, it will be
    // ejected from the load balancing pool.
    // for HTTP services, a 5xx counts as an error and for TCP services
    // connection failures and connection timeouts count as an error.
    // It can be disabled by setting the value to 0.
    // Defaults to 5.
    // +optional
	ConsecutiveServerErrors *uint32 `json:"consecutiveServerErrors,omitempty"`
    
    // Interval is the interval at which host status is evaluated.
    // Defaults to 10s.
    // +optional
    // +kubebuilder:validation:Pattern=`^(((\d*(\.\d*)?h)|(\d*(\.\d*)?m)|(\d*(\.\d*)?s)|(\d*(\.\d*)?ms))+)$`
    Interval *string `json:"interval,omitempty"`
    
    // BaseEjectionTime is the base time that a host is ejected for.
    // A host will remain ejected for a period of time equal to the
    // product of the ejection base duration and the number of times the host has been ejected.
    // Defaults to 30s.
    // +optional
    // +kubebuilder:validation:Pattern=`^(((\d*(\.\d*)?h)|(\d*(\.\d*)?m)|(\d*(\.\d*)?s)|(\d*(\.\d*)?ms))+)$`
    BaseEjectionTime *string `json:"baseEjectionTime,omitempty"`
    
    // MaxEjectionTime is the maximum time a host will be ejected for.
    // After this amount of time, a host will be returned to normal operation.
    // If not specified, the default value (300s) or BaseEjectionTime value is applied, whatever is larger.
    // Defaults to 300s.
    // +optional
    // +kubebuilder:validation:Pattern=`^(((\d*(\.\d*)?h)|(\d*(\.\d*)?m)|(\d*(\.\d*)?s)|(\d*(\.\d*)?ms))+)$`
    MaxEjectionTime *string `json:"maxEjectionTime,omitempty"`
    
    // SplitExternalLocalOriginErrors defines whether to split the local origin errors from the external origin errors.
    // Defaults to false.
    // +optional
    // +kubebuilder:default=false
    SplitExternalLocalOriginErrors bool `json:"splitExternalLocalOriginErrors"`
    
    // ConsecutiveLocalOriginFailure defines the number of consecutive local origin failures before a consecutive local origin ejection occurs.
    // Parameters take effect only when SplitExternalLocalOriginErrors is true.
    // Defaults to 5.
    ConsecutiveLocalOriginFailure *uint32 `json:"consecutiveLocalOriginFailure,omitempty"`
    
    // MaxEjectionPercent is the max percentage of hosts in the load balancing pool for the upstream service that can be ejected.
    // But will eject at least one host regardless of the value here.
    // Defaults to 10%.
    // +optional
    // +kubebuilder:validation:Maximum=100
    MaxEjectionPercent *uint32 `json:"maxEjectionPercent,omitempty"`
	
	// MaxEjectionTimeJitter is The maximum amount of jitter to add to the ejection time, 
	// in order to prevent a ‘thundering herd’ effect where all proxies try to reconnect to host at the same time.
    // Defaults to 0s.
    // +optional
    // +kubebuilder:validation:Pattern=`^(((\d*(\.\d*)?s)|(\d*(\.\d*)?ms))+)$`
    MaxEjectionTimeJitter *string `json:"maxEjectionTimeJitter,omitempty"`
}
```

## Example

If the global configuration is not configured,Below are some configuration examples for outlier detection.

### Example 1    
 ```yaml
apiVersion: projectcontour.io/v1alpha1
kind: HTTPProxy
metadata:
  name: simple-5xx
  namespace: projectcontour
spec:
    virtualhost:
        fqdn: outlierDetection.projectcontour.io
    routes:
      - conditions:
          - prefix: /
        services:
          - name: s1
            port: 80
            outlierDetection:
              consecutiveServerErrors: 5
              maxEjectionPercent: 100
```
In this example, when s1 service experiences 5 consecutive 5xx errors (including locally originated and externally originated (transaction) errors), s1 service will be ejected, with a maximum ejection rate of 100%. Panic is disabled, which is also the default value.

Below is the generated envoy configuration:
```yaml
cluster:
  common_lb_config:
    healthy_panic_threshold:
      value: 0.0
  outlier_detection:
    enforcing_success_rate: 0
    consecutive_5xx: 5
    enforcing_consecutive_5xx: 100
    max_ejection_percent: 100
    enforcing_consecutive_gateway_failure: 0
```

### Example 2
 ```yaml
apiVersion: projectcontour.io/v1alpha1
kind: HTTPProxy
metadata:
  name: only-local-external-origin-errors
  namespace: projectcontour
spec:
    virtualhost:
        fqdn: outlierDetection.projectcontour.io
    routes:
      - conditions:
          - prefix: /
        services:
          - name: s1
            port: 80
            outlierDetection:
              consecutiveServerErrors: 0
              maxEjectionPercent: 100
              splitExternalLocalOriginErrors: true
              consecutiveLocalOriginFailure: 5
```
In this example, when the s1 service has 5 consecutive local origin errors, the s1 service will be ejected, and the 5 xx errors returned by the upstream service will not be counted in the error statistics. The maximum eviction ratio is 100%. Panic is disabled, which is also the default value.

Below is the generated envoy configuration:
```yaml
cluster:
  common_lb_config:
    healthy_panic_threshold:
      value: 0.0
  outlier_detection:
    enforcing_success_rate: 0
    enforcing_consecutive_5xx: 0
    max_ejection_percent: 100
    split_external_local_origin_errors: true
    consecutive_local_origin_failure: 5
    #Default to 100% if not specified
    #enforcing_consecutive_local_origin_failure: 100
    enforcing_local_origin_success_rate: 0
    enforcing_consecutive_gateway_failure: 0
```

### Example 3
 ```yaml
apiVersion: projectcontour.io/v1alpha1
kind: HTTPProxy
metadata:
  name: local-external-origin-errors-and-consecutive-5xx
  namespace: projectcontour
spec:
    virtualhost:
        fqdn: outlierDetection.projectcontour.io
    routes:
      - conditions:
          - prefix: /
        services:
          - name: s1
            port: 80
            outlierDetection:
              consecutiveServerErrors: 10
              maxEjectionPercent: 100
              splitExternalLocalOriginErrors: true
              consecutiveLocalOriginFailure: 5
```
In this example, when the s1 service continuously encounters 5 local origin errors or 10 external errors, the s1 service will be ejected. The maximum eviction rate is 100%. Panic is disabled, which is also the default value.

Below is the generated envoy configuration:
```yaml
cluster:
  common_lb_config:
    healthy_panic_threshold:
      value: 0.0
  outlier_detection:
    enforcing_success_rate: 0
    consecutive_5xx: 10
    enforcing_consecutive_5xx: 100
    max_ejection_percent: 100
    split_external_local_origin_errors: true
    consecutive_local_origin_failure: 5
    #Default to 100% if not specified
    #enforcing_consecutive_local_origin_failure: 100
    enforcing_local_origin_success_rate: 0
    enforcing_consecutive_gateway_failure: 0
``` 

Notes:
- If consecutiveServerErrors is specified as 0, and splitExternalLocalOriginErrors is true, then local errors will be ignored.This is especially useful when the upstream service explicitly returns a 5xx for some requests and you want to ignore those responses from upstream service while determining the outlier detection status of a host.
- When accessing the upstream host through an opaque TCP connection, connection timeouts, connection errors, and request failures are all considered 5xx errors, and therefore these events are included in the 5xx error statistics.
- Please refer to the [official documentation of Envoy][1] for more instructions.


## Open Issues
- Whether the existing supported configuration options meet the needs of the user.
- Should a global switch be provided to record [ejection event logs][2]?

[1]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/outlier
[2]: https://www.envoyproxy.io/docs/envoy/latest/api-v3/config/bootstrap/v3/bootstrap.proto#envoy-v3-api-field-config-bootstrap-v3-clustermanager-outlier-detection
