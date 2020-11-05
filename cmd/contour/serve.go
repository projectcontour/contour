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
	"strconv"
	"strings"
	"syscall"
	"time"

	envoy_auth_v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoy_server_v2 "github.com/envoyproxy/go-control-plane/pkg/server/v2"
	envoy_server_v3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
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
	contour_xds_v2 "github.com/projectcontour/contour/internal/xds/v2"
	contour_xds_v3 "github.com/projectcontour/contour/internal/xds/v3"
	"github.com/projectcontour/contour/internal/xdscache"
	xdscache_v2 "github.com/projectcontour/contour/internal/xdscache/v2"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	corev1 "k8s.io/api/core/v1"
	networking_api_v1beta1 "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
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

	serve.Flag("config-path", "Path to base configuration.").Short('c').Action(parseConfig).ExistingFileVar(&configFile)

	serve.Flag("incluster", "Use in cluster configuration.").BoolVar(&ctx.Config.InCluster)
	serve.Flag("kubeconfig", "Path to kubeconfig (if not in running inside a cluster).").StringVar(&ctx.Config.Kubeconfig)

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
	serve.Flag("ingress-status-address", "Address to set in Ingress object status.").StringVar(&ctx.Config.IngressStatusAddress)
	serve.Flag("envoy-http-access-log", "Envoy HTTP access log.").StringVar(&ctx.httpAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log.").StringVar(&ctx.httpsAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests.").StringVar(&ctx.httpAddr)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests.").StringVar(&ctx.httpsAddr)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests.").IntVar(&ctx.httpPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests.").IntVar(&ctx.httpsPort)
	serve.Flag("envoy-service-name", "Envoy Service Name.").StringVar(&ctx.Config.EnvoyServiceName)
	serve.Flag("envoy-service-namespace", "Envoy Service Namespace.").StringVar(&ctx.Config.EnvoyServiceNamespace)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners.").BoolVar(&ctx.useProxyProto)

	serve.Flag("accesslog-format", "Format for Envoy access logs.").StringVar((*string)(&ctx.Config.AccessLogFormat))
	serve.Flag("disable-leader-election", "Disable leader election mechanism.").BoolVar(&ctx.DisableLeaderElection)

	serve.Flag("debug", "Enable debug logging.").Short('d').BoolVar(&ctx.Config.Debug)
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

// warnTLS logs a warning for HTTPProxies/Ingresses that specify a minimum protocol
// version of 1.1, but the config file isn't explicitly allowing 1.1, since in a
// future release the config file default will bump to 1.2 which will change the behavior
// for these proxies.
//
// TODO(#3010): remove this function after the config file default has switched to 1.2.
func warnTLS(log logrus.FieldLogger, client dynamic.Interface, ctx *serveContext) {
	// if the config file specifies a valid minimum protocol version, behavior
	// won't change, so there's nothing to warn on.
	switch ctx.Config.TLS.MinimumProtocolVersion {
	case "1.1", "1.2", "1.3":
		return
	}

	getWarning := func(kindPlural string, items []string) string {
		template := "In an upcoming Contour release, TLS 1.1 will be globally disabled by default since it's end-of-life. The following %s currently allow " +
			"TLS 1.1: [%s]. You must either update the %s to have a minimum TLS protocol version of 1.2 (which is now the Contour default), or explicitly " +
			"allow TLS 1.1 to be used in Contour by setting \"tls.minimum-protocol-version\" to \"1.1\" in the Contour config file."

		return fmt.Sprintf(template, kindPlural, strings.Join(items, ", "), kindPlural)
	}

	// Check for & warn on HTTPProxies that have a minimum protocol version of 1.1.
	proxyList, err := client.Resource(contour_api_v1.HTTPProxyGVR).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Warnf("error listing HTTPProxies: %v", err)
	} else {
		var warn []string
		for _, p := range proxyList.Items {
			var proxy contour_api_v1.HTTPProxy
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(p.Object, &proxy); err != nil {
				log.Warnf("error converting HTTPProxy %s/%s from unstructured: %v", p.GetNamespace(), p.GetName(), err)
				continue
			}

			if proxy.Spec.VirtualHost == nil || proxy.Spec.VirtualHost.TLS == nil || proxy.Spec.VirtualHost.TLS.MinimumProtocolVersion != "1.1" {
				continue
			}

			warn = append(warn, k8s.NamespacedNameOf(&proxy).String())
		}

		if len(warn) > 0 {
			log.Warn(getWarning("HTTPProxies", warn))
		}
	}

	// Check for & warn on Ingresses that have a minimum protocol version of 1.1.
	ingressGVR := networking_api_v1beta1.SchemeGroupVersion.WithResource("ingresses")
	ingressList, err := client.Resource(ingressGVR).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		log.Warnf("error listing Ingresses: %v", err)
	} else {
		var warn []string
		for _, ing := range ingressList.Items {
			var ingress networking_api_v1beta1.Ingress
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(ing.Object, &ingress); err != nil {
				log.Warnf("error converting Ingress %s/%s from unstructured: %v", ing.GetNamespace(), ing.GetName(), err)
				continue
			}

			if !annotation.MatchesIngressClass(&ingress, ctx.ingressClass) {
				continue
			}

			if annotation.CompatAnnotation(&ingress, "tls-minimum-protocol-version") != "1.1" {
				continue
			}

			warn = append(warn, k8s.NamespacedNameOf(&ingress).String())
		}

		if len(warn) > 0 {
			log.Warn(getWarning("Ingresses", warn))
		}
	}
}

