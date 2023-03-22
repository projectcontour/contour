## HTTPProxy: Internal Redirect support

Contour now supports specifying an `internalRedirectPolicy` on a `Route` to handle 3xx redirects internally, that is capturing a configurable 3xx redirect response, synthesizing a new request,
sending it to the upstream specified by the new route match,
and returning the redirected response as the response to the original request. 
