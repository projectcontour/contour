## HTTPProxy: implement HTTP query parameter matching

Contour now implements HTTP query parameter matching for HTTPProxy
resource-based routes. It supports `Exact`, `Prefix`, `Suffix`, `Regex` and
`Contains` string matching conditions together with the `IgnoreCase` modifier
and also the `Present` matching condition.
For example, the following HTTPProxy will route requests based on the configured
condition examples for the given query parameter `search`:

```yaml
apiVersion: projectcontour.io/v1
kind: HTTPProxy
metadata:
  name: httpproxy-queryparam-matching
spec:
  routes:
    - conditions:
      - queryParam:
          # will match e.g. '?search=example' as is
          name: search
          exact: example
      services:
        - name: s1
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=prefixthis' or any string value prefixed by `prefix` (case insensitive)
          name: search
          prefix: PreFix
          ignoreCase: true
      services:
        - name: s2
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=thispostfix' or any string value suffixed by `postfix` (case sensitive)
          name: search
          suffix: postfix
      services:
        - name: s3
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=regularexp123' or any string value matching the given regular expression
          name: search
          regex: ^regular.*
      services:
        - name: s4
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=somethinginsideanother' or any string value containing the substring 'inside' (case sensitive)
          name: search
          contains: inside
      services:
        - name: s5
          port: 80
    - conditions:
      - queryParam:
          # will match e.g. '?search=' or any string value given to the named parameter
          name: search
          present: true
      services:
        - name: s6
          port: 80
```