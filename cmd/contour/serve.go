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
	"k8s.io/utils/pointer"
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

// doServe runs the contour serve subcommand.
func doServe(log logrus.FieldLogger, ctx *serveContext) error {
	// Establish k8s core & dynamic client connections.
	clients, err := k8s.NewClients(ctx.Config.Kubeconfig, ctx.Config.InCluster)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// Convert the Contour ServeContext into a ContourConfiguration
	contourConfiguration := convertServeContext(ctx)

	// Set up workgroup runner.
	var g workgroup.Group

	scheme, err := k8s.NewContourScheme()
	if err != nil {
		log.WithError(err).Fatal("unable to create scheme")
	}

	// Get the ContourConfiguration CRD if specified
	if len(ctx.contourConfigurationName) > 0 {

		// Determine the name/namespace of the configuration resource utilizing the environment
		// variable "CONTOUR_NAMESPACE" which should exist on the Contour deployment.
		//
		// If the env variable is not present, it will return "" and still fail the lookup
		// of the ContourConfiguration in the cluster.
		namespacedName := types.NamespacedName{Name: ctx.contourConfigurationName, Namespace: os.Getenv("CONTOUR_NAMESPACE")}
		client := clients.DynamicClient().Resource(contour_api_v1alpha1.ContourConfigurationGVR).Namespace(namespacedName.Namespace)

		// ensure the specified ContourConfiguration exists
		res, err := client.Get(context.Background(), namespacedName.Name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("error getting contour configuration %s: %v", namespacedName, err)
		}

		var contourConfiguration contour_api_v1alpha1.ContourConfiguration
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &contourConfiguration); err != nil {
			return fmt.Errorf("error converting contour configuration %s: %v", namespacedName, err)
		}
	}

	// Instantiate a controller-runtime manager. We need this regardless of whether
	// we're running the Gateway API controllers or not, because we use its cache
	// everywhere.
	mgr, err := manager.New(controller_config.GetConfigOrDie(), manager.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.WithError(err).Fatal("unable to set up controller manager")
	}

	// Register the manager with the workgroup.
	g.AddContext(func(taskCtx context.Context) error {
		return mgr.Start(signals.SetupSignalHandler())
	})

	// informerNamespaces is a list of namespaces that we should start informers for.
	var informerNamespaces []string

	fallbackCert := contourConfiguration.HTTPProxy.FallbackCertificate.NamespacedNameOf()
	clientCert := contourConfiguration.Envoy.ClientCertificate.NamespacedNameOf()

	if len(contourConfiguration.HTTPProxy.RootNamespaces) > 0 {
		informerNamespaces = append(informerNamespaces, contourConfiguration.HTTPProxy.RootNamespaces...)

		// Add the FallbackCertificateNamespace to informerNamespaces if it isn't present.
		if !contains(informerNamespaces, fallbackCert.Namespace) && fallbackCert != nil {
			informerNamespaces = append(informerNamespaces, fallbackCert.Namespace)
			log.WithField("context", "fallback-certificate").
				Infof("fallback certificate namespace %q not defined in 'root-namespaces', adding namespace to watch",
					fallbackCert.Namespace)
		}

		// Add the client certificate namespace to informerNamespaces if it isn't present.
		if !contains(informerNamespaces, clientCert.Namespace) && clientCert != nil {
			informerNamespaces = append(informerNamespaces, clientCert.Namespace)
			log.WithField("context", "envoy-client-certificate").
				Infof("client certificate namespace %q not defined in 'root-namespaces', adding namespace to watch",
					clientCert.Namespace)
		}
	}

	// Set up Prometheus registry and register base metrics.
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	registry.MustRegister(collectors.NewGoCollector())

	// Before we can build the event handler, we need to initialize the converter we'll
	// use to convert from Unstructured.
	converter, err := k8s.NewUnstructuredConverter()
	if err != nil {
		return err
	}

	var connectionIdleTimeout timeout.Setting
	var streamIdleTimeout timeout.Setting
	var delayedCloseTimeout timeout.Setting
	var maxConnectionDuration timeout.Setting
	var connectionShutdownGracePeriod timeout.Setting
	var requestTimeout timeout.Setting

	if contourConfiguration.Envoy.Timeouts != nil {
		if contourConfiguration.Envoy.Timeouts.ConnectionIdleTimeout != nil {
			connectionIdleTimeout, err = timeout.Parse(*contourConfiguration.Envoy.Timeouts.ConnectionIdleTimeout)
			if err != nil {
				return fmt.Errorf("error parsing connection idle timeout: %w", err)
			}
		}
		if contourConfiguration.Envoy.Timeouts.StreamIdleTimeout != nil {
			streamIdleTimeout, err = timeout.Parse(*contourConfiguration.Envoy.Timeouts.StreamIdleTimeout)
			if err != nil {
				return fmt.Errorf("error parsing stream idle timeout: %w", err)
			}
		}
		if contourConfiguration.Envoy.Timeouts.DelayedCloseTimeout != nil {
			delayedCloseTimeout, err = timeout.Parse(*contourConfiguration.Envoy.Timeouts.DelayedCloseTimeout)
			if err != nil {
				return fmt.Errorf("error parsing delayed close timeout: %w", err)
			}
		}
		if contourConfiguration.Envoy.Timeouts.MaxConnectionDuration != nil {
			maxConnectionDuration, err = timeout.Parse(*contourConfiguration.Envoy.Timeouts.MaxConnectionDuration)
			if err != nil {
				return fmt.Errorf("error parsing max connection duration: %w", err)
			}
		}
		if contourConfiguration.Envoy.Timeouts.ConnectionShutdownGracePeriod != nil {
			connectionShutdownGracePeriod, err = timeout.Parse(*contourConfiguration.Envoy.Timeouts.ConnectionShutdownGracePeriod)
			if err != nil {
				return fmt.Errorf("error parsing connection shutdown grace period: %w", err)
			}
		}
		if contourConfiguration.Envoy.Timeouts.RequestTimeout != nil {
			requestTimeout, err = timeout.Parse(*contourConfiguration.Envoy.Timeouts.RequestTimeout)
			if err != nil {
				return fmt.Errorf("error parsing request timeout: %w", err)
			}
		}
	}

	// connection balancer
	if ok := contourConfiguration.Envoy.Listener.ConnectionBalancer == "exact" || contourConfiguration.Envoy.Listener.ConnectionBalancer == ""; !ok {
		log.Warnf("Invalid listener connection balancer value %q. Only 'exact' connection balancing is supported for now.", contourConfiguration.Envoy.Listener.ConnectionBalancer)
		contourConfiguration.Envoy.Listener.ConnectionBalancer = ""
	}

	accessLogFormatString := ""
	if contourConfiguration.Envoy.Logging.AccessLogFormatString != nil {
		accessLogFormatString = *contourConfiguration.Envoy.Logging.AccessLogFormatString
	}

	cipherSuites := []string{}
	for _, cs := range contourConfiguration.Envoy.Listener.TLS.CipherSuites {
		cipherSuites = append(cipherSuites, string(cs))
	}

	listenerConfig := xdscache_v3.ListenerConfig{
		UseProxyProto: contourConfiguration.Envoy.Listener.UseProxyProto,
		HTTPListeners: map[string]xdscache_v3.Listener{
			"ingress_http": {
				Name:    "ingress_http",
				Address: contourConfiguration.Envoy.HTTPListener.Address,
				Port:    contourConfiguration.Envoy.HTTPListener.Port,
			},
		},
		HTTPSListeners: map[string]xdscache_v3.Listener{
			"ingress_https": {
				Name:    "ingress_https",
				Address: contourConfiguration.Envoy.HTTPSListener.Address,
				Port:    contourConfiguration.Envoy.HTTPSListener.Port,
			},
		},
		HTTPAccessLog:                 contourConfiguration.Envoy.HTTPListener.AccessLog,
		HTTPSAccessLog:                contourConfiguration.Envoy.HTTPSListener.AccessLog,
		AccessLogType:                 contourConfiguration.Envoy.Logging.AccessLogFormat,
		AccessLogFields:               contourConfiguration.Envoy.Logging.AccessLogFields,
		AccessLogFormatString:         accessLogFormatString,
		AccessLogFormatterExtensions:  AccessLogFormatterExtensions(contourConfiguration.Envoy.Logging.AccessLogFormat, contourConfiguration.Envoy.Logging.AccessLogFields, accessLogFormatString),
		MinimumTLSVersion:             annotation.MinTLSVersion(contourConfiguration.Envoy.Listener.TLS.MinimumProtocolVersion, "1.2"),
		CipherSuites:                  config.SanitizeCipherSuites(cipherSuites),
		RequestTimeout:                requestTimeout,
		ConnectionIdleTimeout:         connectionIdleTimeout,
		StreamIdleTimeout:             streamIdleTimeout,
		DelayedCloseTimeout:           delayedCloseTimeout,
		MaxConnectionDuration:         maxConnectionDuration,
		ConnectionShutdownGracePeriod: connectionShutdownGracePeriod,
		DefaultHTTPVersions:           parseDefaultHTTPVersions(contourConfiguration.Envoy.DefaultHTTPVersions),
		AllowChunkedLength:            !contourConfiguration.Envoy.Listener.DisableAllowChunkedLength,
		XffNumTrustedHops:             contourConfiguration.Envoy.Network.XffNumTrustedHops,
		ConnectionBalancer:            contourConfiguration.Envoy.Listener.ConnectionBalancer,
	}

	if contourConfiguration.RateLimitService != nil {
		namespacedName := contourConfiguration.RateLimitService.ExtensionService.NamespacedNameOf()
		client := clients.DynamicClient().Resource(contour_api_v1alpha1.ExtensionServiceGVR).Namespace(namespacedName.Namespace)

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
	}

	contourMetrics := metrics.NewMetrics(registry)

	// Endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	endpointHandler := xdscache_v3.NewEndpointsTranslator(log.WithField("context", "endpointstranslator"))

	resources := []xdscache.ResourceCache{
		xdscache_v3.NewListenerCache(listenerConfig, contourConfiguration.Envoy.Metrics.Address, contourConfiguration.Envoy.Metrics.Port, contourConfiguration.Envoy.Network.EnvoyAdminPort),
		&xdscache_v3.SecretCache{},
		&xdscache_v3.RouteCache{},
		&xdscache_v3.ClusterCache{},
		endpointHandler,
	}

	// snapshotHandler is used to produce new snapshots when the internal state changes for any xDS resource.
	snapshotHandler := xdscache.NewSnapshotHandler(resources, log.WithField("context", "snapshotHandler"))

	// register observer for endpoints updates.
	endpointHandler.Observer = contour.ComposeObservers(snapshotHandler)

	// Log that we're using the fallback certificate if configured.
	if fallbackCert != nil {
		log.WithField("context", "fallback-certificate").Infof("enabled fallback certificate with secret: %q", fallbackCert)
	}
	if clientCert != nil {
		log.WithField("context", "envoy-client-certificate").Infof("enabled client certificate with secret: %q", clientCert)
	}

	ingressClassName := ""
	if contourConfiguration.Ingress.ClassName != nil {
		ingressClassName = *contourConfiguration.Ingress.ClassName
	}

	// Build the core Kubernetes event handler.
	contourHandler := &contour.EventHandler{
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		Observer:        dag.ComposeObservers(append(xdscache.ObserversOf(resources), snapshotHandler)...),
		Builder: getDAGBuilder(
			ingressClassName,
			contourConfiguration.HTTPProxy.RootNamespaces,
			contourConfiguration.Gateway != nil,
			contourConfiguration.HTTPProxy.DisablePermitInsecure,
			contourConfiguration.EnableExternalNameService,
			contourConfiguration.Envoy.Cluster.DNSLookupFamily,
			contourConfiguration.Policy.RequestHeadersPolicy,
			contourConfiguration.Policy.ResponseHeadersPolicy,
			clients, clientCert, fallbackCert, log),
		FieldLogger: log.WithField("context", "contourEventHandler"),
	}

	// Wrap contourHandler in an EventRecorder which tracks API server events.
	eventHandler := &contour.EventRecorder{
		Next:    contourHandler,
		Counter: contourMetrics.EventHandlerOperations,
	}

	// Register leadership election.
	if contourConfiguration.LeaderElection.DisableLeaderElection {
		contourHandler.IsLeader = disableLeaderElection(log)
	} else {
		contourHandler.IsLeader = setupLeadershipElection(&g, log, contourConfiguration.LeaderElection, clients, contourHandler.UpdateNow)
	}

	// Start setting up StatusUpdateHandler since we need it in
	// the Gateway API controllers. Will finish setting it up and
	// start it later.
	sh := k8s.StatusUpdateHandler{
		Log:       log.WithField("context", "StatusUpdateHandler"),
		Clients:   clients,
		Cache:     mgr.GetCache(),
		Converter: converter,
	}

	// Inform on DefaultResources.
	for _, r := range k8s.DefaultResources() {
		if err := informOnResource(clients, r, eventHandler, mgr.GetCache()); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	for _, r := range k8s.IngressV1Resources() {
		if err := informOnResource(clients, r, eventHandler, mgr.GetCache()); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Only inform on Gateway API resources if Gateway API is found.
	if contourConfiguration.Gateway != nil {
		if clients.ResourcesExist(k8s.GatewayAPIResources()...) {
			// Create and register the gatewayclass controller with the manager.
			gatewayClassControllerName := contourConfiguration.Gateway.ControllerName
			if _, err := controller.NewGatewayClassController(
				mgr,
				eventHandler,
				sh.Writer(),
				log.WithField("context", "gatewayclass-controller"),
				gatewayClassControllerName,
				contourHandler.IsLeader,
			); err != nil {
				log.WithError(err).Fatal("failed to create gatewayclass-controller")
			}

			// Create and register the NewGatewayController controller with the manager.
			if _, err := controller.NewGatewayController(
				mgr,
				eventHandler,
				sh.Writer(),
				log.WithField("context", "gateway-controller"),
				gatewayClassControllerName,
				contourHandler.IsLeader,
			); err != nil {
				log.WithError(err).Fatal("failed to create gateway-controller")
			}

			// Create and register the NewHTTPRouteController controller with the manager.
			if _, err := controller.NewHTTPRouteController(mgr, eventHandler, log.WithField("context", "httproute-controller")); err != nil {
				log.WithError(err).Fatal("failed to create httproute-controller")
			}

			// Create and register the NewTLSRouteController controller with the manager.
			if _, err := controller.NewTLSRouteController(mgr, eventHandler, log.WithField("context", "tlsroute-controller")); err != nil {
				log.WithError(err).Fatal("failed to create tlsroute-controller")
			}

			// Inform on Namespaces.
			if err := informOnResource(clients, k8s.NamespacesResource(), eventHandler, mgr.GetCache()); err != nil {
				log.WithError(err).WithField("resource", k8s.NamespacesResource()).Fatal("failed to create informer")
			}
		} else {
			log.Fatalf("Gateway API Gateway configured but APIs not installed in cluster.")
		}
	}

	// Inform on secrets, filtering by root namespaces.
	for _, r := range k8s.SecretsResources() {
		var handler cache.ResourceEventHandler = eventHandler

		// If root namespaces are defined, filter for secrets in only those namespaces.
		if len(informerNamespaces) > 0 {
			handler = k8s.NewNamespaceFilter(informerNamespaces, eventHandler)
		}

		if err := informOnResource(clients, r, handler, mgr.GetCache()); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Inform on endpoints.
	for _, r := range k8s.EndpointsResources() {
		if err := informOnResource(clients, r, &contour.EventRecorder{
			Next:    endpointHandler,
			Counter: contourMetrics.EventHandlerOperations,
		}, mgr.GetCache()); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Register our event handler with the workgroup.
	g.Add(contourHandler.Start())

	// Create metrics service and register with workgroup.
	metricsvc := httpsvc.Service{
		Addr:        contourConfiguration.Metrics.Address,
		Port:        contourConfiguration.Metrics.Port,
		FieldLogger: log.WithField("context", "metricsvc"),
		ServeMux:    http.ServeMux{},
	}

	metricsvc.ServeMux.Handle("/metrics", metrics.Handler(registry))

	if contourConfiguration.Health.Address == contourConfiguration.Metrics.Address && contourConfiguration.Health.Port == contourConfiguration.Metrics.Port {
		h := health.Handler(clients.ClientSet())
		metricsvc.ServeMux.Handle("/health", h)
		metricsvc.ServeMux.Handle("/healthz", h)
	}

	g.Add(metricsvc.Start)

	// Create a separate health service if required.
	if contourConfiguration.Health.Address != contourConfiguration.Metrics.Address || contourConfiguration.Health.Port != contourConfiguration.Metrics.Port {
		healthsvc := httpsvc.Service{
			Addr:        contourConfiguration.Health.Address,
			Port:        contourConfiguration.Health.Port,
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
			Addr:        contourConfiguration.Debug.Address,
			Port:        contourConfiguration.Debug.Port,
			FieldLogger: log.WithField("context", "debugsvc"),
		},
		Builder: &contourHandler.Builder,
	}
	g.Add(debugsvc.Start)

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
	g.Add(sh.Start)

	// Now we have the statusUpdateHandler, we can create the event handler's StatusUpdater, which will take the
	// status updates from the DAG, and send them to the status update handler.
	contourHandler.StatusUpdater = sh.Writer()

	// Set up ingress load balancer status writer.
	lbsw := loadBalancerStatusWriter{
		log:              log.WithField("context", "loadBalancerStatusWriter"),
		cache:            mgr.GetCache(),
		isLeader:         contourHandler.IsLeader,
		lbStatus:         make(chan corev1.LoadBalancerStatus, 1),
		ingressClassName: ingressClassName,
		statusUpdater:    sh.Writer(),
		Converter:        converter,
	}
	g.Add(lbsw.Start)

	// Register an informer to watch envoy's service if we haven't been given static details.
	if contourConfiguration.Ingress.StatusAddress != nil {
		log.WithField("loadbalancer-address", *contourConfiguration.Ingress.StatusAddress).Info("Using supplied information for Ingress status")
		lbsw.lbStatus <- parseStatusFlag(*contourConfiguration.Ingress.StatusAddress)
	} else {
		serviceHandler := &k8s.ServiceStatusLoadBalancerWatcher{
			ServiceName: contourConfiguration.Envoy.Service.Name,
			LBStatus:    lbsw.lbStatus,
			Log:         log.WithField("context", "serviceStatusLoadBalancerWatcher"),
		}

		for _, r := range k8s.ServicesResources() {
			var handler cache.ResourceEventHandler = serviceHandler

			if contourConfiguration.Envoy.Service.Namespace != "" {
				handler = k8s.NewNamespaceFilter([]string{contourConfiguration.Envoy.Service.Namespace}, handler)
			}

			if err := informOnResource(clients, r, handler, mgr.GetCache()); err != nil {
				log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
			}
		}

		log.WithField("envoy-service-name", contourConfiguration.Envoy.Service.Name).
			WithField("envoy-service-namespace", contourConfiguration.Envoy.Service.Namespace).
			Info("Watching Service for Ingress status")
	}

	g.AddContext(func(taskCtx context.Context) error {
		log := log.WithField("context", "xds")

		log.Printf("waiting for informer caches to sync")
		if !mgr.GetCache().WaitForCacheSync(taskCtx) {
			return errors.New("informer cache failed to sync")
		}
		log.Printf("informer caches synced")

		grpcServer := xds.NewServer(registry, ctx.grpcOptions(log)...)

		switch contourConfiguration.XDSServer.Type {
		case contour_api_v1alpha1.EnvoyServerType:
			v3cache := contour_xds_v3.NewSnapshotCache(false, log)
			snapshotHandler.AddSnapshotter(v3cache)
			contour_xds_v3.RegisterServer(envoy_server_v3.NewServer(taskCtx, v3cache, contour_xds_v3.NewRequestLoggingCallbacks(log)), grpcServer)
		case contour_api_v1alpha1.ContourServerType:
			contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(resources)...), grpcServer)
		default:
			// This can't happen due to config validation.
			log.Fatalf("invalid xDS server type %q", contourConfiguration.XDSServer.Type)
		}

		addr := net.JoinHostPort(contourConfiguration.XDSServer.Address, strconv.Itoa(contourConfiguration.XDSServer.Port))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}

		log = log.WithField("address", addr)
		if tls := contourConfiguration.XDSServer.TLS; tls != nil {
			if tls.Insecure {
				log = log.WithField("insecure", true)
			}
		}

		log.Infof("started xDS server type: %q", contourConfiguration.XDSServer.Type)
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

func getDAGBuilder(ingressClassName string, rootNamespaces []string, gatewayAPIConfigured bool, disablePermitInsecure bool, enableExternalNameService bool,
	dnsLookupFamily contour_api_v1alpha1.ClusterDNSFamilyType, requestHP *contour_api_v1alpha1.HeadersPolicy, responseHP *contour_api_v1alpha1.HeadersPolicy,
	clients *k8s.Clients, clientCert, fallbackCert *types.NamespacedName, log logrus.FieldLogger) dag.Builder {

	var requestHeadersPolicy dag.HeadersPolicy
	var responseHeadersPolicy dag.HeadersPolicy

	if requestHP != nil {
		if requestHP.Set != nil {
			requestHeadersPolicy.Set = make(map[string]string)
			for k, v := range requestHP.Set {
				requestHeadersPolicy.Set[k] = v
			}
		}
		if requestHP.Remove != nil {
			requestHeadersPolicy.Remove = make([]string, 0, len(requestHP.Remove))
			requestHeadersPolicy.Remove = append(requestHeadersPolicy.Remove, requestHP.Remove...)
		}
	}

	if responseHP != nil {
		if responseHP.Set != nil {
			responseHeadersPolicy.Set = make(map[string]string)
			for k, v := range responseHP.Set {
				responseHeadersPolicy.Set[k] = v
			}
		}
		if responseHP.Remove != nil {
			responseHeadersPolicy.Remove = make([]string, 0, len(responseHP.Remove))
			responseHeadersPolicy.Remove = append(responseHeadersPolicy.Remove, responseHP.Remove...)
		}
	}

	var requestHeadersPolicyIngress dag.HeadersPolicy
	var responseHeadersPolicyIngress dag.HeadersPolicy
	if ctx.Config.Policy.ApplyToIngress {
		requestHeadersPolicyIngress = requestHeadersPolicy
		responseHeadersPolicyIngress = responseHeadersPolicy
	}

	log.Debugf("EnableExternalNameService is set to %t", enableExternalNameService)

	// Get the appropriate DAG processors.
	dagProcessors := []dag.Processor{
		&dag.IngressProcessor{
			EnableExternalNameService: enableExternalNameService,
			FieldLogger:               log.WithField("context", "IngressProcessor"),
			ClientCertificate:         clientCert,
			RequestHeadersPolicy:      &requestHeadersPolicyIngress,
			ResponseHeadersPolicy:     &responseHeadersPolicyIngress,
		},
		&dag.ExtensionServiceProcessor{
			// Note that ExtensionService does not support ExternalName, if it does get added,
			// need to bring EnableExternalNameService in here too.
			FieldLogger:       log.WithField("context", "ExtensionServiceProcessor"),
			ClientCertificate: clientCert,
		},
		&dag.HTTPProxyProcessor{
			EnableExternalNameService: enableExternalNameService,
			DisablePermitInsecure:     disablePermitInsecure,
			FallbackCertificate:       fallbackCert,
			DNSLookupFamily:           dnsLookupFamily,
			ClientCertificate:         clientCert,
			RequestHeadersPolicy:      &requestHeadersPolicy,
			ResponseHeadersPolicy:     &responseHeadersPolicy,
		},
	}

	if gatewayAPIConfigured && clients.ResourcesExist(k8s.GatewayAPIResources()...) {
		dagProcessors = append(dagProcessors, &dag.GatewayAPIProcessor{
			EnableExternalNameService: enableExternalNameService,
			FieldLogger:               log.WithField("context", "GatewayAPIProcessor"),
		})
	}

	// The listener processor has to go last since it looks at
	// the output of the other processors.
	dagProcessors = append(dagProcessors, &dag.ListenerProcessor{})

	var configuredSecretRefs []*types.NamespacedName
	if fallbackCert != nil {
		configuredSecretRefs = append(configuredSecretRefs, fallbackCert)
	}
	if clientCert != nil {
		configuredSecretRefs = append(configuredSecretRefs, clientCert)
	}

	builder := dag.Builder{
		Source: dag.KubernetesCache{
			RootNamespaces:       rootNamespaces,
			IngressClassName:     ingressClassName,
			ConfiguredSecretRefs: configuredSecretRefs,
			FieldLogger:          log.WithField("context", "KubernetesCache"),
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

func convertServeContext(ctx *serveContext) contour_api_v1alpha1.ContourConfigurationSpec {
	ingress := &contour_api_v1alpha1.IngressConfig{}
	if len(ctx.ingressClassName) > 0 {
		ingress.ClassName = pointer.StringPtr(ctx.ingressClassName)
	}
	if len(ctx.Config.IngressStatusAddress) > 0 {
		ingress.StatusAddress = pointer.StringPtr(ctx.Config.IngressStatusAddress)
	}

	debugLogLevel := contour_api_v1alpha1.InfoLog
	switch ctx.Config.Debug {
	case true:
		debugLogLevel = contour_api_v1alpha1.DebugLog
	case false:
		debugLogLevel = contour_api_v1alpha1.InfoLog
	}

	var gateway *contour_api_v1alpha1.GatewayConfig
	if ctx.Config.GatewayConfig != nil {
		gateway = &contour_api_v1alpha1.GatewayConfig{
			ControllerName: ctx.Config.GatewayConfig.ControllerName,
		}
	}

	var cipherSuites []contour_api_v1alpha1.TLSCipherType
	for _, suite := range ctx.Config.TLS.CipherSuites {
		cipherSuites = append(cipherSuites, contour_api_v1alpha1.TLSCipherType(suite))
	}

	var accessLogFormat contour_api_v1alpha1.AccessLogType
	switch ctx.Config.AccessLogFormat {
	case config.EnvoyAccessLog:
		accessLogFormat = contour_api_v1alpha1.EnvoyAccessLog
	case config.JSONAccessLog:
		accessLogFormat = contour_api_v1alpha1.JSONAccessLog
	}

	var accessLogFields contour_api_v1alpha1.AccessLogFields
	for _, alf := range ctx.Config.AccessLogFields {
		accessLogFields = append(accessLogFields, alf)
	}

	var defaultHTTPVersions []contour_api_v1alpha1.HTTPVersionType
	for _, version := range ctx.Config.DefaultHTTPVersions {
		defaultHTTPVersions = append(defaultHTTPVersions, contour_api_v1alpha1.HTTPVersionType(version))
	}

	timeoutParams := &contour_api_v1alpha1.TimeoutParameters{}
	if len(ctx.Config.Timeouts.RequestTimeout) > 0 {
		timeoutParams.RequestTimeout = pointer.StringPtr(ctx.Config.Timeouts.RequestTimeout)
	}
	if len(ctx.Config.Timeouts.ConnectionIdleTimeout) > 0 {
		timeoutParams.ConnectionIdleTimeout = pointer.StringPtr(ctx.Config.Timeouts.ConnectionIdleTimeout)
	}
	if len(ctx.Config.Timeouts.StreamIdleTimeout) > 0 {
		timeoutParams.StreamIdleTimeout = pointer.StringPtr(ctx.Config.Timeouts.StreamIdleTimeout)
	}
	if len(ctx.Config.Timeouts.MaxConnectionDuration) > 0 {
		timeoutParams.MaxConnectionDuration = pointer.StringPtr(ctx.Config.Timeouts.MaxConnectionDuration)
	}
	if len(ctx.Config.Timeouts.DelayedCloseTimeout) > 0 {
		timeoutParams.DelayedCloseTimeout = pointer.StringPtr(ctx.Config.Timeouts.DelayedCloseTimeout)
	}
	if len(ctx.Config.Timeouts.ConnectionShutdownGracePeriod) > 0 {
		timeoutParams.ConnectionShutdownGracePeriod = pointer.StringPtr(ctx.Config.Timeouts.ConnectionShutdownGracePeriod)
	}

	var dnsLookupFamily contour_api_v1alpha1.ClusterDNSFamilyType
	switch ctx.Config.Cluster.DNSLookupFamily {
	case config.AutoClusterDNSFamily:
		dnsLookupFamily = contour_api_v1alpha1.AutoClusterDNSFamily
	case config.IPv6ClusterDNSFamily:
		dnsLookupFamily = contour_api_v1alpha1.IPv6ClusterDNSFamily
	case config.IPv4ClusterDNSFamily:
		dnsLookupFamily = contour_api_v1alpha1.IPv4ClusterDNSFamily
	}

	var rateLimitService *contour_api_v1alpha1.RateLimitServiceConfig
	if ctx.Config.RateLimitService.ExtensionService != "" {
		rateLimitService = &contour_api_v1alpha1.RateLimitServiceConfig{
			ExtensionService: contour_api_v1alpha1.NamespacedName{
				Name:      k8s.NamespacedNameFrom(ctx.Config.RateLimitService.ExtensionService).Name,
				Namespace: k8s.NamespacedNameFrom(ctx.Config.RateLimitService.ExtensionService).Namespace,
			},
			Domain:                  ctx.Config.RateLimitService.Domain,
			FailOpen:                ctx.Config.RateLimitService.FailOpen,
			EnableXRateLimitHeaders: ctx.Config.RateLimitService.EnableXRateLimitHeaders,
		}
	}

	policy := &contour_api_v1alpha1.PolicyConfig{
		RequestHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
			Set:    ctx.Config.Policy.RequestHeadersPolicy.Set,
			Remove: ctx.Config.Policy.RequestHeadersPolicy.Remove,
		},
		ResponseHeadersPolicy: &contour_api_v1alpha1.HeadersPolicy{
			Set:    ctx.Config.Policy.ResponseHeadersPolicy.Set,
			Remove: ctx.Config.Policy.ResponseHeadersPolicy.Remove,
		},
	}

	// Convert serveContext to a ContourConfiguration
	contourConfiguration := contour_api_v1alpha1.ContourConfigurationSpec{
		Ingress: ingress,
		Debug: contour_api_v1alpha1.DebugConfig{
			Address:                 ctx.debugAddr,
			Port:                    ctx.debugPort,
			DebugLogLevel:           debugLogLevel,
			KubernetesDebugLogLevel: ctx.KubernetesDebug,
		},
		Health: contour_api_v1alpha1.HealthConfig{
			Address: ctx.healthAddr,
			Port:    ctx.healthPort,
		},
		Envoy: contour_api_v1alpha1.EnvoyConfig{
			Listener: contour_api_v1alpha1.EnvoyListenerConfig{
				UseProxyProto:             ctx.useProxyProto,
				DisableAllowChunkedLength: ctx.Config.DisableAllowChunkedLength,
				ConnectionBalancer:        ctx.Config.Listener.ConnectionBalancer,
				TLS: contour_api_v1alpha1.EnvoyTLS{
					MinimumProtocolVersion: ctx.Config.TLS.MinimumProtocolVersion,
					CipherSuites:           cipherSuites,
				},
			},
			Service: contour_api_v1alpha1.NamespacedName{
				Name:      ctx.Config.EnvoyServiceName,
				Namespace: ctx.Config.EnvoyServiceNamespace,
			},
			HTTPListener: contour_api_v1alpha1.EnvoyListener{
				Address:   ctx.httpAddr,
				Port:      ctx.httpPort,
				AccessLog: ctx.httpAccessLog,
			},
			HTTPSListener: contour_api_v1alpha1.EnvoyListener{
				Address:   ctx.httpsAddr,
				Port:      ctx.httpsPort,
				AccessLog: ctx.httpsAccessLog,
			},
			Metrics: contour_api_v1alpha1.MetricsConfig{
				Address: ctx.statsAddr,
				Port:    ctx.statsPort,
			},
			ClientCertificate: &contour_api_v1alpha1.NamespacedName{
				Name:      ctx.Config.TLS.ClientCertificate.Name,
				Namespace: ctx.Config.TLS.ClientCertificate.Namespace,
			},
			Logging: contour_api_v1alpha1.EnvoyLogging{
				AccessLogFormat:       accessLogFormat,
				AccessLogFormatString: pointer.StringPtr(ctx.Config.AccessLogFormatString),
				AccessLogFields:       accessLogFields,
			},
			DefaultHTTPVersions: defaultHTTPVersions,
			Timeouts:            timeoutParams,
			Cluster: contour_api_v1alpha1.ClusterParameters{
				DNSLookupFamily: dnsLookupFamily,
			},
			Network: contour_api_v1alpha1.NetworkParameters{
				XffNumTrustedHops: ctx.Config.Network.XffNumTrustedHops,
				EnvoyAdminPort:    ctx.Config.Network.EnvoyAdminPort,
			},
		},
		Gateway: gateway,
		HTTPProxy: contour_api_v1alpha1.HTTPProxyConfig{
			DisablePermitInsecure: ctx.Config.DisablePermitInsecure,
			RootNamespaces:        ctx.proxyRootNamespaces(),
			FallbackCertificate: &contour_api_v1alpha1.NamespacedName{
				Name:      ctx.Config.TLS.FallbackCertificate.Name,
				Namespace: ctx.Config.TLS.FallbackCertificate.Namespace,
			},
		},
		LeaderElection: contour_api_v1alpha1.LeaderElectionConfig{
			LeaseDuration: ctx.Config.LeaderElection.LeaseDuration.String(),
			RenewDeadline: ctx.Config.LeaderElection.RenewDeadline.String(),
			RetryPeriod:   ctx.Config.LeaderElection.RetryPeriod.String(),
			Configmap: contour_api_v1alpha1.NamespacedName{
				Name:      ctx.Config.LeaderElection.Name,
				Namespace: ctx.Config.LeaderElection.Namespace,
			},
			DisableLeaderElection: ctx.DisableLeaderElection,
		},
		EnableExternalNameService: ctx.Config.EnableExternalNameService,
		RateLimitService:          rateLimitService,
		Policy:                    policy,
		Metrics: contour_api_v1alpha1.MetricsConfig{
			Address: ctx.metricsAddr,
			Port:    ctx.metricsPort,
		},
	}

	xdsServerType := contour_api_v1alpha1.ContourServerType
	switch ctx.Config.Server.XDSServerType {
	case config.EnvoyServerType:
		xdsServerType = contour_api_v1alpha1.EnvoyServerType
	}

	contourConfiguration.XDSServer = contour_api_v1alpha1.XDSServerConfig{
		Type:    xdsServerType,
		Address: ctx.xdsAddr,
		Port:    ctx.xdsPort,
		TLS: &contour_api_v1alpha1.TLS{
			CAFile:   ctx.caFile,
			CertFile: ctx.contourCert,
			KeyFile:  ctx.contourKey,
			Insecure: ctx.PermitInsecureGRPC,
		},
	}

	return contourConfiguration
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
	accessLogFormatString string) []string {
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
		if contains(accessLogFormatString, "REQ_WITHOUT_QUERY") {
			extensionsMap["envoy.formatter.req_without_query"] = true
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
