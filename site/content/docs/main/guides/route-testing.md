---
title: Route Testing
---

This guide provides a walkthrough for testing routes managed by Contour using Envoy's [route check tool][1]. It covers the installation of the route check tool, generating Envoy routes from Contour configurations, and verifying these routes via assertions.

## Installing the envoy route check tool

The tool can be built locally from source using bazel by using the below command.
```bash
bazel build //test/tools/router_check:router_check_tool
copy router_check_tool /usr/local/bin/
```

You can ensure router check is installed by running
```bash
router_check_tool --version
```

## Generating Envoy routes from Contour
The Contour routegen tool requires Kubernetes manifests as input to generate envoy routes. The manifest should contain atleast one of APIGateway, HTTPProxy or an Ingress along with their referencing Kubernetes services.

A sample manifest is given below

### manifest.yaml
```yaml
---
apiVersion: v1
kind: Service
metadata:
  name: s1
  namespace: service-a
spec:
  selector:
    app.kubernetes.io/name: test
  ports:
  - protocol: TCP
    port: 80
    targetPort: 8080

---
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: proxy-a
  namespace: service-a
spec:
  virtualhost:
    fqdn: foo-basic.bar.com
  routes:
    - conditions:
      - prefix: /
      services:
        - name: s1
          port: 80
```

You can generate Envoy routes by using the following Contour command

```bash
$ contour routegen --output=output.yaml test.yaml
INFO[0000] Generating envoy routes for resources defined in ["manifest.yaml"]
```
### output.json
```json
{
  "name": "ingress_http",
  "virtualHosts": [
    {
      "name": "foo-basic.bar.com",
      "domains": [
        "foo-basic.bar.com"
      ],
      "routes": [
        {
          "match": {
            "prefix": "/"
          },
          "route": {
            "cluster": "service-a/s1/80"
          }
        }
      ]
    }
  ],
  "requestHeadersToAdd": [
    {
      "header": {
        "key": "x-request-start",
        "value": "t=%START_TIME(%s.%3f)%"
      }
    }
  ],
  "ignorePortInHostMatching": true
}
```

### Flags
#### Custom Contour Config
You can pass in the path to the contour config file by using the flag `--config-path=</path/to/file>` to generate config specific envoy routes.

#### Ingress classes
You can also pass in the ingress class name to generate configs associated with a particular class of contour controller by using the flag `--ingress-class-name`.


## Running generated routes against route check tool

The above generated routes need to be run against a an envoy test suite. More details and documentation can be found in the envoy [docs][2].

Below is a sample test suite to run against our generated routes from the sample manifests.

### tests.yaml
```yaml
---
tests:
- test_name: foo.bar
  input:
    authority: foo-basic.bar.com
    path: "/hello"
    method: GET
    additional_request_headers:
      - key: x-param
        value: test
  validate:
    # namespace/service_name/port
    cluster_name: service-a/s1/80
```

Run the tests against the generated routes

```bash
$ router_check_tool -c output.json -t tests.yaml --details --detailed-coverage
foo.bar
Current route coverage: 100%
```

The envoy tool provides many knobs and flags to customise the tests such as coverage, coverages thresholds etc. More details can be found in the Envoy [docs][1].

[1]: https://www.envoyproxy.io/docs/envoy/latest/operations/tools/route_table_check_tool
[2]: https://www.envoyproxy.io/docs/envoy/latest/configuration/operations/tools/router_check
