## Gateway API: handle Route conflicts with HTTPRoute.Matches

It's possible that multiple HTTPRoutes will define the same Match conditions. In this case the following logic is applied to resolve the conflict:

- The oldest Route based on creation timestamp. For example, a Route with a creation timestamp of “2020-09-08 01:02:03” is given precedence over a Route with a creation timestamp of “2020-09-08 01:02:04”.
- The Route appearing first in alphabetical order (namespace/name) for example, foo/bar is given precedence over foo/baz.

With above ordering, any HTTPRoute that ranks lower is marked with Conflict condition.
