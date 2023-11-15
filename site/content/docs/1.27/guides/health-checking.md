---
title: Health Checking
---

Contour exposes two health endpoints `/health` and `/healthz`. By default these paths are serviced by `0.0.0.0:8000` and are configurable using the `--health-address` and `--health-port` flags.

e.g. `--health-port 9999` would create a health listener of `0.0.0.0:9999`

**Note:** the `Service` deployment manifest when installing Contour must be updated to represent the same port as the above configured flags.

The health endpoints perform a connection to the Kubernetes cluster's API.
