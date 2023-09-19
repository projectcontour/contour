# Thrift Proxy Design

# Abstract

Thrift Proxy is an L7 Envoy proxy that is used for routing Thrift protocol. 
This document outlines the rationale, objectives, architectural design, and technical details of integrating Thrift Proxy with Contour. 
The integration of Thrift Proxy into Contour opens up new possibilities for routing and managing Thrift-based applications.


# Background

The Thrift Proxy is an essential component in the Envoy ecosystem, offering the ability to route Thrift protocol traffic at the application layer (Layer 7). 
This functionality is particularly valuable for organizations that rely on the Thrift Protocol as their primary RPC implementation.

Key features of the Envoy Thrift Proxy include:

* Route Matching: The Thrift Proxy enables granular route matching based on various criteria, such as the Thrift method name, Thrift service and name, and a set of headers. 
This level of specificity ensures accurate routing of Thrift requests to their intended destinations.
* Rate Limiting: It supports rate limit filtering within the Thrift filter chain.
* Cluster Management: The Thrift Proxy offers support for weighted clusters and mirrored clusters, enabling efficient load balancing and fail-over strategies.
* Route Discovery: It facilitates route discovery mechanisms, making it easier to dynamically configure and manage Thrift routing rules.

### Contour's Current Capabilities

As of the current Contour release, the project provides "TCP Proxying" functionality. 
While this is valuable, it does not include native support for Thrift Proxying.

### The Need for Thrift Proxy in Contour

The absence of Thrift Proxy support in Contour creates limitations for organizations that heavily rely on the Thrift Protocol. 
As the Thrift Protocol continues to be a popular choice for inter-service communication, integrating Thrift Proxy into Contour is a logical step forward. 
This integration will address the following key needs:
* Protocol Support: Adding Thrift Proxy support ensures that Contour can seamlessly route and manage Thrift-based applications alongside its existing capabilities for HTTP and gRPC traffic.
* Enhanced Routing Control: With Thrift Proxy, Contour gains the ability to implement fine-grained routing logic for Thrift requests, enabling intelligent routing decisions based on Thrift-specific criteria.
* Interoperability: Thrift Proxy ensures that Contour can easily integrate with systems and services utilizing the Thrift Protocol.

By introducing Thrift Proxy support into Contour, we aim to elevate Contour's versatility, making it a more comprehensive solution for modern, distributed application architectures.

## Goals

* Implement an equivalent to HTTPProxy, called ThriftProxy, which allows users to define their Thrift routing logic and services: This goal aims to provide users with a dedicated resource, ThriftProxy, 
where they can specify custom Thrift routing rules, enabling fine-grained control over how Thrift traffic is handled within Contour.
* Add a new Listener for the Thrift protocol: This goal involves enhancing Contour's capabilities by introducing a new Listener specifically designed to handle Thrift protocol traffic.
* Implement ThriftRoute discovery: ThriftRoute discovery mechanisms will allow for dynamic configuration and management of Thrift routing rules.

## Non Goals

* Thrift protocol will be only supported by the ThriftProxy CRD, excluding support for K8s ingresses: In this iteration, the focus is solely on the ThriftProxy CRD for defining Thrift routing logic. 
K8s ingresses and API Gateway support are intentionally excluded from this project to maintain a clear scope.
* This iteration doesn't include API Gateway support: While this project introduces ThriftProxy to enhance Contour's Thrift routing capabilities, it does not encompass the development of a full-fledged API Gateway. 
API Gateway functionality may be considered in future iterations.

# Thrift Proxy Components
Before delving into the high-level design, it's essential to understand the key components that play a crucial role in achieving the project goals. 
The Thrift Proxy comprises several components, each serving a specific purpose in enabling Thrift protocol routing within Contour.

## Thrift Proxy
`extensions.filters.network.thrift_proxy.v3.ThriftProxy`  
The Thrift Proxy, like any Envoy proxy, defines various configuration elements, including route configurations, filters, access logs, and the Thrift route discovery service. 
Each instance of the Thrift Proxy corresponds to one Thrift RouteConfiguration configuration.

