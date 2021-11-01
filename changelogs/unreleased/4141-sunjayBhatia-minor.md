### Source IP hash based load balancing

Contour users can now configure their load balancing policies on `HTTPProxy` resources to hash the source IP of a client to ensure consistent routing to a backend service instance. Using this feature combined with header value hashing can implement advanced request routing and session affinity. Note that if you are using a load balancer to front your Envoy deployment, you will need to ensure it preserves client source IP addresses to ensure this feature is effective.

See [this page](https://projectcontour.io/docs/v1.20.0/config/request-routing/#load-balancing-strategy) for more details on this feature.