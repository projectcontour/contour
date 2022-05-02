## Query parameter hash based load balancing

Contour users can now configure their load balancing policies on `HTTPProxy` resources to hash the query parameter on a request to ensure consistent routing to a backend service instance.

See [this page](https://projectcontour.io/docs/v1.21.0/config/request-routing/#load-balancing-strategy) for more details on this feature.

Credit to @pkit for implementing this feature!
