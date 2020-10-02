// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	envoy_api_v2_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	"github.com/envoyproxy/go-control-plane/pkg/server/v2"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/debug"
	"github.com/projectcontour/contour/internal/health"
	"github.com/projectcontour/contour/internal/httpsvc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/projectcontour/contour/internal/xds"
	"github.com/projectcontour/contour/internal/xdscache"
	xdscache_v2 "github.com/projectcontour/contour/internal/xdscache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Add RBAC policy to support leader election.
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=create;get;update

// Add RBAC policy to support getting CRDs.
// +kubebuilder:rbac:groups="apiextensions.k8s.io",resources=customresourcedefinitions,verbs=list

// registerServe registers the serve subcommand and flags
// with the Application provided.
func registerServe(app *kingpin.Application) (*kingpin.CmdClause, *serveContext) {
	serve := app.Command("serve", "Serve xDS API traffic.")

	// The precedence of configuration for contour serve is as follows:
	// config file, overridden by env vars, overridden by cli flags.
	// however, as -c is a cli flag, we don't know its value til cli flags
	// have been parsed. To correct this ordering we assign a post parse
	// action to -c, then parse cli flags twice (see main.main). On the second
	// parse our action will return early, resulting in the precedence order
	// we want.
	var (
		configFile string
		parsed     bool
	)
	ctx := newServeContext()

	parseConfig := func(_ *kingpin.ParseContext) error {
		if parsed || configFile == "" {
			// if there is no config file supplied, or we've
			// already parsed it, return immediately.
			return nil
		}
		f, err := os.Open(configFile)
		if err != nil {
			return err
		}
		defer f.Close()
		dec := yaml.NewDecoder(f)
		dec.SetStrict(true)
		parsed = true
		if err := dec.Decode(&ctx); err != nil {
			return fmt.Errorf("failed to parse contour configuration: %w", err)
		}
		return nil
	}

	serve.Flag("config-path", "Path to base configuration.").Short('c').Action(parseConfig).ExistingFileVar(&configFile)

	serve.Flag("incluster", "Use in cluster configuration.").BoolVar(&ctx.InCluster)
	serve.Flag("kubeconfig", "Path to kubeconfig (if not in running inside a cluster).").StringVar(&ctx.Kubeconfig)

	serve.Flag("xds-address", "xDS gRPC API address.").StringVar(&ctx.xdsAddr)
	serve.Flag("xds-port", "xDS gRPC API port.").IntVar(&ctx.xdsPort)

	serve.Flag("stats-address", "Envoy /stats interface address.").StringVar(&ctx.statsAddr)
	serve.Flag("stats-port", "Envoy /stats interface port.").IntVar(&ctx.statsPort)

	serve.Flag("debug-http-address", "Address the debug http endpoint will bind to.").StringVar(&ctx.debugAddr)
	serve.Flag("debug-http-port", "Port the debug http endpoint will bind to.").IntVar(&ctx.debugPort)

	serve.Flag("http-address", "Address the metrics HTTP endpoint will bind to.").StringVar(&ctx.metricsAddr)
	serve.Flag("http-port", "Port the metrics HTTP endpoint will bind to.").IntVar(&ctx.metricsPort)
	serve.Flag("health-address", "Address the health HTTP endpoint will bind to.").StringVar(&ctx.healthAddr)
	serve.Flag("health-port", "Port the health HTTP endpoint will bind to.").IntVar(&ctx.healthPort)

	serve.Flag("contour-cafile", "CA bundle file name for serving gRPC with TLS.").Envar("CONTOUR_CAFILE").StringVar(&ctx.caFile)
	serve.Flag("contour-cert-file", "Contour certificate file name for serving gRPC over TLS.").Envar("CONTOUR_CERT_FILE").StringVar(&ctx.contourCert)
	serve.Flag("contour-key-file", "Contour key file name for serving gRPC over TLS.").Envar("CONTOUR_KEY_FILE").StringVar(&ctx.contourKey)
	serve.Flag("insecure", "Allow serving without TLS secured gRPC.").BoolVar(&ctx.PermitInsecureGRPC)
	serve.Flag("root-namespaces", "Restrict contour to searching these namespaces for root ingress routes.").StringVar(&ctx.rootNamespaces)

	serve.Flag("ingress-class-name", "Contour IngressClass name.").StringVar(&ctx.ingressClass)
	serve.Flag("ingress-status-address", "Address to set in Ingress object status.").StringVar(&ctx.IngressStatusAddress)
	serve.Flag("envoy-http-access-log", "Envoy HTTP access log.").StringVar(&ctx.httpAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log.").StringVar(&ctx.httpsAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests.").StringVar(&ctx.httpAddr)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests.").StringVar(&ctx.httpsAddr)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests.").IntVar(&ctx.httpPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests.").IntVar(&ctx.httpsPort)
	serve.Flag("envoy-service-name", "Envoy Service Name.").StringVar(&ctx.EnvoyServiceName)
	serve.Flag("envoy-service-namespace", "Envoy Service Namespace.").StringVar(&ctx.EnvoyServiceNamespace)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners.").BoolVar(&ctx.useProxyProto)

	serve.Flag("accesslog-format", "Format for Envoy access logs.").StringVar(&ctx.AccessLogFormat)
	serve.Flag("disable-leader-election", "Disable leader election mechanism.").BoolVar(&ctx.DisableLeaderElection)

	serve.Flag("debug", "Enable debug logging.").Short('d').BoolVar(&ctx.Debug)
	serve.Flag("kubernetes-debug", "Enable Kubernetes client debug logging.").UintVar(&ctx.KubernetesDebug)
	serve.Flag("experimental-service-apis", "Subscribe to the new service-apis types.").BoolVar(&ctx.UseExperimentalServiceAPITypes)
	return serve, ctx
}

