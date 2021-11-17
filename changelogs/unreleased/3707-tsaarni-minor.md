### Metrics over HTTPS

Both Envoy and Contour metrics can now be served over HTTPS.
Server can alternatively also require client to present certificate which is validated against configured CA certificate.
This feature makes it possible to limit the visibility of metrics to authorized clients.