## Thrift RouteConfiguration
`extensions.filters.network.thrift_proxy.v3.RouteConfiguration`  
The Thrift RouteConfiguration is responsible for defining routes and their corresponding rules within the Thrift filter. It can be identified by a unique name used for route discovery. 
Each RouteConfiguration includes an array of Thrift routes.

## Thrift Routes
Thrift Routes are defined within RouteConfigurations and represent specific routing rules for Thrift traffic. 
Each Thrift route consists of two main entries: the Thrift `RouteMatch` and the Thrift `RouteAction`.

### Thrift RouteMatch
`extensions.filters.network.thrift_proxy.v3.RouteMatch`  
Thrift RouteMatches are essential for defining matching criteria for Thrift routes. 
Each Thrift route **must** have a matching rule, which can be based on either the Thrift `service_name` or the Thrift `method_name`, but not both. 
Additionally, the `RouteMatch` supports an array of `config.route.v3.HeaderMatcher` for additional matching conditions.

### Thrift RouteAction
`extensions.filters.network.thrift_proxy.v3.RouteAction`  
Thrift RouteActions specify the Thrift cluster responsible for handling incoming requests. 
They offer advanced features like weighted clusters, request mirror policies, and rate limits.

## Thrift Route Discovery Service(TRDS)
`extensions.filters.network.thrift_proxy.v3.Trds`  
The Thrift Route Discovery Service (TRDS) serves as a critical component for Thrift route discovery. 
It is currently compatible with Aggregated Service Discovery (ADS) and is responsible for fetching specific route configurations. 
TRDS identifies the desired route configuration by name, allowing for the use of different configurations as needed.

# High-Level Design

The implementation is organized into four essential areas:
### ThriftProxy CRD
The foundation of our design lies in the creation of the ThriftProxy CRD, empowering users to define intricate routing rules and specify the services they require. 
By introducing this Custom Resource Definition, we provide a straightforward and flexible means for users to configure Thrift routing within Contour.
### Thrift Listener
A dedicated Thrift Listener is responsible for receiving incoming Thrift traffic on a distinct port separate from gRPC and HTTP protocols.
### Thrift RouteConfiguration
Thrift RouteConfiguration defines the Envoy configurations that correspond to user-defined routing rules.
### Aggregated Service Discovery
Aggregated Service Discovery (ADS) to enable Thrift route discovery.

# Detailed Design

## ThriftProxy CRD
The ThriftProxy CRD is designed to simplify the configuration of Thrift routing. 
Users can define routing rules specific to their application's needs, ensuring that Thrift traffic is efficiently directed to the appropriate services within the cluster.
```yaml
apiVersion: projectcontour.io/v1
kind: ThriftProxy
metadata:
  name: thriftproxy-1
  namespace: projectcontour
spec:
  routes:
    - condition:
        methodName: <method_name>
        headers:
          - name: <some_header>
            exact: <value>
      services:
        - name: <cluster_name>
          port: <port_number>
```

### Example
```yaml
apiVersion: projectcontour.io/v1
kind: ThriftProxy
metadata:
  name: placeorder
  namespace: placeorder-ns
spec:
  routes:
    - condition:
        methodName: "placeOrder"
        headers:
          - name: "Authorization"
            exact: "Bearer token"
      services:
        - name: "order-service"
          port: 9090

```

## Thrift Listener
When a new ThriftProxy resource is discovered by Contour, a corresponding Thrift Listener is automatically added to the Envoy configuration. 
This listener is configured with default settings to ensure seamless integration into Contour.
### Default Settings:
* Name: The Thrift Listener is named "ingress_thrift" for identification within the Envoy configuration.
* Protocol: The protocol is set to "thrift" to differentiate Thrift traffic from other protocols.
* Address and Port: The address and port are configurable, allowing users to specify the desired listening address and port number, similar to how the HTTP listener operates (default: :9090)
* RouteConfigName: The Thrift Listener is associated with a default Route Configuration named "ingress_thrift.".

