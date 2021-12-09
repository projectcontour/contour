### Add authBeforeRateLimiting option to HTTPProxy's authorizationServer

A new field, `authBeforeRateLimiting`, has been added to the `authorizationServer` block in `HTTPProxy`. When set to true, external auth will occur *before* any local or global rate limiting. When left as false (the default), external auth will occur *after* both local and global rate limiting.

Note that, as part of introducing this field, global rate limiting has been moved to occur before external auth by default. Previously, local rate limiting occurred before external auth, but global rate limiting occurred after external auth. Both types of rate limiting are now consistently either before or after external auth, based on the value of this new field.