// doServe runs the contour serve subcommand.
func doServe(log logrus.FieldLogger, ctx *serveContext) error {
	// Establish k8s core & dynamic client connections.
	clients, err := k8s.NewClients(ctx.Config.Kubeconfig, ctx.Config.InCluster)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clients: %w", err)
	}

	// Validate that Contour CRDs have been updated to v1.
	validateCRDs(clients.DynamicClient(), log)

	// Warn on proxies/ingresses using TLS 1.1 without it being
	// explicitly allowed via the config file, since in an
	// upcoming Contour release, TLS 1.1 will be disallowed by
	// default.
	warnTLS(log, clients.DynamicClient(), ctx)

	// informerNamespaces is a list of namespaces that we should start informers for.
	var informerNamespaces []string

	fallbackCert := namespacedNameOf(ctx.Config.TLS.FallbackCertificate)
	clientCert := namespacedNameOf(ctx.Config.TLS.ClientCertificate)

	if rootNamespaces := ctx.proxyRootNamespaces(); len(rootNamespaces) > 0 {
		informerNamespaces = append(informerNamespaces, rootNamespaces...)

		// Add the FallbackCertificateNamespace to informerNamespaces if it isn't present.
		if !contains(informerNamespaces, ctx.Config.TLS.FallbackCertificate.Namespace) && fallbackCert != nil {
			informerNamespaces = append(informerNamespaces, ctx.Config.TLS.FallbackCertificate.Namespace)
			log.WithField("context", "fallback-certificate").
				Infof("fallback certificate namespace %q not defined in 'root-namespaces', adding namespace to watch",
					ctx.Config.TLS.FallbackCertificate.Namespace)
		}

		// Add the client certificate namespace to informerNamespaces if it isn't present.
		if !contains(informerNamespaces, ctx.Config.TLS.ClientCertificate.Namespace) && clientCert != nil {
			informerNamespaces = append(informerNamespaces, ctx.Config.TLS.ClientCertificate.Namespace)
			log.WithField("context", "envoy-client-certificate").
				Infof("client certificate namespace %q not defined in 'root-namespaces', adding namespace to watch",
					ctx.Config.TLS.ClientCertificate.Namespace)
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

	// XXX(jpeach) we know the config file validated, so all
	// the timeouts will parse. Shall we add a `timeout.MustParse()`
	// and use it here?

	connectionIdleTimeout, err := timeout.Parse(ctx.Config.Timeouts.ConnectionIdleTimeout)
	if err != nil {
		return fmt.Errorf("error parsing connection idle timeout: %w", err)
	}
	streamIdleTimeout, err := timeout.Parse(ctx.Config.Timeouts.StreamIdleTimeout)
	if err != nil {
		return fmt.Errorf("error parsing stream idle timeout: %w", err)
	}
	maxConnectionDuration, err := timeout.Parse(ctx.Config.Timeouts.MaxConnectionDuration)
	if err != nil {
		return fmt.Errorf("error parsing max connection duration: %w", err)
	}
	connectionShutdownGracePeriod, err := timeout.Parse(ctx.Config.Timeouts.ConnectionShutdownGracePeriod)
	if err != nil {
		return fmt.Errorf("error parsing connection shutdown grace period: %w", err)
	}
	requestTimeout, err := timeout.Parse(ctx.Config.Timeouts.RequestTimeout)
	if err != nil {
		return fmt.Errorf("error parsing request timeout: %w", err)
	}

	// Set the global minimum allowed TLS version to 1.1, which allows proxies/ingresses
	// that are explicitly using 1.1 to continue working by default. However, the
	// *default* minimum TLS version for proxies/ingresses that don't specify it
	// is 1.2, set in the DAG processors.
	globalMinTLSVersion := annotation.MinTLSVersion(ctx.Config.TLS.MinimumProtocolVersion, envoy_auth_v2.TlsParameters_TLSv1_1)

	listenerConfig := xdscache_v2.ListenerConfig{
		UseProxyProto:                 ctx.useProxyProto,
		HTTPAddress:                   ctx.httpAddr,
		HTTPPort:                      ctx.httpPort,
		HTTPAccessLog:                 ctx.httpAccessLog,
		HTTPSAddress:                  ctx.httpsAddr,
		HTTPSPort:                     ctx.httpsPort,
		HTTPSAccessLog:                ctx.httpsAccessLog,
		AccessLogType:                 ctx.Config.AccessLogFormat,
		AccessLogFields:               ctx.Config.AccessLogFields,
		MinimumTLSVersion:             globalMinTLSVersion,
		RequestTimeout:                requestTimeout,
		ConnectionIdleTimeout:         connectionIdleTimeout,
		StreamIdleTimeout:             streamIdleTimeout,
		MaxConnectionDuration:         maxConnectionDuration,
		ConnectionShutdownGracePeriod: connectionShutdownGracePeriod,
		DefaultHTTPVersions:           parseDefaultHTTPVersions(ctx.Config.DefaultHTTPVersions),
	}

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

	// snapshotHandler is used to produce new snapshots when the internal state changes for any xDS resource.
	snapshotHandler := xdscache.NewSnapshotHandler(resources, log.WithField("context", "snapshotHandler"))

	// register observer for endpoints updates.
	endpointHandler.Observer = contour.ComposeObservers(snapshotHandler)

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
					DisablePermitInsecure: ctx.Config.DisablePermitInsecure,
					FallbackCertificate:   fallbackCert,
					DNSLookupFamily:       ctx.Config.Cluster.DNSLookupFamily,
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
	dynamicHandler := k8s.DynamicClientHandler{
		Next: &contour.EventRecorder{
			Next:    eventHandler,
			Counter: contourMetrics.EventHandlerOperations,
		},
		Converter: converter,
		Logger:    log.WithField("context", "dynamicHandler"),
	}

	// Inform on DefaultResources.
	for _, r := range k8s.DefaultResources() {
		inf, err := clients.InformerForResource(r)
		if err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}

		inf.AddEventHandler(&dynamicHandler)
	}

	// Inform on service-apis types if they are present.
	if ctx.UseExperimentalServiceAPITypes {
		for _, r := range k8s.ServiceAPIResources() {
			if !clients.ResourcesExist(r) {
				log.WithField("resource", r).Warn("resource type not present on API server")
				continue
			}

			if err := informOnResource(clients, r, &dynamicHandler); err != nil {
				log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
			}
		}
	}

	// Inform on secrets, filtering by root namespaces.
	for _, r := range k8s.SecretsResources() {
		var handler cache.ResourceEventHandler = &dynamicHandler

		// If root namespaces are defined, filter for secrets in only those namespaces.
		if len(informerNamespaces) > 0 {
			handler = k8s.NewNamespaceFilter(informerNamespaces, &dynamicHandler)
		}

		if err := informOnResource(clients, r, handler); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Inform on endpoints.
	for _, r := range k8s.EndpointsResources() {
		if err := informOnResource(clients, r, &k8s.DynamicClientHandler{
			Next: &contour.EventRecorder{
				Next:    endpointHandler,
				Counter: contourMetrics.EventHandlerOperations,
			},
			Converter: converter,
			Logger:    log.WithField("context", "endpointstranslator"),
		}); err != nil {
			log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
		}
	}

	// Set up workgroup runner and register informers.
	var g workgroup.Group

	// Register a task to start all the informers.
	g.Add(func(stop <-chan struct{}) error {
		log := log.WithField("context", "informers")

		log.Info("starting informers")
		defer log.Println("stopped informers")

		if err := clients.StartInformers(stop); err != nil {
			log.WithError(err).Error("failed to start informers")
		}

		<-stop
		return nil
	})

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
	if ctx.DisableLeaderElection {
		eventHandler.IsLeader = disableLeaderElection(log)
	} else {
		eventHandler.IsLeader = setupLeadershipElection(&g, log, &ctx.Config.LeaderElection, clients, eventHandler.UpdateNow)
	}

	// Once we have the leadership detection channel, we can
	// push DAG rebuild metrics onto the observer stack.
	eventHandler.Observer = &contour.RebuildMetricsObserver{
		Metrics:      contourMetrics,
		IsLeader:     eventHandler.IsLeader,
		NextObserver: eventHandler.Observer,
	}

	sh := k8s.StatusUpdateHandler{
		Log:           log.WithField("context", "StatusUpdateHandler"),
		Clients:       clients,
		LeaderElected: eventHandler.IsLeader,
		Converter:     converter,
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
	if lbAddr := ctx.Config.IngressStatusAddress; lbAddr != "" {
		log.WithField("loadbalancer-address", lbAddr).Info("Using supplied information for Ingress status")
		lbsw.lbStatus <- parseStatusFlag(lbAddr)
	} else {
		dynamicServiceHandler := k8s.DynamicClientHandler{
			Next: &k8s.ServiceStatusLoadBalancerWatcher{
				ServiceName: ctx.Config.EnvoyServiceName,
				LBStatus:    lbsw.lbStatus,
				Log:         log.WithField("context", "serviceStatusLoadBalancerWatcher"),
			},
			Converter: converter,
			Logger:    log.WithField("context", "serviceStatusLoadBalancerWatcher"),
		}

		for _, r := range k8s.ServicesResources() {
			var handler cache.ResourceEventHandler = &dynamicServiceHandler

			if ctx.Config.EnvoyServiceNamespace != "" {
				handler = k8s.NewNamespaceFilter([]string{ctx.Config.EnvoyServiceNamespace}, handler)
			}

			if err := informOnResource(clients, r, handler); err != nil {
				log.WithError(err).WithField("resource", r).Fatal("failed to create informer")
			}
		}

		log.WithField("envoy-service-name", ctx.Config.EnvoyServiceName).
			WithField("envoy-service-namespace", ctx.Config.EnvoyServiceNamespace).
			Info("Watching Service for Ingress status")
	}

	g.Add(func(stop <-chan struct{}) error {
		log := log.WithField("context", "xds")

		log.Printf("waiting for informer caches to sync")
		if !clients.WaitForCacheSync(stop) {
			return errors.New("informer cache failed to sync")
		}
		log.Printf("informer caches synced")

		grpcServer := xds.NewServer(registry, ctx.grpcOptions(log)...)

		switch ctx.Config.Server.XDSServerType {
		case config.EnvoyServerType:
			v3cache := contour_xds_v3.NewSnapshotCache(false, log)
			snapshotHandler.AddSnapshotter(v3cache)
			contour_xds_v3.RegisterServer(envoy_server_v3.NewServer(context.Background(), v3cache, nil), grpcServer)

			// Check an internal feature flag to disable xDS v2 endpoints. This is strictly for testing.
			if config.GetenvOr("CONTOUR_INTERNAL_DISABLE_XDSV2", "N") == "N" {
				v2cache := contour_xds_v2.NewSnapshotCache(false, log)
				snapshotHandler.AddSnapshotter(v2cache)
				contour_xds_v2.RegisterServer(envoy_server_v2.NewServer(context.Background(), v2cache, nil), grpcServer)
			}
		case config.ContourServerType:
			contour_xds_v3.RegisterServer(contour_xds_v3.NewContourServer(log, xdscache.ResourcesOf(resources)...), grpcServer)

			// Check an internal feature flag to disable xDS v2 endpoints. This is strictly for testing.
			if config.GetenvOr("CONTOUR_INTERNAL_DISABLE_XDSV2", "N") == "N" {
				contour_xds_v2.RegisterServer(contour_xds_v2.NewContourServer(log, xdscache.ResourcesOf(resources)...), grpcServer)
			}
		default:
			// This can't happen due to config validation.
			log.Fatalf("invalid xDS server type %q", ctx.Config.Server.XDSServerType)
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

		log.Infof("started xDS server type: %q", ctx.Config.Server.XDSServerType)
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

func informOnResource(clients *k8s.Clients, gvr schema.GroupVersionResource, handler cache.ResourceEventHandler) error {
	inf, err := clients.InformerForResource(gvr)
	if err != nil {
		return err
	}

	inf.AddEventHandler(handler)
	return nil
}