The Thrift Listener includes a Thrift filter with essential configurations:
```
&envoy_listener_v3.Filter{
    StatPrefix: name,
    AccessLog:  accesslogger,
    Trds: &envoy_thrift_v3.Trds{
        RouteConfigName: name,
        ConfigSource: &envoy_core_v3.ConfigSource{
            ResourceApiVersion: envoy_core_v3.ApiVersion_V3,
            ConfigSourceSpecifier: &envoy_core_v3.ConfigSource_Ads{
                Ads: &envoy_core_v3.AggregatedConfigSource{},
            },
        },
    },
}
```

## Thrift RouteConfiguration
A Thrift RouteConfiguration is essentially an organized collection of Thrift routes.
Thrift routes are defined as follows:
```
&envoy_thrift_v3.Route{
    Match: ThriftRouteMatch(rule),
    Route: ThriftRouteAction(rule),
}
```
* ThriftRouteMatch: Defines the matching rules that incoming Thrift traffic must satisfy to be routed according to this rule. These rules can be based on factors such as the Thrift method name and custom headers.
* ThriftRouteAction: Specifies the cluster that should handle the Thrift traffic that matches the given rule. This cluster configuration includes details such as load balancing policies, fail-over strategies, and rate limiting.

## Aggregated Service Discovery
Aggregated Service Discovery (ASD) is a dynamic resource management mechanism that allows Envoy to fetch configuration resources from multiple sources, 
aggregate them, and ensure that they are consistently and efficiently distributed to proxy instances. In the context of our project, ASD serves as the backbone of Thrift route discovery.

### Enabling Aggregated Service Discovery
To enable Aggregated Service Discovery for Thrift route discovery, the following steps are required:
1. Add `AdsConfig` to `DynamicResources`:
    Within Contour's configuration, an `AdsConfig` entry must be added to the bootstrapped `DynamicResources`. This configuration tells Envoy what is the Aggregated Discovery Service (ADS).

2. Implement `StreamAggregatedResources`
   The core implementation step involves defining the `StreamAggregatedResource`s function. This function establishes a connection to the ADS and handles the streaming of aggregated resources.
    ```
    func (s *contourServer) StreamAggregatedResources(srv envoy_service_discovery_v3.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
        return s.stream(srv)
    }
    ```

# Other concerns
### Release Strategy
One of the primary concerns surrounding the introduction of Thrift support is the size and complexity of the anticipated pull request. 
Given the potential size of this PR, a thoughtful release strategy is essential to ensure a smooth integration of the Thrift support feature into the existing codebase.
#### Beta Release
To address this concern, the proposed strategy is to release Thrift support as a "beta" feature. During the "beta" phase, efforts will be focused on stabilizing and enhancing Thrift support based on user feedback and usage patterns.
### End-to-End Testing
Thrift, being a binary protocol, adds complexity to the E2E testing process. Unlike HTTP-based testing, where tools and libraries are readily available, Thrift testing often requires custom code or specialized testing libraries.

# Alternatives Considered
### Add ThriftProxy to HTTPProxy
#### Pros:
* Shorter Implementation: Combining ThriftProxy with HTTPProxy could potentially lead to a shorter and more straightforward implementation process.
#### Cons:
* Risk of Regression: Introducing a significant feature like Thrift support within an existing proxy structure could pose a risk of regression, impacting the stability of existing HTTPProxy functionality.
* Semantic Misalignment: Thrift and HTTP are distinct protocols with different semantics. Combining them under a single proxy structure may lead to semantic confusion and misalignment.
* Differing Underlying Structures: Thrift Routes and HTTP filter routes are based on different underlying Envoy structures, which could complicate the integration and introduce compatibility challenges.