// validateCRDs inspects all CRDs in the projectcontour.io group and logs a warning
// if they have spec.preserveUnknownFields set to true, since this indicates that they
// were created as v1beta1 and the user has not upgraded them to be fully v1-compatible.
func validateCRDs(dynamicClient dynamic.Interface, log logrus.FieldLogger) {
	client := dynamicClient.Resource(schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"})

	crds, err := client.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Warnf("error listing v1 custom resource definitions: %v", err)
		return
	}

	for _, crd := range crds.Items {
		log = log.WithField("crd", crd.GetName())

		if group, _, _ := unstructured.NestedString(crd.Object, "spec", "group"); group != contour_api_v1.GroupName {
			log.Debugf("CRD is not in projectcontour.io API group, ignoring")
			continue
		}

		preserveUnknownFields, found, err := unstructured.NestedBool(crd.Object, "spec", "preserveUnknownFields")
		if err != nil {
			log.Warnf("error getting CRD's spec.preserveUnknownFields value: %v", err)
			continue
		}
		if found && preserveUnknownFields {
			log.Warnf("CRD was created as v1beta1 since it has spec.preserveUnknownFields set to true; it should be upgraded to v1 per https://projectcontour.io/resources/upgrading/")
			continue
		}

		log.Debugf("CRD is fully v1-compatible since it has spec.preserveUnknownFields set to false")
	}
}

