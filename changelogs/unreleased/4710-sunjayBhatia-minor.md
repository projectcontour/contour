## HTTPProxy CORS policy supports regex matching on Allowed Origins

The AllowOrigin field of the HTTPProxy CORSPolicy can be configured as a regex to enable more flexibility for users.
More advanced matching can now be performed on the `Origin` header of HTTP requests, instead of restricting users to allow all origins, or enumerating all possible values.
