## Replace GRPC state of world updates with ADS Delta

The mechanism used for xDS updates between Contour and Envoy has been changed from standard `GRPC` (aka "state of the world" updates) to
[Aggregated Discovery Service](https://www.envoyproxy.io/docs/envoy/latest/configuration/overview/xds_api#aggregated-discovery-service) (ADS) `Delta GRPC` updates. This change is a part of the ongoing effort to improve the performance and scalability of Contour.
With this mechanism in place Contour will now send only the changes to Envoy, instead of the entire state of the world on every update. Additionally, using ADS allows all the communication to be delivered on a single, bidirectional gRPC stream. 
Together these changes drastically reduce the cpu and memory footprint of Contour and Envoy, and improve the overall performance of the system allowing for greater numbers of HttpProxies in a single cluster.
