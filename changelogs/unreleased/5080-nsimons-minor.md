## Allow Disabling Features

The `contour serve` command takes a new optional flag, `--disable-feature`, that allows disabling
certain features.

Currently this flag can be used to disable the informer for ExtensionService resources,
effectively making the ExtensionService CRD optional in the cluster.
To do this, use the flag as follows: `--disable-feature=extensionservices`
