## All ContourConfiguration CRD fields are now optional

To better manage configuration defaults, all `ContourConfiguration` CRD fields are now optional without defaults.
Instead, Contour itself will apply defaults to any relevant fields that have not been specified by the user when it starts up, similarly to how processing of the Contour `ConfigMap` works today.
The default values that Contour uses are documented in the `ContourConfiguration` CRD's API documentation.
