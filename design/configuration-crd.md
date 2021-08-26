# Contour Configuration CRD

Currently, Contour gets its configuration from two different places, one is the configuration file represented as a Kubernetes configmap.
The other are flags which are passed to Contour.

Contour's configmap configuration file has grown to the point where moving to a CRD will enable a better user experience as well as allowing Contour to react to changes in its configuration faster.

This design proposes two new CRDs, one that represents a `ContourConfig` and another which represents a `ContourDeployment`.
The Contour configuration mirrors how the configmap functions today. 
A Contour Deployment is managed by a controller (aka Operator) which uses the details of the CRD spec to deploy a fully managed instance of Contour inside a cluster.

## Benefits
- Eliminates the need to translate from a CRD to a configmap (Like the Operator does today)
- Allows for a place to surface information about configuration errors - the CRD status, in addition to the Contour log files
- Allows the Operator workflow to match a non-operator workflow (i.e. you start with a Contour configuration CRD)
- Provides better validation to avoid errors in the configuration file

## Contour Configuration

The Contour configuration file wil be migrated into a Contour Configuration CRD named `Contour`.
The current config file looks like this:

```yaml
server:
  xds-server-type: contour
gateway:
  controllerName: projectcontour.io/projectcontour/contour
incluster: true
kubeconfig: /path/to/.kube/config
disableAllowChunkedLength: false
disablePermitInsecure: false
tls:
  minimum-protocol-version: "1.2"
  cipher-suites:
  - '[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]'
  - '[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]'
  - 'ECDHE-ECDSA-AES256-GCM-SHA384'
  - 'ECDHE-RSA-AES256-GCM-SHA384'
fallback-certificate:
  name: fallback-secret-name
  namespace: projectcontour
envoy-client-certificate:
  name: envoy-client-cert-secret-name
  namespace: projectcontour
leaderelection:
  configmap-name: leader-elect
  configmap-namespace: projectcontour
enableExternalNameService: false
accesslog-format: envoy
accesslog-format-string: "...\n"
json-fields:
  - <Fields Omitted)
default-http-versions:
  - "HTTP/2"
  - "HTTP/1.1"
timeouts:
  request-timeout: infinity
  connection-idle-timeout: 60s
  stream-idle-timeout: 5m
  max-connection-duration: infinity
  delayed-close-timeout: 1s
  connection-shutdown-grace-period: 5s
cluster:
  dns-lookup-family: auto
network:
  num-trusted-hops: 0
  admin-port: 9001
rateLimitService:
  extensionService: projectcontour/ratelimit
  domain: contour
  failOpen: false
  enableXRateLimitHeaders: false
policy:
  request-headers:
    set:
  response-headers:
    set:
```

The contents of this file has grown over time and some fields need to be better categorized. 
The new ContourConfiguration CRD will move the logging options into a `logging` struct. 
All other fields will stay the same where they currently exist in the configmap.

### New fields:

Some fields will be moved around and others added to complete the configuration:
- logging: The logging configuration bits are now moved into a common struct
- envoy: This manages the Envoy configuration of how it should be deployed and exposed from the cluster
- ingressClassName: Adds the `ingress-class-name` flag to the configuration file

## New CRD Spec

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ContourConfiguration
metadata:
  name: contour
spec:
  server:
    xds-server-type: contour
  ingressClassName: contour
  envoy:  
    networkPublishing:
    nodePlacement:
      nodeSelector:
      tolerations:
  gateway:
      controllerName: projectcontour.io/projectcontour/contour
  incluster: true
  kubeconfig: /path/to/.kube/config
  disableAllowChunkedLength: false
  disablePermitInsecure: false
  tls:
    minimum-protocol-version: "1.2"
    cipher-suites:
      - '[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]'
      - '[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]'
      - 'ECDHE-ECDSA-AES256-GCM-SHA384'
      - 'ECDHE-RSA-AES256-GCM-SHA384'
  fallback-certificate:
    name: fallback-secret-name
    namespace: projectcontour
  envoy-client-certificate:
    name: envoy-client-cert-secret-name
    namespace: projectcontour
  leaderelection:
    configmap-name: leader-elect
    configmap-namespace: projectcontour
  enableExternalNameService: false
  logging: 
    accesslog-format: envoy
    accesslog-format-string: "...\n"
    json-fields:
      - <Fields Omitted)
  default-http-versions:
    - "HTTP/2"
    - "HTTP/1.1"
  timeouts:
    request-timeout: infinity
    connection-idle-timeout: 60s
    stream-idle-timeout: 5m
    max-connection-duration: infinity
    delayed-close-timeout: 1s
    connection-shutdown-grace-period: 5s
  cluster:
    dns-lookup-family: auto
  network:
    num-trusted-hops: 0
    admin-port: 9001
  rateLimitService:
    extensionService: projectcontour/ratelimit
    domain: contour
    failOpen: false
    enableXRateLimitHeaders: false
  policy:
    request-headers:
      set:
    response-headers:
      set:
status:
```

## Contour Deployment

A managed version of Contour was made available with the `Contour Operator`.
Since Contour will manage Envoy instances, the Operator will now manage instances of Contour.
The details of how an instance of Contour should be deployed within a cluster will be defined in the second CRD named `ContourDeployment`. 

A controller will watch for these objects to be created and take action on them accordingly to make desired state in the cluster match the configuration on the spec. 

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ContourDeployment
metadata:
  name: contour
spec:
  replicas: 2
  nodePlacement:
    nodeSelector:
    tolerations:
  configuration:
    server:
      xds-server-type: contour
    ingressClassName: contour
    envoy:
      networkPublishing:
      nodePlacement:
        nodeSelector:
        tolerations:
    gateway:
        controllerName: projectcontour.io/projectcontour/contour
    incluster: true
    kubeconfig: /path/to/.kube/config
    disableAllowChunkedLength: false
    disablePermitInsecure: false
    tls:
      minimum-protocol-version: "1.2"
      cipher-suites:
      - '[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]'
      - '[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]'
      - 'ECDHE-ECDSA-AES256-GCM-SHA384'
      - 'ECDHE-RSA-AES256-GCM-SHA384'
    fallback-certificate:
      name: fallback-secret-name
      namespace: projectcontour
    envoy-client-certificate:
      name: envoy-client-cert-secret-name
      namespace: projectcontour
    leaderelection:
      configmap-name: leader-elect
      configmap-namespace: projectcontour
    enableExternalNameService: false
    logging: 
      accesslog-format: envoy
      accesslog-format-string: "...\n"
      json-fields:
        - <Fields Omitted)
    default-http-versions:
      - "HTTP/2"
      - "HTTP/1.1"
    timeouts:
      request-timeout: infinity
      connection-idle-timeout: 60s
      stream-idle-timeout: 5m
      max-connection-duration: infinity
      delayed-close-timeout: 1s
      connection-shutdown-grace-period: 5s
    cluster:
      dns-lookup-family: auto
    network:
      num-trusted-hops: 0
      admin-port: 9001
    rateLimitService:
      extensionService: projectcontour/ratelimit
      domain: contour
      failOpen: false
      enableXRateLimitHeaders: false
    policy:
      request-headers:
        set:
      response-headers:
        set:
status:
```

## Versioning

Initially, the CRDs will use the `v1alpha1` api group to allow for changes to the specs before they are released as a full `v1` spec. 
It's possible that we find issues along the way developing the new controllers and migrating to this new configuration method. 
Having a way to change the spec should we need to will be helpful on the path to a full v1 version. 