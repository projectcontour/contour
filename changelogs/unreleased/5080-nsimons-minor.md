## Allow Disabling Features

The `contour serve` command takes a new optional flag, `--disable-feature`, that allows disabling
certain features.

The flag is used to disable the informer for a custom resource, effectively making the corresponding
CRD optional in the cluster. You can provide the flag multiple times.

Current options include `extensionservices` and the experimental Gateway API features `tlsroutes` and
`grpcroutes`.

For example, to disable ExtensionService CRD, use the flag as follows: `--disable-feature=extensionservices`.
