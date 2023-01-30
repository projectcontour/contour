## Fix handling of duplicate HTTPProxy Include Conditions

Duplicate include conditions are now correctly identified and HTTPProxies are marked with the condition `IncludeError` and reason `DuplicateMatchConditions`.
Previously the HTTPProxy processor was only comparing adjacent includes and comparing conditions element by element rather than as a whole, ANDed together.

In addition, the previous behavior when duplicate Include Conditions were identified was to throw out all routes, including valid ones, on the offending HTTPProxy.
Any referenced child HTTPProxies were marked as `Orphaned` as a result, even if they were included correctly.
With this change, all valid Includes and Route rules are processed and programmed in the data plane, which is a difference in behavior from previous releases.
An Include is deemed to be a duplicate if it has the exact same match Conditions as an Include that precedes it in the list.
Only child HTTPProxies that are referenced by a duplicate Include and not in any other valid Include are marked as `Orphaned`

### Caveat for empty or individual prefix path matches on `/`

A caveat to the above, is that an empty list of include conditions or a set of conditions that only consist of the prefix match on `/` are not treated as duplicates.

This special case has been added because many users rely on the behavior this enables and many Contour examples demonstrating inclusion actually use it.
For example:

```
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example
spec:
  virtualhost:
    fqdn: foo-example.bar.com
  includes:
  - name: example-child1
  - name: example-child2
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-child1
spec:
  routes:
    - conditions:
      - prefix: /
      services:
      - name: s1
        port: 80
---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: example-child2
spec:
  routes:
  - conditions:
    - prefix: /foo
    services:
    - name: s2
      port: 80
```

While the include conditions are equivalent, the resulting routing rules when the child routes are taken into account are distinct.

This special casing is a stop-gap for this release, to ensure we do not break user's configuration that is currently valid and working.

### Future changes to inclusion and route duplicate detection

Currently duplicate route conditions are not checked in an HTTPProxy include tree or within an individual HTTPProxy.
This means that you can have routes listed later in the list of routes on an HTTPProxy silently override others.
The same can happen if you have an include tree that generates duplicate routes based on the include conditions and route conditions.

If you are relying on this behavior, changes will be coming in the next Contour release.

We will be submitting a design document to address this as it will be a significant behavior change and encourage the community to weigh in.
The current plan is to fully validate duplicate route match conditions as they are generated from the tree of includes and routes.
There will likely be changes to status conditions set on HTTPRoutes to improve reporting such invalid configuration.
