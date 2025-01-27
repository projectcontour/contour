## xDS server type fields in config file and ContourConfiguration CRD and legacy `contour` xDS server are removed

Contour now uses a go-control-plane-based xDS server.
The legacy `contour` xDS server that pre-dates `go-control-plane` has been removed.
Since there is now only one supported xDS server, the config fields for selecting an xDS server implementation have been removed.