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
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/alecthomas/kingpin/v2"
	envoy_server_v3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/annotation"
	"github.com/projectcontour/contour/internal/contour"
	"github.com/projectcontour/contour/internal/contourconfig"
	"github.com/projectcontour/contour/internal/controller"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/projectcontour/contour/internal/debug"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/health"
	"github.com/projectcontour/contour/internal/httpsvc"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/leadership"
	"github.com/projectcontour/contour/internal/metrics"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/internal/timeout"
	"github.com/projectcontour/contour/internal/xds"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
	xdscache_v3 "github.com/projectcontour/contour/internal/xdscache/v3"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	networking_v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ctrl_cache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

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
	serve.Flag("accesslog-format", "Format for Envoy access logs.").PlaceHolder("<envoy|json>").StringVar((*string)(&ctx.Config.AccessLogFormat))

	serve.Flag("config-path", "Path to base configuration.").Short('c').PlaceHolder("/path/to/file").Action(parseConfig).ExistingFileVar(&configFile)
	serve.Flag("contour-cafile", "CA bundle file name for serving gRPC with TLS.").Envar("CONTOUR_CAFILE").StringVar(&ctx.caFile)
	serve.Flag("contour-cert-file", "Contour certificate file name for serving gRPC over TLS.").PlaceHolder("/path/to/file").Envar("CONTOUR_CERT_FILE").StringVar(&ctx.contourCert)
	serve.Flag("contour-config-name", "Name of ContourConfiguration CRD.").PlaceHolder("contour").Action(parseConfig).StringVar(&ctx.contourConfigurationName)
	serve.Flag("contour-key-file", "Contour key file name for serving gRPC over TLS.").PlaceHolder("/path/to/file").Envar("CONTOUR_KEY_FILE").StringVar(&ctx.contourKey)

	serve.Flag("debug", "Enable debug logging.").Short('d').BoolVar(&ctx.Config.Debug)
	serve.Flag("debug-http-address", "Address the debug http endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.debugAddr)
	serve.Flag("debug-http-port", "Port the debug http endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.debugPort)
	serve.Flag("disable-feature", "Do not start an informer for the specified resources.").PlaceHolder("<extensionservices>").EnumsVar(&ctx.disabledFeatures, "extensionservices")
	serve.Flag("disable-leader-election", "Disable leader election mechanism.").BoolVar(&ctx.LeaderElection.Disable)

	serve.Flag("envoy-http-access-log", "Envoy HTTP access log.").PlaceHolder("/path/to/file").StringVar(&ctx.httpAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log.").PlaceHolder("/path/to/file").StringVar(&ctx.httpsAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests.").PlaceHolder("<ipaddr>").StringVar(&ctx.httpAddr)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests.").PlaceHolder("<port>").IntVar(&ctx.httpPort)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests.").PlaceHolder("<ipaddr>").StringVar(&ctx.httpsAddr)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests.").PlaceHolder("<port>").IntVar(&ctx.httpsPort)
	serve.Flag("envoy-service-name", "Name of the Envoy service to inspect for Ingress status details.").PlaceHolder("<name>").StringVar(&ctx.Config.EnvoyServiceName)
	serve.Flag("envoy-service-namespace", "Envoy Service Namespace.").PlaceHolder("<namespace>").StringVar(&ctx.Config.EnvoyServiceNamespace)

	serve.Flag("health-address", "Address the health HTTP endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.healthAddr)
	serve.Flag("health-port", "Port the health HTTP endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.healthPort)
	serve.Flag("http-address", "Address the metrics HTTP endpoint will bind to.").PlaceHolder("<ipaddr>").StringVar(&ctx.metricsAddr)
	serve.Flag("http-port", "Port the metrics HTTP endpoint will bind to.").PlaceHolder("<port>").IntVar(&ctx.metricsPort)

	serve.Flag("incluster", "Use in cluster configuration.").BoolVar(&ctx.Config.InCluster)
	serve.Flag("ingress-class-name", "Contour IngressClass name.").PlaceHolder("<name>").StringVar(&ctx.ingressClassName)
	serve.Flag("ingress-status-address", "Address to set in Ingress object status.").PlaceHolder("<address>").StringVar(&ctx.Config.IngressStatusAddress)
	serve.Flag("insecure", "Allow serving without TLS secured gRPC.").BoolVar(&ctx.PermitInsecureGRPC)

	serve.Flag("kubeconfig", "Path to kubeconfig (if not in running inside a cluster).").PlaceHolder("/path/to/file").StringVar(&ctx.Config.Kubeconfig)
	serve.Flag("kubernetes-client-burst", "Burst allowed for the Kubernetes client.").IntVar(&ctx.Config.KubeClientBurst)
	serve.Flag("kubernetes-client-qps", "QPS allowed for the Kubernetes client.").Float32Var(&ctx.Config.KubeClientQPS)
	serve.Flag("kubernetes-debug", "Enable Kubernetes client debug logging with log level.").PlaceHolder("<log level>").UintVar(&ctx.KubernetesDebug)

	serve.Flag("leader-election-lease-duration", "The duration of the leadership lease.").Default("15s").DurationVar(&ctx.LeaderElection.LeaseDuration)
	serve.Flag("leader-election-renew-deadline", "The duration leader will retry refreshing leadership before giving up.").Default("10s").DurationVar(&ctx.LeaderElection.RenewDeadline)
	serve.Flag("leader-election-resource-name", "The name of the resource (Lease) leader election will lease.").Default("leader-elect").StringVar(&ctx.LeaderElection.Name)
	serve.Flag("leader-election-resource-namespace", "The namespace of the resource (Lease) leader election will lease.").Default(config.GetenvOr("CONTOUR_NAMESPACE", "projectcontour")).StringVar(&ctx.LeaderElection.Namespace)
	serve.Flag("leader-election-retry-period", "The interval which Contour will attempt to acquire leadership lease.").Default("2s").DurationVar(&ctx.LeaderElection.RetryPeriod)

	serve.Flag("root-namespaces", "Restrict contour to searching these namespaces for root ingress routes.").PlaceHolder("<ns,ns>").StringVar(&ctx.rootNamespaces)

	serve.Flag("stats-address", "Envoy /stats interface address.").PlaceHolder("<ipaddr>").StringVar(&ctx.statsAddr)
	serve.Flag("stats-port", "Envoy /stats interface port.").PlaceHolder("<port>").IntVar(&ctx.statsPort)

	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners.").BoolVar(&ctx.useProxyProto)

	serve.Flag("xds-address", "xDS gRPC API address.").PlaceHolder("<ipaddr>").StringVar(&ctx.xdsAddr)
	serve.Flag("xds-port", "xDS gRPC API port.").PlaceHolder("<port>").IntVar(&ctx.xdsPort)

	return serve, ctx
}

type Server struct {
	log        logrus.FieldLogger
	ctx        *serveContext
	coreClient *kubernetes.Clientset
	mgr        manager.Manager
	registry   *prometheus.Registry
}

// NewServer returns a Server object which contains the initial configuration
// objects required to start an instance of Contour.
func NewServer(log logrus.FieldLogger, ctx *serveContext) (*Server, error) {

	var restConfigOpts []func(*rest.Config)

	if qps := ctx.Config.KubeClientQPS; qps > 0 {
		log.Debugf("Setting Kubernetes client QPS to %v", qps)
		restConfigOpts = append(restConfigOpts, k8s.OptSetQPS(qps))
	}
	if burst := ctx.Config.KubeClientBurst; burst > 0 {
		log.Debugf("Setting Kubernetes client burst to %v", burst)
		restConfigOpts = append(restConfigOpts, k8s.OptSetBurst(burst))
	}

	// Establish k8s core client connection.
	restConfig, err := k8s.NewRestConfig(ctx.Config.Kubeconfig, ctx.Config.InCluster, restConfigOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config for Kubernetes clients: %w", err)
	}

	coreClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	scheme, err := k8s.NewContourScheme()
	if err != nil {
		return nil, fmt.Errorf("unable to create scheme: %w", err)
	}

	// Instantiate a controller-runtime manager.
	options := manager.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     "0",
		HealthProbeBindAddress: "0",
	}
	if ctx.LeaderElection.Disable {
		log.Info("Leader election disabled")
		options.LeaderElection = false
	} else {
		options.LeaderElection = true
		options.LeaderElectionResourceLock = "leases"
		options.LeaderElectionNamespace = ctx.LeaderElection.Namespace
		options.LeaderElectionID = ctx.LeaderElection.Name
		options.LeaseDuration = &ctx.LeaderElection.LeaseDuration
		options.RenewDeadline = &ctx.LeaderElection.RenewDeadline
		options.RetryPeriod = &ctx.LeaderElection.RetryPeriod
		options.LeaderElectionReleaseOnCancel = true
	}
	mgr, err := manager.New(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("unable to set up controller manager: %w", err)
	}

	// Set up Prometheus registry and register base metrics.
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())

	return &Server{
		log:        log,
		ctx:        ctx,
		coreClient: coreClient,
		mgr:        mgr,
		registry:   registry,
	}, nil
}

