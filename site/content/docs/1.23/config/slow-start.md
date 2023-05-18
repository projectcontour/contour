# Slow Start Mode

Slow start mode is a configuration setting that is used to gradually increase the amount of traffic targeted to a newly added upstream endpoint.
By default, the amount of traffic will increase linearly for the duration of time window set by `window` field, starting from 10% of the target load balancing weight and increasing to 100% gradually.
The easing function for the traffic increase can be adjusted by setting optional field `aggression`.
A value above 1.0 results in a more aggressive increase initially, slowing down when nearing the end of the time window.
Value below 1.0 results in slow initial increase, picking up speed when nearing the end of the time window.
Optional field `minWeightPercent` can be set to change the minimum percent of target weight.
It is used to avoid too small new weight, which may cause endpoint to receive no traffic in beginning of the slow start window.

Slow start mode can be useful for example with JVM based applications, that might otherwise get overwhelmed during JIT warm-up period.
Such applications may respond to requests slowly or return errors immediately after pod start or after container restarts.
User impact of this behavior can be mitigated by using slow start configuration to gradually increase traffic to recently started service endpoints.

The following example configures slow start mode for a service:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: slow-start
spec:
  virtualhost:
    fqdn: www.example.com
  routes:
    - services:
        - name: java-app
          port: 80
          slowStartPolicy:
            window: 3s
            aggression: "1.0"
            minWeightPercent: 10
```

Slow start mode works only with `RoundRobin` and `WeightedLeastRequest` [load balancing strategies][2].
For more details see [Envoy documentation][1].

[1]: https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/slow_start
[2]: api/#projectcontour.io/v1.LoadBalancerPolicy
