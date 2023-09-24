## Fix bug with algorithm used to sort Envoy regex/prefix path rules

Envoy is greedy matching path routes and as a result order is important. Contour to produce consistent routing tables in the precence of HTTPProxy inclusion and Ingress Conformance sorts the routes before updating Envoy.

This changes fixes the sorting algorithm used for `Prefix` and `Regex` based path matching. Previously the algorithm lexicographically sorted the regex routes instead of sorting them based on the length of the `Prefix`|`Regex`.

### How to update safely

Caution is advised if you update Contour and you are operating large routing tables. We advise you to:

1. Deploy a duplicate Contour installation that parses the same CRDs
2. Port-forward to the Envoy admin interface [docs](https://projectcontour.io/docs/v1.3.0/troubleshooting/)
3. Access `http://127.0.0.1:9001/config_dump` and compare the configuration of Envoy. In particular the routes and their order.

