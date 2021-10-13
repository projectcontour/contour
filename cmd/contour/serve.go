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
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	envoy_server_v3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/controller"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/debug"
	"github.com/projectcontour/contour/internal/health"
	"github.com/projectcontour/contour/internal/httpsvc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/projectcontour/contour/internal/xds"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	ctrl_cache "sigs.k8s.io/controller-runtime/pkg/cache"
	controller_config "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

// Add RBAC policy to support leader election.
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=create;get;update

// registerServe registers the serve subcommand and flags
// with the Application provided.
func registerServe(app *kingpin.Application) (*kingpin.CmdClause, *serveContext) {
	serve := app.Command("serve", "Serve xDS API traffic.")

	// The precedence of configuration for contour serve is as follows:
	// If ContourConfiguration resource is specified, it takes precedence,
	// otherwise config file, overridden by env vars, overridden by cli flags.
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

		if ctx.contourConfigurationName != "" && configFile != "" {
			return fmt.Errorf("cannot specify both %s and %s", "--contour-config", "-c/--config-path")
		}

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

		params, err := config.Parse(f)
		if err != nil {
			return err
		}

		if err := params.Validate(); err != nil {
			return fmt.Errorf("invalid Contour configuration: %w", err)
		}

		parsed = true

		ctx.Config = *params

		return nil
	}

	serve.Flag("config-path", "Path to base configuration.").Short('c').PlaceHolder("/path/to/file").Action(parseConfig).ExistingFileVar(&configFile)
	serve.Flag("contour-config-name", "Name of ContourConfiguration CRD.").PlaceHolder("contour").Action(parseConfig).StringVar(&ctx.contourConfigurationName)

	serve.Flag("incluster", "Use in cluster configuration.").BoolVar(&ctx.Config.InCluster)
	serve.Flag("kubeconfig", "Path to kubeconfig (if not in running inside a cluster).").PlaceHolder("/path/to/file").StringVar(&ctx.Config.Kubeconfig)

	serve.Flag("xds-address", "xDS gRPC API address.").PlaceHolder("<ipaddr>").StringVar(&ctx.xdsAddr)
	serve.Flag("xds-port", "xDS gRPC API port.").PlaceHolder("<port>").IntVar(&ctx.xdsPort)

	serve.Flag("stats-address", "Envoy /stats interface address.").PlaceHolder("<ipaddr>").StringVar(&ctx.statsAddr)
	serve.Flag("stats-port", "Envoy /stats interface port.").PlaceHolder("<port>").IntVar(&ctx.statsPort)

	serve.Flag("debug-http-address", "Address the debug http endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.debugAddr)
	serve.Flag("debug-http-port", "Port the debug http endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.debugPort)

	serve.Flag("http-address", "Address the metrics HTTP endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.metricsAddr)
	serve.Flag("http-port", "Port the metrics HTTP endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.metricsPort)
	serve.Flag("health-address", "Address the health HTTP endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.healthAddr)
	serve.Flag("health-port", "Port the health HTTP endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.healthPort)

	serve.Flag("contour-cafile", "CA bundle file name for serving gRPC with TLS.").Envar("CONTOUR_CAFILE").StringVar(&ctx.caFile)
	serve.Flag("contour-cert-file", "Contour certificate file name for serving gRPC over TLS.").PlaceHolder("/path/to/file").Envar("CONTOUR_CERT_FILE").StringVar(&ctx.contourCert)
	serve.Flag("contour-key-file", "Contour key file name for serving gRPC over TLS.").PlaceHolder("/path/to/file").Envar("CONTOUR_KEY_FILE").StringVar(&ctx.contourKey)
	serve.Flag("insecure", "Allow serving without TLS secured gRPC.").BoolVar(&ctx.PermitInsecureGRPC)
	serve.Flag("root-namespaces", "Restrict contour to searching these namespaces for root ingress routes.").PlaceHolder("<ns,ns>").StringVar(&ctx.rootNamespaces)

	serve.Flag("ingress-class-name", "Contour IngressClass name.").PlaceHolder("<name>").StringVar(&ctx.ingressClassName)
	serve.Flag("ingress-status-address", "Address to set in Ingress object status.").PlaceHolder("<address>").StringVar(&ctx.Config.IngressStatusAddress)
	serve.Flag("envoy-http-access-log", "Envoy HTTP access log.").PlaceHolder("/path/to/file").StringVar(&ctx.httpAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log.").PlaceHolder("/path/to/file").StringVar(&ctx.httpsAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests.").PlaceHolder("<ipaddr>").StringVar(&ctx.httpAddr)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests.").PlaceHolder("<ipaddr>").StringVar(&ctx.httpsAddr)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests.").PlaceHolder("<port>").IntVar(&ctx.httpPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests.").PlaceHolder("<port>").IntVar(&ctx.httpsPort)
	serve.Flag("envoy-service-name", "Name of the Envoy service to inspect for Ingress status details.").PlaceHolder("<name>").StringVar(&ctx.Config.EnvoyServiceName)
	serve.Flag("envoy-service-namespace", "Envoy Service Namespace.").PlaceHolder("<namespace>").StringVar(&ctx.Config.EnvoyServiceNamespace)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners.").BoolVar(&ctx.useProxyProto)

	serve.Flag("accesslog-format", "Format for Envoy access logs.").PlaceHolder("<envoy|json>").StringVar((*string)(&ctx.Config.AccessLogFormat))
	serve.Flag("disable-leader-election", "Disable leader election mechanism.").BoolVar(&ctx.DisableLeaderElection)

	serve.Flag("debug", "Enable debug logging.").Short('d').BoolVar(&ctx.Config.Debug)
	serve.Flag("kubernetes-debug", "Enable Kubernetes client debug logging with log level.").PlaceHolder("<log level>").UintVar(&ctx.KubernetesDebug)
	return serve, ctx
}

type Server struct {
	group    workgroup.Group
	log      logrus.FieldLogger
	ctx      *serveContext
	clients  *k8s.Clients
	mgr      manager.Manager
	registry *prometheus.Registry
}

// NewServer returns a Server object which contains the initial configuration
// objects required to start an instance of Contour.
func NewServer(log logrus.FieldLogger, ctx *serveContext) (*Server, error) {

	// Set up workgroup runner.
	var group workgroup.Group

	// Establish k8s core & dynamic client connections.
	clients, err := k8s.NewClients(ctx.Config.Kubeconfig, ctx.Config.InCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	scheme, err := k8s.NewContourScheme()
	if err != nil {
		return nil, fmt.Errorf("unable to create scheme: %w", err)
	}

	// Instantiate a controller-runtime manager.
	mgr, err := manager.New(controller_config.GetConfigOrDie(), manager.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to set up controller manager: %w", err)
	}

	// Set up Prometheus registry and register base metrics.
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())

	return &Server{
		group:    group,
		log:      log,
		ctx:      ctx,
		clients:  clients,
		mgr:      mgr,
		registry: registry,
	}, nil
}

// doServe runs the contour serve subcommand.
func (s *Server) doServe() error {

	var contourConfiguration contour_api_v1alpha1.ContourConfigurationSpec

	// Get the ContourConfiguration CRD if specified
	if len(s.ctx.contourConfigurationName) > 0 {
		// Determine the name/namespace of the configuration resource utilizing the environment
		// variable "CONTOUR_NAMESPACE" which should exist on the Contour deployment.
		//
		// If the env variable is not present, it will default to "projectcontour".
		contourNamespace, found := os.LookupEnv("CONTOUR_NAMESPACE")
		if !found {
			contourNamespace = "projectcontour"
		}

		namespacedName := types.NamespacedName{Name: s.ctx.contourConfigurationName, Namespace: contourNamespace}
		client := s.clients.DynamicClient().Resource(contour_api_v1alpha1.ContourConfigurationGVR).Namespace(namespacedName.Namespace)

		// ensure the specified ContourConfiguration exists
		res, err := client.Get(context.Background(), namespacedName.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting contour configuration %s: %v", namespacedName, err)
		}

		var contourConfig contour_api_v1alpha1.ContourConfiguration
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &contourConfig); err != nil {
			return fmt.Errorf("error converting contour configuration %s: %v", namespacedName, err)
		}

		// Copy the Spec from the parsed Configuration
		contourConfiguration = contourConfig.Spec
	} else {
		// No contour configuration passed, so convert the ServeContext into a ContourConfigurationSpec.
		contourConfiguration = s.ctx.convertToContourConfigurationSpec()
	}

	// Register the manager with the workgroup.
	s.group.AddContext(func(taskCtx context.Context) error {
		return s.mgr.Start(signals.SetupSignalHandler())
	})

	// informerNamespaces is a list of namespaces that we should start informers for.
	var informerNamespaces []string

	if len(contourConfiguration.HTTPProxy.RootNamespaces) > 0 {
		informerNamespaces = append(informerNamespaces, contourConfiguration.HTTPProxy.RootNamespaces...)

		// Add the FallbackCertificateNamespace to informerNamespaces if it isn't present.
		if contourConfiguration.HTTPProxy.FallbackCertificate != nil && !contains(informerNamespaces, contourConfiguration.HTTPProxy.FallbackCertificate.Namespace) {
			informerNamespaces = append(informerNamespaces, contourConfiguration.HTTPProxy.FallbackCertificate.Namespace)
			s.log.WithField("context", "fallback-certificate").
				Infof("fallback certificate namespace %q not defined in 'root-namespaces', adding namespace to watch",
					contourConfiguration.HTTPProxy.FallbackCertificate.Namespace)
		}

		// Add the client certificate namespace to informerNamespaces if it isn't present.
		if contourConfiguration.Envoy.ClientCertificate != nil && !contains(informerNamespaces, contourConfiguration.Envoy.ClientCertificate.Namespace) {
			informerNamespaces = append(informerNamespaces, contourConfiguration.Envoy.ClientCertificate.Namespace)
			s.log.WithField("context", "envoy-client-certificate").
				Infof("client certificate namespace %q not defined in 'root-namespaces', adding namespace to watch",
					contourConfiguration.Envoy.ClientCertificate.Namespace)
		}
	}

	// Before we can build the event handler, we need to initialize the converter we'll
	// use to convert from Unstructured.
	converter, err := k8s.NewUnstructuredConverter()
	if err != nil {
		return err
	}

	cipherSuites := []string{}
	for _, cs := range contourConfiguration.Envoy.Listener.TLS.CipherSuites {
		cipherSuites = append(cipherSuites, string(cs))
	}

	listenerConfig := xdscache_v3.NewListenerConfig(
		contourConfiguration.Envoy.Listener.UseProxyProto,
		contourConfiguration.Envoy.HTTPListener,
		contourConfiguration.Envoy.HTTPSListener,
		contourConfiguration.Envoy.Logging.AccessLogFormat,
		contourConfiguration.Envoy.Logging.AccessLogFields,
		contourConfiguration.Envoy.Logging.AccessLogFormatString,
		AccessLogFormatterExtensions(contourConfiguration.Envoy.Logging.AccessLogFormat, contourConfiguration.Envoy.Logging.AccessLogFields, contourConfiguration.Envoy.Logging.AccessLogFormatString),
		annotation.MinTLSVersion(contourConfiguration.Envoy.Listener.TLS.MinimumProtocolVersion, "1.2"),
		config.SanitizeCipherSuites(cipherSuites),
		contourConfiguration.Envoy.Timeouts,
		parseDefaultHTTPVersions(contourConfiguration.Envoy.DefaultHTTPVersions),
		!contourConfiguration.Envoy.Listener.DisableAllowChunkedLength,
		contourConfiguration.Envoy.Network.XffNumTrustedHops,
		contourConfiguration.Envoy.Listener.ConnectionBalancer,
		s.log,
	)

	if err = s.setupRateLimitService(contourConfiguration, &listenerConfig); err != nil {
		return err
	}

	contourMetrics := metrics.NewMetrics(s.registry)

	// Endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	endpointHandler := xdscache_v3.NewEndpointsTranslator(s.log.WithField("context", "endpointstranslator"))

	resources := []xdscache.ResourceCache{
		xdscache_v3.NewListenerCache(listenerConfig, contourConfiguration.Envoy.Metrics.Address, contourConfiguration.Envoy.Metrics.Port, contourConfiguration.Envoy.Network.EnvoyAdminPort),
		&xdscache_v3.SecretCache{},
		&xdscache_v3.RouteCache{},
		&xdscache_v3.ClusterCache{},
		endpointHandler,
	}

	// snapshotHandler is used to produce new snapshots when the internal state changes for any xDS resource.
	snapshotHandler := xdscache.NewSnapshotHandler(resources, s.log.WithField("context", "snapshotHandler"))

	// register observer for endpoints updates.
	endpointHandler.Observer = contour.ComposeObservers(snapshotHandler)

	// Log that we're using the fallback certificate if configured.
	if contourConfiguration.HTTPProxy.FallbackCertificate != nil {
		s.log.WithField("context", "fallback-certificate").Infof("enabled fallback certificate with secret: %q", contourConfiguration.HTTPProxy.FallbackCertificate)
	}
	if contourConfiguration.Envoy.ClientCertificate != nil {
		s.log.WithField("context", "envoy-client-certificate").Infof("enabled client certificate with secret: %q", contourConfiguration.Envoy.ClientCertificate)
	}

	ingressClassName := ""
	if contourConfiguration.Ingress != nil && contourConfiguration.Ingress.ClassName != nil {
		ingressClassName = *contourConfiguration.Ingress.ClassName
	}

	var clientCert *types.NamespacedName
	var fallbackCert *types.NamespacedName
	if contourConfiguration.Envoy.ClientCertificate != nil {
		clientCert = &types.NamespacedName{Name: contourConfiguration.Envoy.ClientCertificate.Name, Namespace: contourConfiguration.Envoy.ClientCertificate.Namespace}
	}
	if contourConfiguration.HTTPProxy.FallbackCertificate != nil {
		fallbackCert = &types.NamespacedName{Name: contourConfiguration.HTTPProxy.FallbackCertificate.Name, Namespace: contourConfiguration.HTTPProxy.FallbackCertificate.Namespace}
	}

	// Build the core Kubernetes event handler.
	contourHandler := &contour.EventHandler{
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		Observer:        dag.ComposeObservers(append(xdscache.ObserversOf(resources), snapshotHandler)...),
		Builder: s.getDAGBuilder(dagBuilderConfig{
			ingressClassName:          ingressClassName,
			rootNamespaces:            contourConfiguration.HTTPProxy.RootNamespaces,
			gatewayAPIConfigured:      contourConfiguration.Gateway != nil,
			disablePermitInsecure:     contourConfiguration.HTTPProxy.DisablePermitInsecure,
			enableExternalNameService: contourConfiguration.EnableExternalNameService,
			dnsLookupFamily:           contourConfiguration.Envoy.Cluster.DNSLookupFamily,
			headersPolicy:             contourConfiguration.Policy,
			clients:                   s.clients,
			clientCert:                clientCert,
			fallbackCert:              fallbackCert,
		}),
		FieldLogger: s.log.WithField("context", "contourEventHandler"),
	}

	// Wrap contourHandler in an EventRecorder which tracks API server events.
	eventHandler := &contour.EventRecorder{
		Next:    contourHandler,
		Counter: contourMetrics.EventHandlerOperations,
	}

	// Register leadership election.
	if contourConfiguration.LeaderElection.DisableLeaderElection {
		contourHandler.IsLeader = disableLeaderElection(s.log)
	} else {
		contourHandler.IsLeader = setupLeadershipElection(&s.group, s.log, contourConfiguration.LeaderElection, s.clients, contourHandler.UpdateNow)
	}

	// Start setting up StatusUpdateHandler since we need it in
	// the Gateway API controllers. Will finish setting it up and
	// start it later.
	sh := k8s.StatusUpdateHandler{
		Log:       s.log.WithField("context", "StatusUpdateHandler"),
		Clients:   s.clients,
		Cache:     s.mgr.GetCache(),
		Converter: converter,
	}

	// Inform on DefaultResources.
	for _, r := range k8s.DefaultResources() {
		if err := informOnResource(s.clients, r, eventHandler, s.mgr.GetCache()); err != nil {
			s.log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Inform on Gateway API resources.
	s.setupGatewayAPI(contourConfiguration, s.mgr, eventHandler, &sh, contourHandler.IsLeader)

	// Inform on secrets, filtering by root namespaces.
	for _, r := range k8s.SecretsResources() {
		var handler cache.ResourceEventHandler = eventHandler

		// If root namespaces are defined, filter for secrets in only those namespaces.
		if len(informerNamespaces) > 0 {
			handler = k8s.NewNamespaceFilter(informerNamespaces, eventHandler)
		}

		if err := informOnResource(s.clients, r, handler, s.mgr.GetCache()); err != nil {
			s.log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Inform on endpoints.
	for _, r := range k8s.EndpointsResources() {
		if err := informOnResource(s.clients, r, &contour.EventRecorder{
			Next:    endpointHandler,
			Counter: contourMetrics.EventHandlerOperations,
		}, s.mgr.GetCache()); err != nil {
			s.log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Register our event handler with the workgroup.
	s.group.Add(contourHandler.Start())

	// Create metrics service.
	s.setupMetrics(contourConfiguration.Metrics, contourConfiguration.Health, s.registry)

	// Create a separate health service if required.
	s.setupHealth(contourConfiguration.Health, contourConfiguration.Metrics)

	// Create debug service and register with workgroup.
	s.setupDebugService(contourConfiguration.Debug, contourHandler)

	// Once we have the leadership detection channel, we can
	// push DAG rebuild metrics onto the observer stack.
	contourHandler.Observer = &contour.RebuildMetricsObserver{
		Metrics:      contourMetrics,
		IsLeader:     contourHandler.IsLeader,
		NextObserver: contourHandler.Observer,
	}

	// Finish setting up the StatusUpdateHandler and
	// add it to the work group.
	sh.LeaderElected = contourHandler.IsLeader
	s.group.Add(sh.Start)

	// Now we have the statusUpdateHandler, we can create the event handler's StatusUpdater, which will take the
	// status updates from the DAG, and send them to the status update handler.
	contourHandler.StatusUpdater = sh.Writer()

	// Set up ingress load balancer status writer.
	lbsw := loadBalancerStatusWriter{
		log:              s.log.WithField("context", "loadBalancerStatusWriter"),
		cache:            s.mgr.GetCache(),
		isLeader:         contourHandler.IsLeader,
		lbStatus:         make(chan corev1.LoadBalancerStatus, 1),
		ingressClassName: ingressClassName,
		statusUpdater:    sh.Writer(),
		Converter:        converter,
	}
	s.group.Add(lbsw.Start)

	// Register an informer to watch envoy's service if we haven't been given static details.
	if contourConfiguration.Ingress != nil && contourConfiguration.Ingress.StatusAddress != nil {
		s.log.WithField("loadbalancer-address", *contourConfiguration.Ingress.StatusAddress).Info("Using supplied information for Ingress status")
		lbsw.lbStatus <- parseStatusFlag(*contourConfiguration.Ingress.StatusAddress)
	} else {
		serviceHandler := &k8s.ServiceStatusLoadBalancerWatcher{
			ServiceName: contourConfiguration.Envoy.Service.Name,
			LBStatus:    lbsw.lbStatus,
			Log:         s.log.WithField("context", "serviceStatusLoadBalancerWatcher"),
		}

		for _, r := range k8s.ServicesResources() {
			var handler cache.ResourceEventHandler = serviceHandler

			if contourConfiguration.Envoy.Service.Namespace != "" {
				handler = k8s.NewNamespaceFilter([]string{contourConfiguration.Envoy.Service.Namespace}, handler)
			}

			if err := informOnResource(s.clients, r, handler, s.mgr.GetCache()); err != nil {
				s.log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
			}
		}

		s.log.WithField("envoy-service-name", contourConfiguration.Envoy.Service.Name).
			WithField("envoy-service-namespace", contourConfiguration.Envoy.Service.Namespace).
			Info("Watching Service for Ingress status")
	}

	s.setupXDSServer(s.mgr, s.registry, contourConfiguration.XDSServer, snapshotHandler, resources)

	// Set up SIGTERM handler for graceful shutdown.
	s.group.Add(func(stop <-chan struct{}) error {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)
		select {
		case sig := <-c:
			s.log.WithField("context", "sigterm-handler").WithField("signal", sig).Info("shutting down")
		case <-stop:
			// Do nothing. The group is shutting down.
		}
		return nil
	})

	// GO!
	return s.group.Run(context.Background())
}

func (s *Server) setupRateLimitService(contourConfiguration contour_api_v1alpha1.ContourConfigurationSpec, listenerConfig *xdscache_v3.ListenerConfig) error {
	if contourConfiguration.RateLimitService == nil {
		return nil
	}

	namespacedName := &types.NamespacedName{
		Namespace: contourConfiguration.RateLimitService.ExtensionService.Namespace,
		Name:      contourConfiguration.RateLimitService.ExtensionService.Name,
	}
	client := s.clients.DynamicClient().Resource(contour_api_v1alpha1.ExtensionServiceGVR).Namespace(namespacedName.Namespace)

	// ensure the specified ExtensionService exists
	res, err := client.Get(context.Background(), namespacedName.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting rate limit extension service %s: %v", namespacedName, err)
	}
	var extensionSvc contour_api_v1alpha1.ExtensionService
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &extensionSvc); err != nil {
		return fmt.Errorf("error converting rate limit extension service %s: %v", namespacedName, err)
	}
	// get the response timeout from the ExtensionService
	var responseTimeout timeout.Setting
	if tp := extensionSvc.Spec.TimeoutPolicy; tp != nil {
		responseTimeout, err = timeout.Parse(tp.Response)
		if err != nil {
			return fmt.Errorf("error parsing rate limit extension service %s response timeout: %v", namespacedName, err)
		}
	}

	listenerConfig.RateLimitConfig = &xdscache_v3.RateLimitConfig{
		ExtensionService:        *namespacedName,
		Domain:                  contourConfiguration.RateLimitService.Domain,
		Timeout:                 responseTimeout,
		FailOpen:                contourConfiguration.RateLimitService.FailOpen,
		EnableXRateLimitHeaders: contourConfiguration.RateLimitService.EnableXRateLimitHeaders,
	}

	return nil
}

func (s *Server) setupDebugService(debugConfig contour_api_v1alpha1.DebugConfig, contourHandler *contour.EventHandler) {
	debugsvc := debug.Service{
		Service: httpsvc.Service{
			Addr:        debugConfig.Address,
			Port:        debugConfig.Port,
			FieldLogger: s.log.WithField("context", "debugsvc"),
		},
		Builder: &contourHandler.Builder,
	}
	s.group.Add(debugsvc.Start)
}

func (s *Server) setupXDSServer(mgr manager.Manager, registry *prometheus.Registry, contourConfiguration contour_api_v1alpha1.XDSServerConfig,
	snapshotHandler *xdscache.SnapshotHandler, resources []xdscache.ResourceCache) {

	s.group.AddContext(func(taskCtx context.Context) error {
		log := s.log.WithField("context", "xds")

		log.Printf("waiting for informer caches to sync")
		if !mgr.GetCache().WaitForCacheSync(taskCtx) {
			return errors.New("informer cache failed to sync")
		}
		log.Printf("informer caches synced")

		grpcServer := xds.NewServer(registry, grpcOptions(log, contourConfiguration.TLS)...)

		switch contourConfiguration.Type {
		case contour_api_v1alpha1.EnvoyServerType:
			v3cache := contour_xds_v3.NewSnapshotCache(false, log)
			snapshotHandler.AddSnapshotter(v3cache)
			contour_xds_v3.RegisterServer(envoy_server_v3.NewServer(taskCtx, v3cache, contour_xds_v3.NewRequestLoggingCallbacks(log)), grpcServer)
		case contour_api_v1alpha1.ContourServerType:
			contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(resources)...), grpcServer)
		default:
			// This can't happen due to config validation.
			log.Fatalf("invalid xDS server type %q", contourConfiguration.Type)
		}

		addr := net.JoinHostPort(contourConfiguration.Address, strconv.Itoa(contourConfiguration.Port))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		log = log.WithField("address", addr)
		if tls := contourConfiguration.TLS; tls != nil {
			if tls.Insecure {
				log = log.WithField("insecure", true)
			}
		}

		log.Infof("started xDS server type: %q", contourConfiguration.Type)
		defer log.Info("stopped xDS server")

		go func() {
			<-taskCtx.Done()

			// We don't use GracefulStop here because envoy
			// has long-lived hanging xDS requests. There's no
			// mechanism to make those pending requests fail,
			// so we forcibly terminate the TCP sessions.
			grpcServer.Stop()
		}()

		return grpcServer.Serve(l)
	})
}

func (s *Server) setupMetrics(metricsConfig contour_api_v1alpha1.MetricsConfig, healthConfig contour_api_v1alpha1.HealthConfig,
	registry *prometheus.Registry) {

	// Create metrics service and register with workgroup.
	metricsvc := httpsvc.Service{
		Addr:        metricsConfig.Address,
		Port:        metricsConfig.Port,
		FieldLogger: s.log.WithField("context", "metricsvc"),
		ServeMux:    http.ServeMux{},
	}

	metricsvc.ServeMux.Handle("/metrics", metrics.Handler(registry))

	if healthConfig.Address == metricsConfig.Address && healthConfig.Port == metricsConfig.Port {
		h := health.Handler(s.clients.ClientSet())
		metricsvc.ServeMux.Handle("/health", h)
		metricsvc.ServeMux.Handle("/healthz", h)
	}

	s.group.Add(metricsvc.Start)
}

func (s *Server) setupHealth(healthConfig contour_api_v1alpha1.HealthConfig,
	metricsConfig contour_api_v1alpha1.MetricsConfig) {

	if healthConfig.Address != metricsConfig.Address || healthConfig.Port != metricsConfig.Port {
		healthsvc := httpsvc.Service{
			Addr:        healthConfig.Address,
			Port:        healthConfig.Port,
			FieldLogger: s.log.WithField("context", "healthsvc"),
		}

		h := health.Handler(s.clients.ClientSet())
		healthsvc.ServeMux.Handle("/health", h)
		healthsvc.ServeMux.Handle("/healthz", h)

		s.group.Add(healthsvc.Start)
	}
}

func (s *Server) setupGatewayAPI(contourConfiguration contour_api_v1alpha1.ContourConfigurationSpec,
	mgr manager.Manager, eventHandler *contour.EventRecorder, sh *k8s.StatusUpdateHandler, isLeader chan struct{}) {

	// Check if GatewayAPI is configured.
	if contourConfiguration.Gateway != nil {

		// Only inform on GatewayAPI if found in the cluster.
		if s.clients.ResourcesExist(k8s.GatewayAPIResources()...) {

			// Create and register the gatewayclass controller with the manager.
			gatewayClassControllerName := contourConfiguration.Gateway.ControllerName
			if _, err := controller.NewGatewayClassController(
				mgr,
				eventHandler,
				sh.Writer(),
				s.log.WithField("context", "gatewayclass-controller"),
				gatewayClassControllerName,
				isLeader,
			); err != nil {
				s.log.WithError(err).Fatal("failed to create gatewayclass-controller")
			}

			// Create and register the NewGatewayController controller with the manager.
			if _, err := controller.NewGatewayController(
				mgr,
				eventHandler,
				sh.Writer(),
				s.log.WithField("context", "gateway-controller"),
				gatewayClassControllerName,
				isLeader,
			); err != nil {
				s.log.WithError(err).Fatal("failed to create gateway-controller")
			}

			// Create and register the NewHTTPRouteController controller with the manager.
			if _, err := controller.NewHTTPRouteController(mgr, eventHandler, s.log.WithField("context", "httproute-controller")); err != nil {
				s.log.WithError(err).Fatal("failed to create httproute-controller")
			}

			// Create and register the NewTLSRouteController controller with the manager.
			if _, err := controller.NewTLSRouteController(mgr, eventHandler, s.log.WithField("context", "tlsroute-controller")); err != nil {
				s.log.WithError(err).Fatal("failed to create tlsroute-controller")
			}

			// Inform on Namespaces.
			if err := informOnResource(s.clients, k8s.NamespacesResource(), eventHandler, mgr.GetCache()); err != nil {
				s.log.WithError(err).WithField("resource", k8s.NamespacesResource()).Fatal("failed to create informer")
			}
		} else {
			log.Fatalf("Gateway API Gateway configured but APIs not installed in cluster.")
		}
	}
}

type dagBuilderConfig struct {
	ingressClassName           string
	rootNamespaces             []string
	gatewayAPIConfigured       bool
	disablePermitInsecure      bool
	enableExternalNameService  bool
	dnsLookupFamily            contour_api_v1alpha1.ClusterDNSFamilyType
	headersPolicy              *contour_api_v1alpha1.PolicyConfig
	applyHeaderPolicyToIngress bool
	clients                    *k8s.Clients
	clientCert                 *types.NamespacedName
	fallbackCert               *types.NamespacedName
}

func (s *Server) getDAGBuilder(dbc dagBuilderConfig) dag.Builder {

	var requestHeadersPolicy dag.HeadersPolicy
	var responseHeadersPolicy dag.HeadersPolicy

	if dbc.headersPolicy != nil {
		if dbc.headersPolicy.RequestHeadersPolicy != nil {
			if dbc.headersPolicy.RequestHeadersPolicy.Set != nil {
				requestHeadersPolicy.Set = make(map[string]string)
				for k, v := range dbc.headersPolicy.RequestHeadersPolicy.Set {
					requestHeadersPolicy.Set[k] = v
				}
			}
			if dbc.headersPolicy.RequestHeadersPolicy.Remove != nil {
				requestHeadersPolicy.Remove = make([]string, 0, len(dbc.headersPolicy.RequestHeadersPolicy.Remove))
				requestHeadersPolicy.Remove = append(requestHeadersPolicy.Remove, dbc.headersPolicy.RequestHeadersPolicy.Remove...)
			}
		}

		if dbc.headersPolicy.ResponseHeadersPolicy != nil {
			if dbc.headersPolicy.ResponseHeadersPolicy.Set != nil {
				responseHeadersPolicy.Set = make(map[string]string)
				for k, v := range dbc.headersPolicy.ResponseHeadersPolicy.Set {
					responseHeadersPolicy.Set[k] = v
				}
			}
			if dbc.headersPolicy.ResponseHeadersPolicy.Remove != nil {
				responseHeadersPolicy.Remove = make([]string, 0, len(dbc.headersPolicy.ResponseHeadersPolicy.Remove))
				responseHeadersPolicy.Remove = append(responseHeadersPolicy.Remove, dbc.headersPolicy.ResponseHeadersPolicy.Remove...)
			}
		}
	}

	var requestHeadersPolicyIngress dag.HeadersPolicy
	var responseHeadersPolicyIngress dag.HeadersPolicy
	if dbc.applyHeaderPolicyToIngress {
		requestHeadersPolicyIngress = requestHeadersPolicy
		responseHeadersPolicyIngress = responseHeadersPolicy
	}

	s.log.Debugf("EnableExternalNameService is set to %t", dbc.enableExternalNameService)

	// Get the appropriate DAG processors.
	dagProcessors := []dag.Processor{
		&dag.IngressProcessor{
			EnableExternalNameService: dbc.enableExternalNameService,
			FieldLogger:               s.log.WithField("context", "IngressProcessor"),
			ClientCertificate:         dbc.clientCert,
			RequestHeadersPolicy:      &requestHeadersPolicyIngress,
			ResponseHeadersPolicy:     &responseHeadersPolicyIngress,
		},
		&dag.ExtensionServiceProcessor{
			// Note that ExtensionService does not support ExternalName, if it does get added,
			// need to bring EnableExternalNameService in here too.
			FieldLogger:       s.log.WithField("context", "ExtensionServiceProcessor"),
			ClientCertificate: dbc.clientCert,
		},
		&dag.HTTPProxyProcessor{
			EnableExternalNameService: dbc.enableExternalNameService,
			DisablePermitInsecure:     dbc.disablePermitInsecure,
			FallbackCertificate:       dbc.fallbackCert,
			DNSLookupFamily:           dbc.dnsLookupFamily,
			ClientCertificate:         dbc.clientCert,
			RequestHeadersPolicy:      &requestHeadersPolicy,
			ResponseHeadersPolicy:     &responseHeadersPolicy,
		},
	}

	if dbc.gatewayAPIConfigured && dbc.clients.ResourcesExist(k8s.GatewayAPIResources()...) {
		dagProcessors = append(dagProcessors, &dag.GatewayAPIProcessor{
			EnableExternalNameService: dbc.enableExternalNameService,
			FieldLogger:               s.log.WithField("context", "GatewayAPIProcessor"),
		})
	}

	// The listener processor has to go last since it looks at
	// the output of the other processors.
	dagProcessors = append(dagProcessors, &dag.ListenerProcessor{})

	var configuredSecretRefs []*types.NamespacedName
	if dbc.fallbackCert != nil {
		configuredSecretRefs = append(configuredSecretRefs, dbc.fallbackCert)
	}
	if dbc.clientCert != nil {
		configuredSecretRefs = append(configuredSecretRefs, dbc.clientCert)
	}

	builder := dag.Builder{
		Source: dag.KubernetesCache{
			RootNamespaces:       dbc.rootNamespaces,
			IngressClassName:     dbc.ingressClassName,
			ConfiguredSecretRefs: configuredSecretRefs,
			FieldLogger:          s.log.WithField("context", "KubernetesCache"),
		},
		Processors: dagProcessors,
	}

	// govet complains about copying the sync.Once that's in the dag.KubernetesCache
	// but it's safe to ignore since this function is only called once.
	// nolint:govet
	return builder
}

func contains(namespaces []string, ns string) bool {
	for _, namespace := range namespaces {
		if ns == namespace {
			return true
		}
	}
	return false
}

func informOnResource(clients *k8s.Clients, gvr schema.GroupVersionResource, handler cache.ResourceEventHandler, cache ctrl_cache.Cache) error {
	gvk, err := clients.KindFor(gvr)
	if err != nil {
		return err
	}

	inf, err := cache.GetInformerForKind(context.Background(), gvk)
	if err != nil {
		return err
	}

	inf.AddEventHandler(handler)
	return nil
}

// commandOperatorRegexp parses the command operators used in Envoy access log configuration
//
// Capture Groups:
// Given string "the start time is %START_TIME(%s):3% wow!"
//
//   0. Whole match "%START_TIME(%s):3%"
//   1. Full operator: "START_TIME(%s):3%"
//   2. Operator Name: "START_TIME"
//   3. Arguments: "(%s)"
//   4. Truncation length: ":3"
var commandOperatorRegexp = regexp.MustCompile(`%(([A-Z_]+)(\([^)]+\)(:[0-9]+)?)?%)?`)

// AccessLogFormatterExtensions returns a list of formatter extension names required by the access log format.
//
// Note: When adding support for new formatter, update the list of extensions here and
// add corresponding configuration in internal/envoy/v3/accesslog.go extensionConfig().
// Currently only one extension exist in Envoy.
func AccessLogFormatterExtensions(accessLogFormat contour_api_v1alpha1.AccessLogType, accessLogFields contour_api_v1alpha1.AccessLogFields,
	accessLogFormatString *string) []string {
	// Function that finds out if command operator is present in a format string.
	contains := func(format, command string) bool {
		tokens := commandOperatorRegexp.FindAllStringSubmatch(format, -1)
		for _, t := range tokens {
			if t[2] == command {
				return true
			}
		}
		return false
	}

	extensionsMap := make(map[string]bool)
	switch accessLogFormat {
	case contour_api_v1alpha1.EnvoyAccessLog:
		if accessLogFormatString != nil {
			if contains(*accessLogFormatString, "REQ_WITHOUT_QUERY") {
				extensionsMap["envoy.formatter.req_without_query"] = true
			}
		}
	case contour_api_v1alpha1.JSONAccessLog:
		for _, f := range accessLogFields.AsFieldMap() {
			if contains(f, "REQ_WITHOUT_QUERY") {
				extensionsMap["envoy.formatter.req_without_query"] = true
			}
		}
	}

	var extensions []string
	for k := range extensionsMap {
		extensions = append(extensions, k)
	}

	return extensions
}