func (s *Server) getConfig() (contour_api_v1alpha1.ContourConfigurationSpec, error) {
	var userConfig contour_api_v1alpha1.ContourConfigurationSpec

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

		contourConfig := &contour_api_v1alpha1.ContourConfiguration{}
		key := client.ObjectKey{Namespace: contourNamespace, Name: s.ctx.contourConfigurationName}

		// Using GetAPIReader() here because the manager's caches won't be started yet,
		// so reads from the manager's client (which uses the caches for reads) will fail.
		if err := s.mgr.GetAPIReader().Get(context.Background(), key, contourConfig); err != nil {
			return contour_api_v1alpha1.ContourConfigurationSpec{}, fmt.Errorf("error getting contour configuration %s: %v", key, err)
		}

		// Copy the Spec from the parsed Configuration
		userConfig = contourConfig.Spec
	} else {
		// No contour configuration passed, so convert the ServeContext into a ContourConfigurationSpec.
		userConfig = s.ctx.convertToContourConfigurationSpec()
	}

	// Overlay the user-specified config onto the default config to come up
	// with the final set of config to use.
	contourConfiguration, err := contourconfig.OverlayOnDefaults(userConfig)
	if err != nil {
		return contour_api_v1alpha1.ContourConfigurationSpec{}, err
	}

	if err := contourConfiguration.Validate(); err != nil {
		return contour_api_v1alpha1.ContourConfigurationSpec{}, err
	}

	return contourConfiguration, nil
}

