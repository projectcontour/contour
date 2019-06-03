# Prometheus

Contour and Envoy expose metrics that can be scraped with Prometheus. By
default, annotations to gather them are in all the `deployment` yamls and they
should work out of the box with most configurations.

## Envoy Metrics

Envoy typically exposes metrics through an endpoint on its admin interface. To
avoid exposing the entire admin interface to Prometheus (and other workloads in
the cluster), Contour configures a static listener that sends traffic to the
stats endpoint and nowhere else.

Envoy supports Prometheus-compatible `/stats/prometheus` endpoint for metrics on
port `8002`.

## Contour Metrics

Contour exposes a Prometheus-compatible `/metrics` endpoint on port `8000` with
the following metrics:

- **contour_ingressroute_total (gauge):** Total number of IngressRoutes objects that exist regardless of status (i.e. Valid / Invalid / Orphaned, etc). This metric should match the sum of `Orphaned` + `Valid` + `Invalid` IngressRoutes.
  - namespace
- **contour_ingressroute_orphaned_total (gauge):**  Number of `Orphaned` IngressRoute objects which have no root delegating to them
  - namespace
- **contour_ingressroute_root_total (gauge):**  Number of `Root` IngressRoute objects (Note: There will only be a single `Root` IngressRoute per vhost)
  - namespace
- **contour_ingressroute_valid_total (gauge):**  Number of `Valid` IngressRoute objects
  - namespace
  - vhost
- **contour_ingressroute_invalid_total (gauge):**  Number of `Invalid` IngressRoute objects
  - namespace
  - vhost
- **contour_ingressroute_dagrebuild_timestamp (gauge):** Timestamp of the last DAG rebuild

## Sample Deployment

In the `/examples` directory there are example deployment files that can be used to spin up an example environment.
All deployments there are configured with annotations for prometheus to scrape by default, so it should be possible to utilize any of them with the following quickstart example instructions.

### Deploy Prometheus

A sample deployment of Prometheus and Alertmanager is provided that uses temporary storage. This deployment can be used for testing and development, but might not be suitable for all environments.

#### Stateful Deployment

 A stateful deployment of Prometheus should use persistent storage with [Persistent Volumes and Persistent Volume Claims](https://kubernetes.io/docs/concepts/storage/persistent-volumes/) to maintain a correlation between a data volume and the Prometheus Pod. 
 Persistent volumes can be static or dynamic and depends on the backend storage implementation utilized in environment in which the cluster is deployed. For more information, see the [Kubernetes documentation on types of persistent volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#types-of-persistent-volumes).

#### Quick start

```sh
# Deploy 
$ kubectl apply -f examples/prometheus
```

#### Access the Prometheus web UI

```sh
$ kubectl -n contour-monitoring port-forward $(kubectl -n contour-monitoring get pods -l app=prometheus -l component=server -o jsonpath='{.items[0].metadata.name}') 9090:9090
```

then go to [http://localhost:9090](http://localhost:9090) in your browser.

#### Access the Alertmanager web UI

```sh
$ kubectl -n contour-monitoring port-forward $(kubectl -n contour-monitoring get pods -l app=prometheus -l component=alertmanager -o jsonpath='{.items[0].metadata.name}') 9093:9093
```

then go to [http://localhost:9093](http://localhost:9093) in your browser.

### Deploy Grafana

A sample deployment of Grafana is provided that uses temporary storage.

#### Quick start

```sh
# Deploy
$ kubectl apply -f examples/grafana/

# Create secret with grafana credentials
$ kubectl create secret generic grafana -n contour-monitoring \
    --from-literal=grafana-admin-password=admin \
    --from-literal=grafana-admin-user=admin
```

#### Access the Grafana UI

```sh
$ kubectl port-forward $(kubectl get pods -l app=grafana -n contour-monitoring -o jsonpath='{.items[0].metadata.name}') 3000 -n contour-monitoring
```

then go to [http://localhost:3000](http://localhost:3000) in your browser.
The username and password are from when you defined the Grafana secret in the previous step.
