### feature(httpproxy): Enhance retry policy

Enhances retry policy with retry_host_predicate which allows users to eject the failing host from Envoy retries.

This pattern is described in https://www.envoyproxy.io/docs/envoy/latest/faq/load_balancing/transient_failures.html as a way to have more reliable retries.