// doServe runs the contour serve subcommand.
func (s *Server) doServe() error {
	// Get config. Any user-specified settings are "overlaid" onto default settings
	// to come up with the final config. Note that because the default settings (as
	// defined in contourconfig.Defaults()) instantiate structs for nearly every pointer
	// in the ContourConfigurationSpec object structure, most nil-checking can be
	// omitted in the below code. The exceptions are truly optional fields, such as
	// RateLimitService, which is not required to exist/have a default.
	contourConfiguration, err := s.getConfig()
	if err != nil {
		return err
	}

	// informerNamespaces is a set of namespaces that we should start informers for.
	// If empty, informers will be started for all namespaces.
	informerNamespaces := sets.NewString()

	if rootNamespaces := contourConfiguration.HTTPProxy.RootNamespaces; len(rootNamespaces) > 0 {
		s.log.WithField("context", "root-namespaces").Infof("watching root namespaces %q", rootNamespaces)
		informerNamespaces.Insert(rootNamespaces...)

		// The fallback cert and client cert's namespaces only need to be added to informerNamespaces
		// if we're processing specifici root namespaces, because otherwise, the informers will start
		// for all namespaces so the below will automatically be included.

		if fallbackCert := contourConfiguration.HTTPProxy.FallbackCertificate; fallbackCert != nil {
			s.log.WithField("context", "fallback-certificate").Infof("watching fallback certificate namespace %q", fallbackCert.Namespace)
			informerNamespaces.Insert(fallbackCert.Namespace)
		}

		if clientCert := contourConfiguration.Envoy.ClientCertificate; clientCert != nil {
			s.log.WithField("context", "envoy-client-certificate").Infof("watching client certificate namespace %q", clientCert.Namespace)
			informerNamespaces.Insert(clientCert.Namespace)
		}
	}

	timeouts, err := contourconfig.ParseTimeoutPolicy(contourConfiguration.Envoy.Timeouts)
	if err != nil {
		return err
	}

	listenerConfig := xdscache_v3.ListenerConfig{
		UseProxyProto:                *contourConfiguration.Envoy.Listener.UseProxyProto,
		HTTPAccessLog:                contourConfiguration.Envoy.HTTPListener.AccessLog,
		HTTPSAccessLog:               contourConfiguration.Envoy.HTTPSListener.AccessLog,
		AccessLogType:                contourConfiguration.Envoy.Logging.AccessLogFormat,
		AccessLogJSONFields:          contourConfiguration.Envoy.Logging.AccessLogJSONFields,
		AccessLogLevel:               contourConfiguration.Envoy.Logging.AccessLogLevel,
		AccessLogFormatString:        contourConfiguration.Envoy.Logging.AccessLogFormatString,
		AccessLogFormatterExtensions: contourConfiguration.Envoy.Logging.AccessLogFormatterExtensions(),
		MinimumTLSVersion:            annotation.MinTLSVersion(contourConfiguration.Envoy.Listener.TLS.MinimumProtocolVersion, "1.2"),
		CipherSuites:                 contourConfiguration.Envoy.Listener.TLS.SanitizedCipherSuites(),
		Timeouts:                     timeouts,
		DefaultHTTPVersions:          parseDefaultHTTPVersions(contourConfiguration.Envoy.DefaultHTTPVersions),
		AllowChunkedLength:           !*contourConfiguration.Envoy.Listener.DisableAllowChunkedLength,
		MergeSlashes:                 !*contourConfiguration.Envoy.Listener.DisableMergeSlashes,
		ServerHeaderTransformation:   contourConfiguration.Envoy.Listener.ServerHeaderTransformation,
		XffNumTrustedHops:            *contourConfiguration.Envoy.Network.XffNumTrustedHops,
		ConnectionBalancer:           contourConfiguration.Envoy.Listener.ConnectionBalancer,
	}

	if listenerConfig.RateLimitConfig, err = s.setupRateLimitService(contourConfiguration); err != nil {
		return err
	}

	if listenerConfig.GlobalExternalAuthConfig, err = s.setupGlobalExternalAuthentication(contourConfiguration); err != nil {
		return err
	}

	contourMetrics := metrics.NewMetrics(s.registry)

	// Endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	endpointHandler := xdscache_v3.NewEndpointsTranslator(s.log.WithField("context", "endpointstranslator"))

	resources := []xdscache.ResourceCache{
		xdscache_v3.NewListenerCache(listenerConfig, *contourConfiguration.Envoy.Metrics, *contourConfiguration.Envoy.Health, *contourConfiguration.Envoy.Network.EnvoyAdminPort),
		xdscache_v3.NewSecretsCache(envoy_v3.StatsSecrets(contourConfiguration.Envoy.Metrics.TLS)),
		&xdscache_v3.RouteCache{},
		&xdscache_v3.ClusterCache{},
		endpointHandler,
		&xdscache_v3.RuntimeCache{},
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

	var ingressClassNames []string
	if contourConfiguration.Ingress != nil {
		ingressClassNames = contourConfiguration.Ingress.ClassNames
	}

	var clientCert *types.NamespacedName
	var fallbackCert *types.NamespacedName
	if contourConfiguration.Envoy.ClientCertificate != nil {
		clientCert = &types.NamespacedName{Name: contourConfiguration.Envoy.ClientCertificate.Name, Namespace: contourConfiguration.Envoy.ClientCertificate.Namespace}
	}
	if contourConfiguration.HTTPProxy.FallbackCertificate != nil {
		fallbackCert = &types.NamespacedName{Name: contourConfiguration.HTTPProxy.FallbackCertificate.Name, Namespace: contourConfiguration.HTTPProxy.FallbackCertificate.Namespace}
	}

	sh := k8s.NewStatusUpdateHandler(s.log.WithField("context", "StatusUpdateHandler"), s.mgr.GetClient())
	if err := s.mgr.Add(sh); err != nil {
		return err
	}

	var gatewayControllerName string
	var gatewayRef *types.NamespacedName

	if contourConfiguration.Gateway != nil {
		gatewayControllerName = contourConfiguration.Gateway.ControllerName

		if contourConfiguration.Gateway.GatewayRef != nil {
			gatewayRef = &types.NamespacedName{
				Namespace: contourConfiguration.Gateway.GatewayRef.Namespace,
				Name:      contourConfiguration.Gateway.GatewayRef.Name,
			}
		}
	}

	builder := s.getDAGBuilder(dagBuilderConfig{
		ingressClassNames:                  ingressClassNames,
		rootNamespaces:                     contourConfiguration.HTTPProxy.RootNamespaces,
		gatewayControllerName:              gatewayControllerName,
		gatewayRef:                         gatewayRef,
		disablePermitInsecure:              *contourConfiguration.HTTPProxy.DisablePermitInsecure,
		enableExternalNameService:          *contourConfiguration.EnableExternalNameService,
		dnsLookupFamily:                    contourConfiguration.Envoy.Cluster.DNSLookupFamily,
		headersPolicy:                      contourConfiguration.Policy,
		clientCert:                         clientCert,
		fallbackCert:                       fallbackCert,
		connectTimeout:                     timeouts.ConnectTimeout,
		client:                             s.mgr.GetClient(),
		metrics:                            contourMetrics,
		httpAddress:                        contourConfiguration.Envoy.HTTPListener.Address,
		httpPort:                           contourConfiguration.Envoy.HTTPListener.Port,
		httpsAddress:                       contourConfiguration.Envoy.HTTPSListener.Address,
		httpsPort:                          contourConfiguration.Envoy.HTTPSListener.Port,
		globalExternalAuthorizationService: contourConfiguration.GlobalExternalAuthorization,
	})

	// Build the core Kubernetes event handler.
	observer := contour.NewRebuildMetricsObserver(
		contourMetrics,
		dag.ComposeObservers(append(xdscache.ObserversOf(resources), snapshotHandler)...),
	)
	contourHandler := contour.NewEventHandler(contour.EventHandlerConfig{
		Logger:          s.log.WithField("context", "contourEventHandler"),
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		Observer:        observer,
		StatusUpdater:   sh.Writer(),
		Builder:         builder,
	})

	// Wrap contourHandler in an EventRecorder which tracks API server events.
	eventHandler := &contour.EventRecorder{
		Next:    contourHandler,
		Counter: contourMetrics.EventHandlerOperations,
	}

	// Start to build informers.
	informerResources := map[string]client.Object{
		"httpproxies":               &contour_api_v1.HTTPProxy{},
		"tlscertificatedelegations": &contour_api_v1.TLSCertificateDelegation{},
		"extensionservices":         &contour_api_v1alpha1.ExtensionService{},
		"services":                  &corev1.Service{},
		"ingresses":                 &networking_v1.Ingress{},
	}

	// Some of the resources are optional and can be disabled, do not create informers for those.
	for _, feat := range s.ctx.disabledFeatures {
		delete(informerResources, feat)
	}

	// Inform on the remaining resources.
	for name, r := range informerResources {
		if err := informOnResource(r, eventHandler, s.mgr.GetCache()); err != nil {
			s.log.WithError(err).WithField("resource", name).Fatal("failed to create informer")
		}
	}

	// Inform on Gateway API resources.
	needsNotification := s.setupGatewayAPI(contourConfiguration, s.mgr, eventHandler, sh)

	// Inform on secrets, filtering by root namespaces.
	var handler cache.ResourceEventHandler = eventHandler

	// If root namespaces are defined, filter for secrets in only those namespaces.
	if len(informerNamespaces) > 0 {
		handler = k8s.NewNamespaceFilter(informerNamespaces.List(), eventHandler)
	}

	if err := informOnResource(&corev1.Secret{}, handler, s.mgr.GetCache()); err != nil {
		s.log.WithError(err).WithField("resource", "secrets").Fatal("failed to create informer")
	}

	// Inform on endpoints.
	if err := informOnResource(&corev1.Endpoints{}, &contour.EventRecorder{
		Next:    endpointHandler,
		Counter: contourMetrics.EventHandlerOperations,
	}, s.mgr.GetCache()); err != nil {
		s.log.WithError(err).WithField("resource", "endpoints").Fatal("failed to create informer")
	}

	// Register our event handler with the manager.
	if err := s.mgr.Add(contourHandler); err != nil {
		return err
	}

	// Create metrics service.
	if err := s.setupMetrics(*contourConfiguration.Metrics, *contourConfiguration.Health, s.registry); err != nil {
		return err
	}

	// Create a separate health service if required.
	if err := s.setupHealth(*contourConfiguration.Health, *contourConfiguration.Metrics); err != nil {
		return err
	}

	// Create debug service and register with mgr.
	if err := s.setupDebugService(*contourConfiguration.Debug, builder); err != nil {
		return err
	}

	// Set up ingress load balancer status writer.
	lbsw := &loadBalancerStatusWriter{
		log:                   s.log.WithField("context", "loadBalancerStatusWriter"),
		cache:                 s.mgr.GetCache(),
		lbStatus:              make(chan corev1.LoadBalancerStatus, 1),
		ingressClassNames:     ingressClassNames,
		gatewayControllerName: gatewayControllerName,
		gatewayRef:            gatewayRef,
		statusUpdater:         sh.Writer(),
	}
	if err := s.mgr.Add(lbsw); err != nil {
		return err
	}

	// Register an informer to watch envoy's service if we haven't been given static details.
	if lbAddress := contourConfiguration.Ingress.StatusAddress; len(lbAddress) > 0 {
		s.log.WithField("loadbalancer-address", lbAddress).Info("Using supplied information for Ingress status")
		lbsw.lbStatus <- parseStatusFlag(lbAddress)
	} else {
		serviceHandler := &k8s.ServiceStatusLoadBalancerWatcher{
			ServiceName: contourConfiguration.Envoy.Service.Name,
			LBStatus:    lbsw.lbStatus,
			Log:         s.log.WithField("context", "serviceStatusLoadBalancerWatcher"),
		}

		var handler cache.ResourceEventHandler = serviceHandler
		if contourConfiguration.Envoy.Service.Namespace != "" {
			handler = k8s.NewNamespaceFilter([]string{contourConfiguration.Envoy.Service.Namespace}, handler)
		}

		if err := informOnResource(&corev1.Service{}, handler, s.mgr.GetCache()); err != nil {
			s.log.WithError(err).WithField("resource", "services").Fatal("failed to create informer")
		}

		s.log.WithField("envoy-service-name", contourConfiguration.Envoy.Service.Name).
			WithField("envoy-service-namespace", contourConfiguration.Envoy.Service.Namespace).
			Info("Watching Service for Ingress status")
	}

	xdsServer := &xdsServer{
		log:             s.log,
		mgr:             s.mgr,
		registry:        s.registry,
		config:          *contourConfiguration.XDSServer,
		snapshotHandler: snapshotHandler,
		resources:       resources,
	}
	if err := s.mgr.Add(xdsServer); err != nil {
		return err
	}

	notifier := &leadership.Notifier{
		ToNotify: append([]leadership.NeedLeaderElectionNotification{
			contourHandler,
			observer,
		}, needsNotification...),
	}
	if err := s.mgr.Add(notifier); err != nil {
		return err
	}

	// GO!
	return s.mgr.Start(signals.SetupSignalHandler())
}

func (s *Server) setupRateLimitService(contourConfiguration contour_api_v1alpha1.ContourConfigurationSpec) (*xdscache_v3.RateLimitConfig, error) {
	if contourConfiguration.RateLimitService == nil {
		return nil, nil
	}

	// ensure the specified ExtensionService exists
	extensionSvc := &contour_api_v1alpha1.ExtensionService{}
	key := client.ObjectKey{
		Namespace: contourConfiguration.RateLimitService.ExtensionService.Namespace,
		Name:      contourConfiguration.RateLimitService.ExtensionService.Name,
	}

	// Using GetAPIReader() here because the manager's caches won't be started yet,
	// so reads from the manager's client (which uses the caches for reads) will fail.
	if err := s.mgr.GetAPIReader().Get(context.Background(), key, extensionSvc); err != nil {
		return nil, fmt.Errorf("error getting rate limit extension service %s: %v", key, err)
	}

	// get the response timeout from the ExtensionService
	var responseTimeout timeout.Setting
	var err error

	if tp := extensionSvc.Spec.TimeoutPolicy; tp != nil {
		responseTimeout, err = timeout.Parse(tp.Response)
		if err != nil {
			return nil, fmt.Errorf("error parsing rate limit extension service %s response timeout: %v", key, err)
		}
	}

	var sni string
	if extensionSvc.Spec.UpstreamValidation != nil {
		sni = extensionSvc.Spec.UpstreamValidation.SubjectName
	}

	return &xdscache_v3.RateLimitConfig{
		ExtensionService:            key,
		SNI:                         sni,
		Domain:                      contourConfiguration.RateLimitService.Domain,
		Timeout:                     responseTimeout,
		FailOpen:                    ref.Val(contourConfiguration.RateLimitService.FailOpen, false),
		EnableXRateLimitHeaders:     ref.Val(contourConfiguration.RateLimitService.EnableXRateLimitHeaders, false),
		EnableResourceExhaustedCode: ref.Val(contourConfiguration.RateLimitService.EnableResourceExhaustedCode, false),
	}, nil
}

func (s *Server) setupGlobalExternalAuthentication(contourConfiguration contour_api_v1alpha1.ContourConfigurationSpec) (*xdscache_v3.GlobalExternalAuthConfig, error) {
	if contourConfiguration.GlobalExternalAuthorization == nil {
		return nil, nil
	}

	// ensure the specified ExtensionService exists
	extensionSvc := &contour_api_v1alpha1.ExtensionService{}

	key := client.ObjectKey{
		Namespace: contourConfiguration.GlobalExternalAuthorization.ExtensionServiceRef.Namespace,
		Name:      contourConfiguration.GlobalExternalAuthorization.ExtensionServiceRef.Name,
	}

	// Using GetAPIReader() here because the manager's caches won't be started yet,
	// so reads from the manager's client (which uses the caches for reads) will fail.
	if err := s.mgr.GetAPIReader().Get(context.Background(), key, extensionSvc); err != nil {
		return nil, fmt.Errorf("error getting global external authorization extension service %s: %v", key, err)
	}

	// get the response timeout from the ExtensionService
	var responseTimeout timeout.Setting
	var err error

	if tp := extensionSvc.Spec.TimeoutPolicy; tp != nil {
		responseTimeout, err = timeout.Parse(tp.Response)
		if err != nil {
			return nil, fmt.Errorf("error parsing global http ext auth extension service %s response timeout: %v", key, err)
		}
	}

	var sni string
	if extensionSvc.Spec.UpstreamValidation != nil {
		sni = extensionSvc.Spec.UpstreamValidation.SubjectName
	}

	var context map[string]string
	if contourConfiguration.GlobalExternalAuthorization.AuthPolicy.Context != nil {
		context = contourConfiguration.GlobalExternalAuthorization.AuthPolicy.Context
	}

	globalExternalAuthConfig := &xdscache_v3.GlobalExternalAuthConfig{
		ExtensionService: key,
		SNI:              sni,
		Timeout:          responseTimeout,
		FailOpen:         contourConfiguration.GlobalExternalAuthorization.FailOpen,
		Context:          context,
	}

	if contourConfiguration.GlobalExternalAuthorization.WithRequestBody != nil {
		globalExternalAuthConfig.WithRequestBody = &dag.AuthorizationServerBufferSettings{
			PackAsBytes:         contourConfiguration.GlobalExternalAuthorization.WithRequestBody.PackAsBytes,
			AllowPartialMessage: contourConfiguration.GlobalExternalAuthorization.WithRequestBody.AllowPartialMessage,
			MaxRequestBytes:     contourConfiguration.GlobalExternalAuthorization.WithRequestBody.MaxRequestBytes,
		}
	}
	return globalExternalAuthConfig, nil
}

func (s *Server) setupDebugService(debugConfig contour_api_v1alpha1.DebugConfig, builder *dag.Builder) error {
	debugsvc := &debug.Service{
		Service: httpsvc.Service{
			Addr:        debugConfig.Address,
			Port:        debugConfig.Port,
			FieldLogger: s.log.WithField("context", "debugsvc"),
		},
		Builder: builder,
	}
	return s.mgr.Add(debugsvc)
}

type xdsServer struct {
	log             logrus.FieldLogger
	mgr             manager.Manager
	registry        *prometheus.Registry
	config          contour_api_v1alpha1.XDSServerConfig
	snapshotHandler *xdscache.SnapshotHandler
	resources       []xdscache.ResourceCache
}

func (x *xdsServer) NeedLeaderElection() bool {
	return false
}

func (x *xdsServer) Start(ctx context.Context) error {
	log := x.log.WithField("context", "xds")

	log.Printf("waiting for informer caches to sync")
	if !x.mgr.GetCache().WaitForCacheSync(ctx) {
		return errors.New("informer cache failed to sync")
	}
	log.Printf("informer caches synced")

	grpcServer := xds.NewServer(x.registry, grpcOptions(log, x.config.TLS)...)

	switch x.config.Type {
	case contour_api_v1alpha1.EnvoyServerType:
		v3cache := contour_xds_v3.NewSnapshotCache(false, log)
		x.snapshotHandler.AddSnapshotter(v3cache)
		contour_xds_v3.RegisterServer(envoy_server_v3.NewServer(ctx, v3cache, contour_xds_v3.NewRequestLoggingCallbacks(log)), grpcServer)
	case contour_api_v1alpha1.ContourServerType:
		contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(x.resources)...), grpcServer)
	default:
		// This can't happen due to config validation.
		log.Fatalf("invalid xDS server type %q", x.config.Type)
	}

	addr := net.JoinHostPort(x.config.Address, strconv.Itoa(x.config.Port))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	log = log.WithField("address", addr)
	if *x.config.TLS.Insecure {
		log = log.WithField("insecure", true)
	}

	log.Infof("started xDS server type: %q", x.config.Type)
	defer log.Info("stopped xDS server")

	go func() {
		<-ctx.Done()

		// We don't use GracefulStop here because envoy
		// has long-lived hanging xDS requests. There's no
		// mechanism to make those pending requests fail,
		// so we forcibly terminate the TCP sessions.
		grpcServer.Stop()
	}()

	return grpcServer.Serve(l)
}