// doServe runs the contour serve subcommand.
func doServe(log logrus.FieldLogger, ctx *serveContext) error {
	// log a warning if the Contour config file doesn't already disable TLS 1.1 and 1.0.
	if annotation.MinTLSVersion(ctx.TLSConfig.MinimumProtocolVersion) <= envoy_api_v2_auth.TlsParameters_TLSv1_1 {
		log.Warn("In Contour 1.10, TLS 1.1 will be disabled by default. HTTPProxies with no Spec.VirtualHost.TLS.MinimumProtocolVersion" +
			" and Ingresses without the projectcontour.io/tls-minimum-protocol-version annotation will default to TLS 1.2. If TLS 1.1" +
			" is required for any of your HTTPProxies or Ingresses going forward, you must explicitly specify 1.1, though we recommend" +
			" against continuing to use TLS 1.1 as it's end-of-life.")
	}

	// Establish k8s core & dynamic client connections.
	clients, err := k8s.NewClients(ctx.Kubeconfig, ctx.InCluster)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// Validate that Contour CRDs have been updated to v1.
	validateCRDs(clients.DynamicClient(), log)

	// Factory for cluster-wide informers.
	clusterInformerFactory := clients.NewInformerFactory()

	// Factories for per-namespace informers.
	namespacedInformerFactories := map[string]k8s.InformerFactory{}

	// Validate fallback certificate parameters.
	fallbackCert, err := ctx.fallbackCertificate()
	if err != nil {
		log.WithField("context", "fallback-certificate").Fatalf("invalid fallback certificate configuration: %q", err)
	}

	// Validate client certificate parameters.
	clientCert, err := ctx.envoyClientCertificate()
	if err != nil {
		log.WithField("context", "envoy-client-certificate").Fatalf("invalid client certificate configuration: %q", err)
	}

	if rootNamespaces := ctx.proxyRootNamespaces(); len(rootNamespaces) > 0 {
		// Add the FallbackCertificateNamespace to the root-namespaces if not already
		if !contains(rootNamespaces, ctx.TLSConfig.FallbackCertificate.Namespace) && fallbackCert != nil {
			rootNamespaces = append(rootNamespaces, ctx.FallbackCertificate.Namespace)
			log.WithField("context", "fallback-certificate").Infof("fallback certificate namespace %q not defined in 'root-namespaces', adding namespace to watch", ctx.FallbackCertificate.Namespace)
		}

		if !contains(rootNamespaces, ctx.TLSConfig.ClientCertificate.Namespace) && clientCert != nil {
			rootNamespaces = append(rootNamespaces, ctx.ClientCertificate.Namespace)
			log.WithField("context", "envoy-client-certificate").Infof("client certificate namespace %q not defined in 'root-namespaces', adding namespace to watch", ctx.ClientCertificate.Namespace)
		}

		for _, ns := range rootNamespaces {
			if _, ok := namespacedInformerFactories[ns]; !ok {
				namespacedInformerFactories[ns] = clients.NewInformerFactoryForNamespace(ns)
			}
		}
	}

	// Set up Prometheus registry and register base metrics.
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	// Before we can build the event handler, we need to initialize the converter we'll
	// use to convert from Unstructured. Thanks to kubebuilder types from service-apis, this now can
	// return an error.
	converter, err := k8s.NewUnstructuredConverter()
	if err != nil {
		return err
	}

	connectionIdleTimeout, err := timeout.Parse(ctx.ConnectionIdleTimeout)
	if err != nil {
		return fmt.Errorf("error parsing connection idle timeout: %w", err)
	}
	streamIdleTimeout, err := timeout.Parse(ctx.StreamIdleTimeout)
	if err != nil {
		return fmt.Errorf("error parsing stream idle timeout: %w", err)
	}
	maxConnectionDuration, err := timeout.Parse(ctx.MaxConnectionDuration)
	if err != nil {
		return fmt.Errorf("error parsing max connection duration: %w", err)
	}
	connectionShutdownGracePeriod, err := timeout.Parse(ctx.ConnectionShutdownGracePeriod)
	if err != nil {
		return fmt.Errorf("error parsing connection shutdown grace period: %w", err)
	}
	requestTimeout, err := getRequestTimeout(log, ctx)
	if err != nil {
		return fmt.Errorf("error parsing request timeout: %w", err)
	}

	listenerConfig := xdscache_v2.ListenerConfig{
		UseProxyProto:                 ctx.useProxyProto,
		HTTPAddress:                   ctx.httpAddr,
		HTTPPort:                      ctx.httpPort,
		HTTPAccessLog:                 ctx.httpAccessLog,
		HTTPSAddress:                  ctx.httpsAddr,
		HTTPSPort:                     ctx.httpsPort,
		HTTPSAccessLog:                ctx.httpsAccessLog,
		AccessLogType:                 ctx.AccessLogFormat,
		AccessLogFields:               ctx.AccessLogFields,
		MinimumTLSVersion:             annotation.MinTLSVersion(ctx.TLSConfig.MinimumProtocolVersion),
		RequestTimeout:                requestTimeout,
		ConnectionIdleTimeout:         connectionIdleTimeout,
		StreamIdleTimeout:             streamIdleTimeout,
		MaxConnectionDuration:         maxConnectionDuration,
		ConnectionShutdownGracePeriod: connectionShutdownGracePeriod,
	}

	defaultHTTPVersions, err := parseDefaultHTTPVersions(ctx.DefaultHTTPVersions)
	if err != nil {
		return fmt.Errorf("failed to configure default HTTP versions: %w", err)
	}

	listenerConfig.DefaultHTTPVersions = defaultHTTPVersions

	contourMetrics := metrics.NewMetrics(registry)

	// Endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	endpointHandler := xdscache_v2.NewEndpointsTranslator(log.WithField("context", "endpointstranslator"))

	resources := []xdscache.ResourceCache{
		xdscache_v2.NewListenerCache(listenerConfig, ctx.statsAddr, ctx.statsPort),
		&xdscache_v2.SecretCache{},
		&xdscache_v2.RouteCache{},
		&xdscache_v2.ClusterCache{},
		endpointHandler,
	}

	// snapshotCache is used to store the state of what all xDS services should
	// contain at any given point in time.
	snapshotCache := cache.NewSnapshotCache(false, xds.DefaultHash,
		log.WithField("context", "xDS"))

	// snapshotHandler is used to produce new snapshots when the internal state changes for any xDS resource.
	snapshotHandler := xdscache.NewSnapshotHandler(snapshotCache, resources, log.WithField("context", "snapshotHandler"))

	// register observer for endpoints updates.
	endpointHandler.Observer = contour.ComposeObservers(snapshotHandler)

	dnsLookupFamily, err := ParseDNSLookupFamily(ctx.DNSLookupFamily)
	if err != nil {
		return fmt.Errorf("failed to configure configuration file parameter cluster.dns-lookup-family: %w", err)
	}

	// Build the core Kubernetes event handler.
	eventHandler := &contour.EventHandler{
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		Observer:        dag.ComposeObservers(append(xdscache.ObserversOf(resources), snapshotHandler)...),
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				RootNamespaces: ctx.proxyRootNamespaces(),
				IngressClass:   ctx.ingressClass,
				FieldLogger:    log.WithField("context", "KubernetesCache"),
			},
			Processors: []dag.Processor{
				&dag.IngressProcessor{
					FieldLogger:       log.WithField("context", "IngressProcessor"),
					ClientCertificate: clientCert,
				},
				&dag.ExtensionServiceProcessor{
					FieldLogger:       log.WithField("context", "ExtensionServiceProcessor"),
					ClientCertificate: clientCert,
				},
				&dag.HTTPProxyProcessor{
					DisablePermitInsecure: ctx.DisablePermitInsecure,
					FallbackCertificate:   fallbackCert,
					DNSLookupFamily:       dnsLookupFamily,
					ClientCertificate:     clientCert,
				},
				&dag.ListenerProcessor{},
			},
		},
		FieldLogger: log.WithField("context", "contourEventHandler"),
	}

	// Log that we're using the fallback certificate if configured.
	if fallbackCert != nil {
		log.WithField("context", "fallback-certificate").Infof("enabled fallback certificate with secret: %q", fallbackCert)
	}

	if clientCert != nil {
		log.WithField("context", "envoy-client-certificate").Infof("enabled client certificate with secret: %q", clientCert)
	}

	// Wrap eventHandler in a converter for objects from the dynamic client.
	// and an EventRecorder which tracks API server events.
	dynamicHandler := &k8s.DynamicClientHandler{
		Next: &contour.EventRecorder{
			Next:    eventHandler,
			Counter: contourMetrics.EventHandlerOperations,
		},
		Converter: converter,
		Logger:    log.WithField("context", "dynamicHandler"),
	}

	// Register our resource event handler with the k8s informers,
	// using the SyncList to keep track of what to sync later.
	var informerSyncList k8s.InformerSyncList

	// Inform on DefaultResources
	informerSyncList.InformOnResources(clusterInformerFactory, dynamicHandler, k8s.DefaultResources()...)

	if ctx.UseExperimentalServiceAPITypes {
		// Check if the resource exists in the API server before setting up the informer.
		if !clients.ResourcesExist(k8s.ServiceAPIResources()...) {
			log.WithField("InformOnResources", "ExperimentalServiceAPITypes").Warnf("resources %v not found in api server", k8s.ServiceAPIResources())
		} else {
			informerSyncList.InformOnResources(clusterInformerFactory, dynamicHandler, k8s.ServiceAPIResources()...)
		}
	}

	// TODO(youngnick): Move this logic out to internal/k8s/informers.go somehow.
	// Add informers for each root namespace
	for _, factory := range namespacedInformerFactories {
		informerSyncList.InformOnResources(factory, dynamicHandler, k8s.SecretsResources()...)
	}

	// If root namespaces are not defined, then add the informer for all namespaces
	if len(namespacedInformerFactories) == 0 {
		informerSyncList.InformOnResources(clusterInformerFactory, dynamicHandler, k8s.SecretsResources()...)
	}

	informerSyncList.InformOnResources(clusterInformerFactory,
		&k8s.DynamicClientHandler{
			Next: &contour.EventRecorder{
				Next:    endpointHandler,
				Counter: contourMetrics.EventHandlerOperations,
			},
			Converter: converter,
			Logger:    log.WithField("context", "endpointstranslator"),
		}, k8s.EndpointsResources()...)

	// Set up workgroup runner and register informers.
	var g workgroup.Group
	g.Add(startInformer(clusterInformerFactory, log.WithField("context", "contourinformers")))

	for ns, factory := range namespacedInformerFactories {
		g.Add(startInformer(factory, log.WithField("context", "corenamespacedinformers").WithField("namespace", ns)))
	}

	// Register our event handler with the workgroup.
	g.Add(eventHandler.Start())

	// Create metrics service and register with workgroup.
	metricsvc := httpsvc.Service{
		Addr:        ctx.metricsAddr,
		Port:        ctx.metricsPort,
		FieldLogger: log.WithField("context", "metricsvc"),
		ServeMux:    http.ServeMux{},
	}

	metricsvc.ServeMux.Handle("/metrics", metrics.Handler(registry))

	if ctx.healthAddr == ctx.metricsAddr && ctx.healthPort == ctx.metricsPort {
		h := health.Handler(clients.ClientSet())
		metricsvc.ServeMux.Handle("/health", h)
		metricsvc.ServeMux.Handle("/healthz", h)
	}

	g.Add(metricsvc.Start)

	// Create a separate health service if required.
	if ctx.healthAddr != ctx.metricsAddr || ctx.healthPort != ctx.metricsPort {
		healthsvc := httpsvc.Service{
			Addr:        ctx.healthAddr,
			Port:        ctx.healthPort,
			FieldLogger: log.WithField("context", "healthsvc"),
		}

		h := health.Handler(clients.ClientSet())
		healthsvc.ServeMux.Handle("/health", h)
		healthsvc.ServeMux.Handle("/healthz", h)

		g.Add(healthsvc.Start)
	}

	// Create debug service and register with workgroup.
	debugsvc := debug.Service{
		Service: httpsvc.Service{
			Addr:        ctx.debugAddr,
			Port:        ctx.debugPort,
			FieldLogger: log.WithField("context", "debugsvc"),
		},
		Builder: &eventHandler.Builder,
	}
	g.Add(debugsvc.Start)

	// Register leadership election.
	eventHandler.IsLeader = setupLeadershipElection(&g, log, ctx, clients, eventHandler.UpdateNow)

	// Once we have the leadership detection channel, we can
	// push DAG rebuild metrics onto the observer stack.
	eventHandler.Observer = &contour.RebuildMetricsObserver{
		Metrics:      contourMetrics,
		IsLeader:     eventHandler.IsLeader,
		NextObserver: eventHandler.Observer,
	}

	sh := k8s.StatusUpdateHandler{
		Log:             log.WithField("context", "StatusUpdateHandler"),
		Clients:         clients,
		LeaderElected:   eventHandler.IsLeader,
		Converter:       converter,
		InformerFactory: clusterInformerFactory,
	}
	g.Add(sh.Start)

	// Now we have the statusUpdateHandler, we can create the event handler's StatusUpdater, which will take the
	// status updates from the DAG, and send them to the status update handler.
	eventHandler.StatusUpdater = sh.Writer()

	// Set up ingress load balancer status writer.
	lbsw := loadBalancerStatusWriter{
		log:           log.WithField("context", "loadBalancerStatusWriter"),
		clients:       clients,
		isLeader:      eventHandler.IsLeader,
		lbStatus:      make(chan corev1.LoadBalancerStatus, 1),
		ingressClass:  ctx.ingressClass,
		statusUpdater: sh.Writer(),
		Converter:     converter,
	}
	g.Add(lbsw.Start)

	// Register an informer to watch envoy's service if we haven't been given static details.
	if ctx.IngressStatusAddress == "" {
		dynamicServiceHandler := &k8s.DynamicClientHandler{
			Next: &k8s.ServiceStatusLoadBalancerWatcher{
				ServiceName: ctx.EnvoyServiceName,
				LBStatus:    lbsw.lbStatus,
				Log:         log.WithField("context", "serviceStatusLoadBalancerWatcher"),
			},
			Converter: converter,
			Logger:    log.WithField("context", "serviceStatusLoadBalancerWatcher"),
		}
		factory := clients.NewInformerFactoryForNamespace(ctx.EnvoyServiceNamespace)
		informerSyncList.InformOnResources(factory, dynamicServiceHandler, k8s.ServicesResources()...)

		g.Add(startInformer(factory, log.WithField("context", "serviceStatusLoadBalancerWatcher")))
		log.WithField("envoy-service-name", ctx.EnvoyServiceName).
			WithField("envoy-service-namespace", ctx.EnvoyServiceNamespace).
			Info("Watching Service for Ingress status")
	} else {
		log.WithField("loadbalancer-address", ctx.IngressStatusAddress).Info("Using supplied information for Ingress status")
		lbsw.lbStatus <- parseStatusFlag(ctx.IngressStatusAddress)
	}

	g.Add(func(stop <-chan struct{}) error {
		log := log.WithField("context", "xds")

		log.Printf("waiting for informer caches to sync")
		if err := informerSyncList.WaitForSync(stop); err != nil {
			return err
		}
		log.Printf("informer caches synced")

		var grpcServer *grpc.Server

		switch ctx.XDSServerType {
		case "contour":
			grpcServer = xds.RegisterServer(
				xds.NewContourServer(log, xdscache.ResourcesOf(resources)...),
				registry,
				ctx.grpcOptions(log)...)
		case "envoy":
			grpcServer = xds.RegisterServer(
				server.NewServer(context.Background(), snapshotCache, nil),
				registry,
				ctx.grpcOptions(log)...)
		default:
			log.Fatalf("invalid xdsServerType %q configured", ctx.XDSServerType)
		}

		addr := net.JoinHostPort(ctx.xdsAddr, strconv.Itoa(ctx.xdsPort))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		log = log.WithField("address", addr)
		if ctx.PermitInsecureGRPC {
			log = log.WithField("insecure", true)
		}

		log.Info("started xDS server")
		defer log.Info("stopped xDS server")

		go func() {
			<-stop

			// We don't use GracefulStop here because envoy
			// has long-lived hanging xDS requests. There's no
			// mechanism to make those pending requests fail,
			// so we forcibly terminate the TCP sessions.
			grpcServer.Stop()
		}()

		return grpcServer.Serve(l)
	})

	// Set up SIGTERM handler for graceful shutdown.
	g.Add(func(stop <-chan struct{}) error {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		select {
		case sig := <-c:
			log.WithField("context", "sigterm-handler").WithField("signal", sig).Info("shutting down")
		case <-stop:
			// Do nothing. The group is shutting down.
		}
		return nil
	})

	// GO!
	return g.Run(context.Background())
}

func contains(namespaces []string, ns string) bool {
	for _, namespace := range namespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

func startInformer(inf k8s.InformerFactory, log logrus.FieldLogger) func(stop <-chan struct{}) error {
	return func(stop <-chan struct{}) error {
		log.Println("started informer")
		defer log.Println("stopped informer")
		inf.Start(stop)
		<-stop
		return nil
	}
}

// getRequestTimeout gets the request timeout setting from ctx.TimeoutConfig.RequestTimeout
// if it's set, or else ctx.RequestTimeoutDeprecated if it's set, or else a default setting.
func getRequestTimeout(log logrus.FieldLogger, ctx *serveContext) (timeout.Setting, error) {
	if ctx.RequestTimeout != "" {
		return timeout.Parse(ctx.RequestTimeout)
	}
	if ctx.RequestTimeoutDeprecated > 0 {
		log.Warn("The request-timeout field in the Contour config file is deprecated and will be removed in a future release. Use timeout-config.request-timeout instead.")
		return timeout.DurationSetting(ctx.RequestTimeoutDeprecated), nil
	}

	return timeout.DefaultSetting(), nil
}
