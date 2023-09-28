## Fix bug with algorithm used to sort Envoy regex/prefix path rules

Envoy greedy matches routes and as a result the order route matches are presented to Envoy is important. Contour attempts to produce consistent routing tables so that the most specific route matches are given preference. This is done to facilitate consistency when using HTTPProxy inclusion and provide a uniform user experience for route matching to be inline with Ingress and Gateway API Conformance.

This changes fixes the sorting algorithm used for `Prefix` and `Regex` based path matching. Previously the algorithm lexicographically sorted based on the path match string instead of sorting them based on the length of the `Prefix`|`Regex`. i.e. Longer prefix/regexes will be sorted first in order to give preference to more specific routes, then lexicographic sorting for things of the same length.

Note that for prefix matching, this change is _not_ expected to change the relative ordering of more specific prefixes vs. less specific ones when the more specific prefix match string has the less specific one as a prefix, e.g. `/foo/bar` will continue to sort before `/foo`. However, relative ordering of other combinations of prefix matches may change per the above description.
### How to update safely

Caution is advised if you update Contour and you are operating large routing tables. We advise you to:

1. Deploy a duplicate Contour installation that parses the same CRDs
2. Port-forward to the Envoy admin interface [docs](https://projectcontour.io/docs/v1.3.0/troubleshooting/)
3. Access `http://127.0.0.1:9001/config_dump` and compare the configuration of Envoy. In particular the routes and their order. The prefix routes might be changing in order, so if they are you need to verify that the route matches as expected.