// setupMetrics creates metrics service for Contour.
func (s *Server) setupMetrics(metricsConfig contour_api_v1alpha1.MetricsConfig, healthConfig contour_api_v1alpha1.HealthConfig,
	registry *prometheus.Registry) error {

	// Create metrics service and register with mgr.
	metricsvc := &httpsvc.Service{
		Addr:        metricsConfig.Address,
		Port:        metricsConfig.Port,
		FieldLogger: s.log.WithField("context", "metricsvc"),
		ServeMux:    http.ServeMux{},
	}

	metricsvc.ServeMux.Handle("/metrics", metrics.Handler(registry))

	if metricsConfig.TLS != nil {
		metricsvc.Cert = metricsConfig.TLS.CertFile
		metricsvc.Key = metricsConfig.TLS.KeyFile
		metricsvc.CABundle = metricsConfig.TLS.CAFile
	}

	if healthConfig.Address == metricsConfig.Address && healthConfig.Port == metricsConfig.Port {
		h := health.Handler(s.coreClient)
		metricsvc.ServeMux.Handle("/health", h)
		metricsvc.ServeMux.Handle("/healthz", h)
	}

	return s.mgr.Add(metricsvc)
}

func (s *Server) setupHealth(healthConfig contour_api_v1alpha1.HealthConfig,
	metricsConfig contour_api_v1alpha1.MetricsConfig) error {

	if healthConfig.Address != metricsConfig.Address || healthConfig.Port != metricsConfig.Port {
		healthsvc := &httpsvc.Service{
			Addr:        healthConfig.Address,
			Port:        healthConfig.Port,
			FieldLogger: s.log.WithField("context", "healthsvc"),
		}

		h := health.Handler(s.coreClient)
		healthsvc.ServeMux.Handle("/health", h)
		healthsvc.ServeMux.Handle("/healthz", h)

		return s.mgr.Add(healthsvc)
	}

	return nil
}

