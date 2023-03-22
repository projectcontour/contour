## Watching specific namespaces

The `contour serve` command takes a new optional flag, `--watch-namespaces`, that can
be used to restrict the namespaces where the Contour instance watches for resources.
Consequently, resources in other namespaces will not be known to Contour and will not
be acted upon.

You can watch a single or multiple namespaces, and you can further restrict the root
namespaces with `--root-namespaces` just like before. Root namespaces must be a subset
of the namespaces being watched, for example:

`--watch-namespaces=my-admin-namespace,my-app-namespace --root-namespaces=my-admin-namespace`

If the `--watch-namespaces` flag is not used, then all namespaces will be watched by default.
