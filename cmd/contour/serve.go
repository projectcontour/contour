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
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/debug"
	cgrpc "github.com/projectcontour/contour/internal/grpc"
	"github.com/projectcontour/contour/internal/health"
	"github.com/projectcontour/contour/internal/httpsvc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

// Add RBAC policy to support leader election.
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=create;get;update

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
		parsed = true
		return dec.Decode(&ctx)
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
	serve.Flag("experimental-service-apis", "Subscribe to the new service-apis types.").BoolVar(&ctx.UseExperimentalServiceAPITypes)
	return serve, ctx
}

// doServe runs the contour serve subcommand.
func doServe(log logrus.FieldLogger, ctx *serveContext) error {

	// step 1. establish k8s core & dynamic client connections
	clients, err := k8s.NewClients(ctx.Kubeconfig, ctx.InCluster)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// step 2. create informer factories

	// Factory for cluster-wide informers.
	clusterInformerFactory := clients.NewInformerFactory()

	// Factories for per-namespace informers.
	namespacedInformerFactories := map[string]k8s.InformerFactory{}

	// Validate fallback certificate parameters
	fallbackCert, err := ctx.fallbackCertificate()
	if err != nil {
		log.WithField("context", "fallback-certificate").Fatalf("invalid fallback certificate configuration: %q", err)
	}

	if rootNamespaces := ctx.proxyRootNamespaces(); len(rootNamespaces) > 0 {
		// Add the FallbackCertificateNamespace to the root-namespaces if not already
		if !contains(rootNamespaces, ctx.TLSConfig.FallbackCertificate.Namespace) && fallbackCert != nil {
			rootNamespaces = append(rootNamespaces, ctx.FallbackCertificate.Namespace)
			log.WithField("context", "fallback-certificate").Infof("fallback certificate namespace %q not defined in 'root-namespaces', adding namespace to watch", ctx.FallbackCertificate.Namespace)
		}

		for _, ns := range rootNamespaces {
			if _, ok := namespacedInformerFactories[ns]; !ok {
				namespacedInformerFactories[ns] = clients.NewInformerFactoryForNamespace(ns)
			}
		}
	}

	// setup prometheus registry and register base metrics.
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

	listenerConfig := contour.ListenerVisitorConfig{
		UseProxyProto:         ctx.useProxyProto,
		HTTPAddress:           ctx.httpAddr,
		HTTPPort:              ctx.httpPort,
		HTTPAccessLog:         ctx.httpAccessLog,
		HTTPSAddress:          ctx.httpsAddr,
		HTTPSPort:             ctx.httpsPort,
		HTTPSAccessLog:        ctx.httpsAccessLog,
		AccessLogType:         ctx.AccessLogFormat,
		AccessLogFields:       ctx.AccessLogFields,
		MinimumTLSVersion:     annotation.MinTLSVersion(ctx.TLSConfig.MinimumProtocolVersion),
		RequestTimeout:        ctx.RequestTimeout,
		ConnectionIdleTimeout: ctx.ConnectionIdleTimeout,
		StreamIdleTimeout:     ctx.StreamIdleTimeout,
		MaxConnectionDuration: ctx.MaxConnectionDuration,
		DrainTimeout:          ctx.DrainTimeout,
	}

	defaultHTTPVersions, err := parseDefaultHTTPVersions(ctx.DefaultHTTPVersions)
	if err != nil {
		return fmt.Errorf("failed to configure default HTTP versions: %w", err)
	}

	listenerConfig.DefaultHTTPVersions = defaultHTTPVersions

	// step 3. build our mammoth Kubernetes event handler.
	eventHandler := &contour.EventHandler{
		CacheHandler: &contour.CacheHandler{
			ListenerVisitorConfig: listenerConfig,
			ListenerCache:         contour.NewListenerCache(ctx.statsAddr, ctx.statsPort),
			FieldLogger:           log.WithField("context", "CacheHandler"),
			Metrics:               metrics.NewMetrics(registry),
		},
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				RootNamespaces: ctx.proxyRootNamespaces(),
				IngressClass:   ctx.ingressClass,
				FieldLogger:    log.WithField("context", "KubernetesCache"),
			},
			DisablePermitInsecure: ctx.DisablePermitInsecure,
		},
		FieldLogger: log.WithField("context", "contourEventHandler"),
	}

	// Set the fallback certificate if configured.
	if fallbackCert != nil {
		log.WithField("context", "fallback-certificate").Infof("enabled fallback certificate with secret: %q", fallbackCert)
		eventHandler.FallbackCertificate = fallbackCert
	}

	// wrap eventHandler in a converter for objects from the dynamic client.
	// and an EventRecorder which tracks API server events.
	dynamicHandler := &k8s.DynamicClientHandler{
		Next: &contour.EventRecorder{
			Next:    eventHandler,
			Counter: eventHandler.Metrics.EventHandlerOperations,
		},
		Converter: converter,
		Logger:    log.WithField("context", "dynamicHandler"),
	}

	// step 4. register our resource event handler with the k8s informers,
	// using the SyncList to keep track of what to sync later.
	var informerSyncList k8s.InformerSyncList

	informerSyncList.InformOnResources(clusterInformerFactory, dynamicHandler, k8s.DefaultResources()...)

	if ctx.UseExperimentalServiceAPITypes {
		// Check if the resource exists in the API server before setting up the informer.
		if !clients.ResourceExists(k8s.ServiceAPIResources()...) {
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

	// step 5. endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	et := &contour.EndpointsTranslator{
		FieldLogger: log.WithField("context", "endpointstranslator"),
	}

	informerSyncList.InformOnResources(clusterInformerFactory,
		&k8s.DynamicClientHandler{
			Next: &contour.EventRecorder{
				Next:    et,
				Counter: eventHandler.Metrics.EventHandlerOperations,
			},
			Converter: converter,
			Logger:    log.WithField("context", "endpointstranslator"),
		}, k8s.EndpointsResources()...)

	// step 6. setup workgroup runner and register informers.
	var g workgroup.Group
	g.Add(startInformer(clusterInformerFactory, log.WithField("context", "contourinformers")))

	for ns, factory := range namespacedInformerFactories {
		g.Add(startInformer(factory, log.WithField("context", "corenamespacedinformers").WithField("namespace", ns)))
	}

	// step 7. register our event handler with the workgroup
	g.Add(eventHandler.Start())

	// step 8. create metrics service and register with workgroup.
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

	// step 9. create a separate health service if required.
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

	// step 10. create debug service and register with workgroup.
	debugsvc := debug.Service{
		Service: httpsvc.Service{
			Addr:        ctx.debugAddr,
			Port:        ctx.debugPort,
			FieldLogger: log.WithField("context", "debugsvc"),
		},
		Builder: &eventHandler.Builder,
	}
	g.Add(debugsvc.Start)

	// step 11. register leadership election.
	eventHandler.IsLeader = setupLeadershipElection(&g, log, ctx, clients, eventHandler.UpdateNow)

	sh := k8s.StatusUpdateHandler{
		Log:           log.WithField("context", "StatusUpdateWriter"),
		Clients:       clients,
		LeaderElected: eventHandler.IsLeader,
		Converter:     converter,
	}
	g.Add(sh.Start)

	// Now we have the statusUpdateWriter, we can create the StatusWriter, which will take the
	// status updates from the DAG, and send them to the status update handler.
	eventHandler.StatusClient = &k8s.StatusWriter{
		Updater: sh.Writer(),
	}

	// step 11. set up ingress load balancer status writer
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

	// step 12. register an informer to watch envoy's service if we haven't been given static details.
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
		log := log.WithField("context", "grpc")

		log.Printf("waiting for informer caches to sync")
		if err := informerSyncList.WaitForSync(stop); err != nil {
			return err
		}
		log.Printf("informer caches synced")

		resources := map[string]cgrpc.Resource{
			eventHandler.CacheHandler.ClusterCache.TypeURL():  &eventHandler.CacheHandler.ClusterCache,
			eventHandler.CacheHandler.RouteCache.TypeURL():    &eventHandler.CacheHandler.RouteCache,
			eventHandler.CacheHandler.ListenerCache.TypeURL(): &eventHandler.CacheHandler.ListenerCache,
			eventHandler.CacheHandler.SecretCache.TypeURL():   &eventHandler.CacheHandler.SecretCache,
			et.TypeURL(): et,
		}
		opts := ctx.grpcOptions()
		s := cgrpc.NewAPI(log, resources, registry, opts...)
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
			s.Stop()
		}()

		return s.Serve(l)
	})

	// step 14. Setup SIGTERM handler
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
	return g.Run()
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
