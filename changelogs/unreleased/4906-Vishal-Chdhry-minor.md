## Enable configuring Server header transformation 

Envoy's treatment of the Server header on responses can now be configured in the Contour config file or ContourConfiguration CRD.
When configured as `OVERWRITE`, Envoy overwrites any Server header with "envoy".
When configured as `APPEND_IF_ABSENT`, ⁣if a Server header is present, Envoy will pass it through, otherwise, it will set it to "envoy".
When configured as `PASS_THROUGH`, Envoy passes through the value of the Server header and does not append a header if none is present.