func (s *Server) setupGatewayAPI(contourConfiguration contour_api_v1alpha1.ContourConfigurationSpec,
	mgr manager.Manager, eventHandler *contour.EventRecorder, sh *k8s.StatusUpdateHandler) []leadership.NeedLeaderElectionNotification {

	needLeadershipNotification := []leadership.NeedLeaderElectionNotification{}

	// Check if GatewayAPI is configured.
	if contourConfiguration.Gateway != nil && (contourConfiguration.Gateway.GatewayRef != nil || len(contourConfiguration.Gateway.ControllerName) > 0) {
		switch {
		// If a specific gateway was specified, we don't need to run the
		// GatewayClass and Gateway controllers to determine which gateway
		// to process, we just need informers to get events.
		case contourConfiguration.Gateway.GatewayRef != nil:
			// Inform on GatewayClasses.
			if err := informOnResource(&gatewayapi_v1beta1.GatewayClass{}, eventHandler, mgr.GetCache()); err != nil {
				s.log.WithError(err).WithField("resource", "gatewayclasses").Fatal("failed to create informer")
			}

			// Inform on Gateways.
			if err := informOnResource(&gatewayapi_v1beta1.Gateway{}, eventHandler, mgr.GetCache()); err != nil {
				s.log.WithError(err).WithField("resource", "gateways").Fatal("failed to create informer")
			}
		// Otherwise, run the GatewayClass and Gateway controllers to determine
		// the appropriate gateway class and gateway to process.
		default:
			// Create and register the gatewayclass controller with the manager.
			gatewayClassControllerName := contourConfiguration.Gateway.ControllerName
			gwClass, err := controller.RegisterGatewayClassController(
				s.log.WithField("context", "gatewayclass-controller"),
				mgr,
				eventHandler,
				sh.Writer(),
				gatewayClassControllerName,
			)
			if err != nil {
				s.log.WithError(err).Fatal("failed to create gatewayclass-controller")
			}
			needLeadershipNotification = append(needLeadershipNotification, gwClass)

			// Create and register the NewGatewayController controller with the manager.
			gw, err := controller.RegisterGatewayController(
				s.log.WithField("context", "gateway-controller"),
				mgr,
				eventHandler,
				sh.Writer(),
				gatewayClassControllerName,
			)
			if err != nil {
				s.log.WithError(err).Fatal("failed to create gateway-controller")
			}
			needLeadershipNotification = append(needLeadershipNotification, gw)
		}

		// Create and register the HTTPRoute controller with the manager.
		if err := controller.RegisterHTTPRouteController(s.log.WithField("context", "httproute-controller"), mgr, eventHandler); err != nil {
			s.log.WithError(err).Fatal("failed to create httproute-controller")
		}

		// Create and register the TLSRoute controller with the manager.
		if err := controller.RegisterTLSRouteController(s.log.WithField("context", "tlsroute-controller"), mgr, eventHandler); err != nil {
			s.log.WithError(err).Fatal("failed to create tlsroute-controller")
		}

		// Create and register the GRPCRoute controller with the manager.
		if err := controller.RegisterGRPCRouteController(s.log.WithField("context", "grpcroute-controller"), mgr, eventHandler); err != nil {
			s.log.WithError(err).Fatal("failed to create grpcroute-controller")
		}

		// Inform on ReferenceGrants.
		if err := informOnResource(&gatewayapi_v1beta1.ReferenceGrant{}, eventHandler, mgr.GetCache()); err != nil {
			s.log.WithError(err).WithField("resource", "referencegrants").Fatal("failed to create informer")
		}

		// Inform on Namespaces.
		if err := informOnResource(&corev1.Namespace{}, eventHandler, mgr.GetCache()); err != nil {
			s.log.WithError(err).WithField("resource", "namespaces").Fatal("failed to create informer")
		}
	}
	return needLeadershipNotification
}

