# Contour Configuration CRD

Currently, Contour gets its configuration from two different places, one is the configuration file represented as a Kubernetes configmap.
The other are flags which are passed to Contour.

Contour's configmap configuration file has grown to the point where moving to a CRD will enable a better user experience as well as allowing Contour to react to changes in its configuration faster.

This design proposes two new CRDs, one that represents a `ContourConfiguration` (Short name `ContourConfig`) and another which represents a `ContourDeployment`, both which are namespaced.
The Contour configuration mirrors how the configmap functions today. 
A Contour Deployment is managed by a controller (aka Operator) which uses the details of the CRD spec to deploy a fully managed instance of Contour inside a cluster.

## Benefits
- Eliminates the need to translate from a CRD to a configmap (Like the Operator does today)
- Allows for a place to surface information about configuration errors - the CRD status, in addition to the Contour log files
- Allows the Operator workflow to match a non-operator workflow (i.e. you start with a Contour configuration CRD)
- Provides better validation to avoid errors in the configuration file
- Dynamic restarting of Contour when configuration changes

## New CRD Spec

The contents of current configuration file has grown over time and some fields need to be better categorized.
- Any field that was not previously `camelCased` will be renamed to match.
- All the non-startup required command flags have been moved to the configuration CRD.
- New groupings of similar fields have been created to make the file flow better.

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: ContourConfiguration
metadata:
  name: contour
spec:
  xdsServer:
    type: contour
    address: 0.0.0.0
    port: 8001
    insecure: false
    tls:
      caFile: 
      certFile:
      keyFile:
  ingress:
    className: contour
    statusAddress: local.projectcontour.io
  debug:
    address: 127.0.0.1
    port: 6060
    logLevel: Info 
    kubernetesLogLevel: 0
  health:
    address: 0.0.0.0
    port: 8000
  metrics:
    address: 0.0.0.0
    port: 8002    
  envoy:
    listener:
      useProxyProtocol: false
      disableAllowChunkedLength: false
      connectionBalancer: exact
      tls:
        minimumProtocolVersion: "1.2"
        cipherSuites:
          - '[ECDHE-ECDSA-AES128-GCM-SHA256|ECDHE-ECDSA-CHACHA20-POLY1305]'
          - '[ECDHE-RSA-AES128-GCM-SHA256|ECDHE-RSA-CHACHA20-POLY1305]'
          - 'ECDHE-ECDSA-AES256-GCM-SHA384'
          - 'ECDHE-RSA-AES256-GCM-SHA384'
    service:
      name: contour
      namespace: projectcontour
    http: 
      address: 0.0.0.0
      port: 80
      accessLog: /dev/STDOUT
    https:
      address: 0.0.0.0
      port: 443
      accessLog: /dev/STDOUT
    metrics:
      address: 0.0.0.0
      port: 8002
    clientCertificate:
      name: envoy-client-cert-secret-name
      namespace: projectcontour
    network:
      numTrustedHops: 0
      adminPort: 9001
    managed:
      networkPublishing:
      nodePlacement:
        nodeSelector:
        tolerations:
    logging:
      accessLogFormat: envoy
      accessLogFormatString: "...\n"
      jsonFields:
        - <Fields Omitted)
    defaultHTTPVersions:
      - "HTTP/2"
      - "HTTP/1.1"
    timeouts:
      requestTimeout: infinity
      connectionIdleTimeout: 60s
      streamIdleTimeout: 5m
      maxConnectionDuration: infinity
      delayedCloseTimeout: 1s
      connectionShutdownGracePeriod: 5s
    cluster:
      dnsLookupFamily: auto
  gateway:
      controllerName: projectcontour.io/projectcontour/contour
  httpproxy:
    disablePermitInsecure: false
    rootNamespaces: 
      - foo
      - bar
    fallbackCertificate:
      name: fallback-secret-name
      namespace: projectcontour
  leaderElection:
    configmap:
      name: leader-elect
      namespace: projectcontour
    disableLeaderElection: false
  enableExternalNameService: false
  rateLimitService:
    extensionService: projectcontour/ratelimit
    domain: contour
    failOpen: false
    enableXRateLimitHeaders: false
  policy:
    requestHeaders:
      set:
    responseHeaders:
      set:
status:
```

## Converting from Configmap

Contour will provide a way internally to move to the new CRD and not require users to manually migrate to the new CRD format.
Contour will provide a new command or external tool (similar to ir2proxy) which will migrate between the specs accordingly. 

## Contour Deployment

A managed version of Contour was made available with the `Contour Operator`.
Since Contour will manage Envoy instances, the Operator will now manage instances of Contour.
The details of how an instance of Contour should be deployed within a cluster will be defined in the second CRD named `ContourDeployment`. 
The `spec.configuration` of this object will be the same struct defined in the `ContourConfiguration`. 

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
    <same config as above>
status:
```

## Processing Logic

Contour will require a new flag (`--contour-config`), which will allow for customizing the name of the `ContourConfiguration` CRD that is it to use.
It will default to one named `contour`, but could be overridden if desired.
The ContourConfiguration referenced must also be in the same namespace as Contour is running, it's not valid to reference a configuration from another namespace.
The current flag `--config-path`/`-c` will continue to point to the Configmap file, but over time could eventually be deprecated and the short code `-c` be used for the CRD location (i.e. `--contour-config`) for simplicity.
The Contour configuration CRD will still remain optional.
In its absence, Contour will operate with reasonable defaults.
Where Contour settings can also be specified with command-line flags, the command-line value takes precedence over the configuration file.

On startup, Contour will look for a `ContourConfiguration` CRD named `contour` or using the value of the flag `--contour-config` in the same namespace which Contour is running in, Contour won't support referencing from a different namespace.
If the `ContourConfiguration` CRD is not found Contour will start up with reasonable defaults.

Contour will set status on the object informing the user if there are any errors or issues with the config file or that is all fine and processing correctly.
If the configuration file is not valid, Contour will not start up its controllers, and will fail its readiness probe.
Once the configuration is valid again, Contour will start its controllers with the valid configuration.

Once Contour begins using a Configuration CRD, it will add a finalizer to it such that if that resource is going to get deleted, Contour is aware of it.
Should the Configuration CRD be deleted while it is in use, Contour will default back to reasonable defaults and log the issue.

When config in the CRD changes we will gracefully stop the dependent ingress/gateway controllers and restart them with new config, or dynamically update some in-memory data that the controllers use.
Contour will first validate the new Configuration, ff that new change set results in the object being invalid, Contour will stop its Controller and will become not ready, and not serve any xDS traffic.
As soon as the configuration does become valid, Contour will start up its controllers and begin processing as normal.

### Initial Implementation

Contour will initially start implementation by restarting the Contour pod and allowing Kubernetes to restart itself when the config file changes.
Should the configuration be invalid, Contour will start up, set status on the ContourConfig CRD and then crash.
Kubernetes will crash-loop until the configuration is valid, however, due to the nature of the exponential backoff, updates to the Configuration CRD will not be realized until the next restart loop, or Contour is restarted manually. 

## Versioning

Initially, the CRDs will use the `v1alpha1` api group to allow for changes to the specs before they are released as a full `v1` spec. 
It's possible that we find issues along the way developing the new controllers and migrating to this new configuration method. 
Having a way to change the spec should we need to will be helpful on the path to a full v1 version. 

Once we get to `v1` we have hard compatibility requirements - no more breaking changes without a major version rev.
This should result in increased review scrutiny for proposed changes to the CRD spec.