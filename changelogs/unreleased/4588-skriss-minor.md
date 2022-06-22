## Gateway API: implement HTTP query parameter matching

Contour now implements Gateway API's [HTTP query parameter matching](https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1alpha2.HTTPQueryParamMatch).
Only `Exact` matching is supported.
For example, the following HTTPRoute will send a request with a query string of `?animal=whale` to `s1`, and a request with a querystring of `?animal=dolphin` to `s2`.

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: HTTPRoute
metadata:
  name: httproute-queryparam-matching
spec:
  parentRefs:
  - name: contour-gateway
  rules:
  - matches:
    - queryParams:
      - type: Exact
        name: animal
        value: whale
    backendRefs:
    - name: s1
  - matches:
    - queryParams:
      - type: Exact
        name: animal
        value: dolphin
    backendRefs:
    - name: s2
```