type dagBuilderConfig struct {
	ingressClassNames                  []string
	rootNamespaces                     []string
	gatewayControllerName              string
	gatewayRef                         *types.NamespacedName
	disablePermitInsecure              bool
	enableExternalNameService          bool
	dnsLookupFamily                    contour_api_v1alpha1.ClusterDNSFamilyType
	headersPolicy                      *contour_api_v1alpha1.PolicyConfig
	clientCert                         *types.NamespacedName
	fallbackCert                       *types.NamespacedName
	connectTimeout                     time.Duration
	client                             client.Client
	metrics                            *metrics.Metrics
	httpAddress                        string
	httpPort                           int
	httpsAddress                       string
	httpsPort                          int
	globalExternalAuthorizationService *contour_api_v1.AuthorizationServer
}

func (s *Server) getDAGBuilder(dbc dagBuilderConfig) *dag.Builder {

	var (
		requestHeadersPolicy       dag.HeadersPolicy
		responseHeadersPolicy      dag.HeadersPolicy
		applyHeaderPolicyToIngress bool
	)

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

		applyHeaderPolicyToIngress = *dbc.headersPolicy.ApplyToIngress
	}

	var requestHeadersPolicyIngress dag.HeadersPolicy
	var responseHeadersPolicyIngress dag.HeadersPolicy

	if applyHeaderPolicyToIngress {
		requestHeadersPolicyIngress = requestHeadersPolicy
		responseHeadersPolicyIngress = responseHeadersPolicy
	}

	s.log.Debugf("EnableExternalNameService is set to %t", dbc.enableExternalNameService)

	// Get the appropriate DAG processors.
	dagProcessors := []dag.Processor{
		// The listener processor has to go first since it
		// adds listeners which are roots of the DAG.
		&dag.ListenerProcessor{
			HTTPAddress:  dbc.httpAddress,
			HTTPPort:     dbc.httpPort,
			HTTPSAddress: dbc.httpsAddress,
			HTTPSPort:    dbc.httpsPort,
		},
		&dag.IngressProcessor{
			EnableExternalNameService: dbc.enableExternalNameService,
			FieldLogger:               s.log.WithField("context", "IngressProcessor"),
			ClientCertificate:         dbc.clientCert,
			RequestHeadersPolicy:      &requestHeadersPolicyIngress,
			ResponseHeadersPolicy:     &responseHeadersPolicyIngress,
			ConnectTimeout:            dbc.connectTimeout,
		},
		&dag.ExtensionServiceProcessor{
			// Note that ExtensionService does not support ExternalName, if it does get added,
			// need to bring EnableExternalNameService in here too.
			FieldLogger:       s.log.WithField("context", "ExtensionServiceProcessor"),
			ClientCertificate: dbc.clientCert,
			ConnectTimeout:    dbc.connectTimeout,
		},
		&dag.HTTPProxyProcessor{
			EnableExternalNameService:   dbc.enableExternalNameService,
			DisablePermitInsecure:       dbc.disablePermitInsecure,
			FallbackCertificate:         dbc.fallbackCert,
			DNSLookupFamily:             dbc.dnsLookupFamily,
			ClientCertificate:           dbc.clientCert,
			RequestHeadersPolicy:        &requestHeadersPolicy,
			ResponseHeadersPolicy:       &responseHeadersPolicy,
			ConnectTimeout:              dbc.connectTimeout,
			GlobalExternalAuthorization: dbc.globalExternalAuthorizationService,
		},
	}

	if len(dbc.gatewayControllerName) > 0 || dbc.gatewayRef != nil {
		dagProcessors = append(dagProcessors, &dag.GatewayAPIProcessor{
			EnableExternalNameService: dbc.enableExternalNameService,
			FieldLogger:               s.log.WithField("context", "GatewayAPIProcessor"),
			ConnectTimeout:            dbc.connectTimeout,
		})
	}

	var configuredSecretRefs []*types.NamespacedName
	if dbc.fallbackCert != nil {
		configuredSecretRefs = append(configuredSecretRefs, dbc.fallbackCert)
	}
	if dbc.clientCert != nil {
		configuredSecretRefs = append(configuredSecretRefs, dbc.clientCert)
	}

	builder := &dag.Builder{
		Source: dag.KubernetesCache{
			RootNamespaces:           dbc.rootNamespaces,
			IngressClassNames:        dbc.ingressClassNames,
			ConfiguredGatewayToCache: dbc.gatewayRef,
			ConfiguredSecretRefs:     configuredSecretRefs,
			FieldLogger:              s.log.WithField("context", "KubernetesCache"),
			Client:                   dbc.client,
			Metrics:                  dbc.metrics,
		},
		Processors: dagProcessors,
		Metrics:    dbc.metrics,
	}

	// govet complains about copying the sync.Once that's in the dag.KubernetesCache
	// but it's safe to ignore since this function is only called once.
	// nolint:govet
	return builder
}

func informOnResource(obj client.Object, handler cache.ResourceEventHandler, cache ctrl_cache.Cache) error {
	inf, err := cache.GetInformer(context.Background(), obj)
	if err != nil {
		return err
	}

	_, err = inf.AddEventHandler(handler)
	return err
}
