# Overload Manager

Envoy uses heap memory when processing requests.
When the system runs out of memory or memory resource limit for the container is reached, Envoy process is terminated abruptly.
To avoid this, Envoy [overload manager][1] can be enabled.
Overload manager controls how much memory Envoy will allocate at maximum and what actions it takes when the limit is reached.

Overload manager is disabled by default.
It can be enabled at deployment time by using `--overload-max-heap=[MAX_BYTES]` command line flag in [`contour bootstrap`][2] command.
The bootstrap command is executed in [init container of Envoy pod][3] to generate initial configuration for Envoy.
To enable overload manager, modify the deployment manifest and add for example `--overload-max-heap=2147483648` to set maximum heap size to 2 GiB.
The appropriate number of bytes can be different from system to system.

After the feature is enabled, following two overload actions are configured to Envoy:

* Shrink heap action is executed when 95% of the maximum heap size is reached.
* Envoy will stop accepting requests when 98% of the maximum heap size is reached.

When requests are denied due to high memory pressure, `503 Service Unavailable` will be returned with a response body containing text `envoy overloaded`.
Shrink heap action will try to free unused heap memory, eventually allowing requests to be processed again.

**NOTE:**
The side effect of overload is that Envoy will deny also requests `/ready` and `/stats` endpoints.
This is due to the way how Contour secures Envoy's admin API and exposes only selected admin API endpoints by proxying itself.
When readiness probe fails, the overloaded Envoy will be removed from the list of service endpoints.
If the maximum heap size is set too low, Envoy may be unable to free enough memory and never become ready again.

[1]: https://www.envoyproxy.io/docs/envoy/latest/configuration/operations/overload_manager/overload_manager
[2]: ../configuration#bootstrap-flags
[3]: https://github.com/projectcontour/contour/blob/cbec8eca9e8b639318588c5aa7ec0b5b751938c5/examples/render/contour.yaml#L5204-L5216
