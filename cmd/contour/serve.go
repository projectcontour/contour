// Copyright © 2019 Heptio
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
	"net"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"
	contourinformers "github.com/heptio/contour/apis/generated/informers/externalversions"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/dag"
	"github.com/heptio/contour/internal/debug"
	cgrpc "github.com/heptio/contour/internal/grpc"
	"github.com/heptio/contour/internal/httpsvc"
	"github.com/heptio/contour/internal/k8s"
	"github.com/heptio/contour/internal/metrics"
	"github.com/heptio/contour/internal/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreinformers "k8s.io/client-go/informers"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

// registerServe registers the serve subcommand and flags
// with the Application provided.
func registerServe(app *kingpin.Application) (*kingpin.CmdClause, *serveContext) {
	serve := app.Command("serve", "Serve xDS API traffic")

	// The precedence of configuration for contour serve is as follows:
	// config file, overridden by env vars, overridden by cli flags.
	// however, as -c is a cli flag, we don't know its valye til cli flags
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

	serve.Flag("config-path", "path to base configuration").Short('c').Action(parseConfig).ExistingFileVar(&configFile)

	serve.Flag("incluster", "use in cluster configuration.").BoolVar(&ctx.InCluster)
	serve.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").StringVar(&ctx.Kubeconfig)

	serve.Flag("xds-address", "xDS gRPC API address").StringVar(&ctx.xdsAddr)
	serve.Flag("xds-port", "xDS gRPC API port").IntVar(&ctx.xdsPort)

	serve.Flag("stats-address", "Envoy /stats interface address").StringVar(&ctx.statsAddr)
	serve.Flag("stats-port", "Envoy /stats interface port").IntVar(&ctx.statsPort)

	serve.Flag("debug-http-address", "address the debug http endpoint will bind to").StringVar(&ctx.debugAddr)
	serve.Flag("debug-http-port", "port the debug http endpoint will bind to").IntVar(&ctx.debugPort)

	serve.Flag("http-address", "address the metrics http endpoint will bind to").StringVar(&ctx.metricsAddr)
	serve.Flag("http-port", "port the metrics http endpoint will bind to").IntVar(&ctx.metricsPort)

	serve.Flag("contour-cafile", "CA bundle file name for serving gRPC with TLS").Envar("CONTOUR_CAFILE").StringVar(&ctx.caFile)
	serve.Flag("contour-cert-file", "Contour certificate file name for serving gRPC over TLS").Envar("CONTOUR_CERT_FILE").StringVar(&ctx.contourCert)
	serve.Flag("contour-key-file", "Contour key file name for serving gRPC over TLS").Envar("CONTOUR_KEY_FILE").StringVar(&ctx.contourKey)
	serve.Flag("insecure", "Allow serving without TLS secured gRPC").BoolVar(&ctx.PermitInsecureGRPC)
	serve.Flag("ingressroute-root-namespaces", "Restrict contour to searching these namespaces for root ingress routes").StringVar(&ctx.rootNamespaces)

	serve.Flag("ingress-class-name", "Contour IngressClass name").StringVar(&ctx.ingressClass)

	serve.Flag("envoy-http-access-log", "Envoy HTTP access log").StringVar(&ctx.httpAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log").StringVar(&ctx.httpsAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests").StringVar(&ctx.httpAddr)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests").StringVar(&ctx.httpsAddr)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests").IntVar(&ctx.httpPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests").IntVar(&ctx.httpsPort)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners").BoolVar(&ctx.useProxyProto)

	serve.Flag("enable-leader-election", "Enable leader election mechanism").BoolVar(&ctx.EnableLeaderElection)
	return serve, ctx
}

// doServe runs the contour serve subcommand.
func doServe(log logrus.FieldLogger, ctx *serveContext) error {

	// step 1. establish k8s client connection
	client, contourClient, coordinationClient := newClient(ctx.Kubeconfig, ctx.InCluster)

	// step 2. create informers
	// note: 0 means resync timers are disabled
	coreInformers := coreinformers.NewSharedInformerFactory(client, 0)
	contourInformers := contourinformers.NewSharedInformerFactory(contourClient, 0)

	// Create a set of SharedInformerFactories for each root-ingressroute namespace (if defined)
	var namespacedInformers []coreinformers.SharedInformerFactory
	for _, namespace := range ctx.ingressRouteRootNamespaces() {
		inf := coreinformers.NewSharedInformerFactoryWithOptions(client, 0, coreinformers.WithNamespace(namespace))
		namespacedInformers = append(namespacedInformers, inf)
	}

	// step 3. establish our (poorly named) gRPC cache handler.
	ch := &contour.CacheHandler{
		ListenerVisitorConfig: contour.ListenerVisitorConfig{
			UseProxyProto:          ctx.useProxyProto,
			HTTPAddress:            ctx.httpAddr,
			HTTPPort:               ctx.httpPort,
			HTTPAccessLog:          ctx.httpAccessLog,
			HTTPSAddress:           ctx.httpsAddr,
			HTTPSPort:              ctx.httpsPort,
			HTTPSAccessLog:         ctx.httpsAccessLog,
			MinimumProtocolVersion: dag.MinProtoVersion(ctx.TLSConfig.MinimumProtocolVersion),
		},
		ListenerCache: contour.NewListenerCache(ctx.statsAddr, ctx.statsPort),
		FieldLogger:   log.WithField("context", "CacheHandler"),
		IngressRouteStatus: &k8s.IngressRouteStatus{
			Client: contourClient,
		},
	}

	// step 4. wrap the cache handler in a k8s event handler.
	eh := &contour.EventHandler{
		CacheHandler:    ch,
		HoldoffDelay:    100 * time.Millisecond,
		HoldoffMaxDelay: 500 * time.Millisecond,
		Builder: dag.Builder{
			Source: dag.KubernetesCache{
				IngressRouteRootNamespaces: ctx.ingressRouteRootNamespaces(),
				IngressClass:               ctx.ingressClass,
			},
			DisablePermitInsecure: ctx.DisablePermitInsecure,
		},
		FieldLogger: log.WithField("context", "contourEventHandler"),
	}

	// step 5. register our resource event handler with the k8s informers.
	coreInformers.Core().V1().Services().Informer().AddEventHandler(eh)
	coreInformers.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(eh)
	contourInformers.Contour().V1beta1().IngressRoutes().Informer().AddEventHandler(eh)
	contourInformers.Contour().V1beta1().TLSCertificateDelegations().Informer().AddEventHandler(eh)
	// Add informers for each root-ingressroute namespaces
	for _, inf := range namespacedInformers {
		inf.Core().V1().Secrets().Informer().AddEventHandler(eh)
	}
	// If root-ingressroutes are not defined, then add the informer for all namespaces
	if len(namespacedInformers) == 0 {
		coreInformers.Core().V1().Secrets().Informer().AddEventHandler(eh)
	}

	// step 6. endpoints updates are handled directly by the EndpointsTranslator
	// due to their high update rate and their orthogonal nature.
	et := &contour.EndpointsTranslator{
		FieldLogger: log.WithField("context", "endpointstranslator"),
	}
	coreInformers.Core().V1().Endpoints().Informer().AddEventHandler(et)

	// step 7. setup workgroup runner and register informers.
	var g workgroup.Group
	g.Add(startInformer(coreInformers, log.WithField("context", "coreinformers")))
	g.Add(startInformer(contourInformers, log.WithField("context", "contourinformers")))
	for _, inf := range namespacedInformers {
		g.Add(startInformer(inf, log.WithField("context", "corenamespacedinformers")))
	}

	// step 8. register our event handler with the workgroup
	g.Add(eh.Start())

	// step 9. setup prometheus registry and register base metrics.
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	// step 10. create metrics service and register with workgroup.
	metricsvc := metrics.Service{
		Service: httpsvc.Service{
			Addr:        ctx.metricsAddr,
			Port:        ctx.metricsPort,
			FieldLogger: log.WithField("context", "metricsvc"),
		},
		Client:   client,
		Registry: registry,
	}
	g.Add(metricsvc.Start)

	// step 11. create debug service and register with workgroup.
	debugsvc := debug.Service{
		Service: httpsvc.Service{
			Addr:        ctx.debugAddr,
			Port:        ctx.debugPort,
			FieldLogger: log.WithField("context", "debugsvc"),
		},
		Builder: &eh.Builder,
	}
	g.Add(debugsvc.Start)

	// step 12. Setup leader election

	// leaderOK will block gRPC startup until it's closed.
	leaderOK := make(chan struct{})
	// deposed is closed by the leader election callback when
	// we are deposed as leader so that we can clean up.
	deposed := make(chan struct{})

	// Set up the leader election
	// Generate the event recorder to send election events to the logs.
	// Set up the event bits
	eventBroadcaster := record.NewBroadcaster()
	// Broadcast election events to the config map
	eventBroadcaster.StartRecordingToSink(&clientcorev1.EventSinkImpl{Interface: clientcorev1.New(client.CoreV1().RESTClient()).Events("")})
	eventsScheme := runtime.NewScheme()
	// Need an eventsScheme
	err := coreV1.AddToScheme(eventsScheme)
	check(err)
	// The recorder is what will record events about the resource lock
	recorder := eventBroadcaster.NewRecorder(eventsScheme, coreV1.EventSource{Component: "contour"})

	// Figure out the resource lock ID
	resourceLockID, isPodNameEnvSet := os.LookupEnv("POD_NAME")
	if !isPodNameEnvSet {
		resourceLockID = uuid.New().String()
	}

	// Generate the resource lock.
	// TODO(youngnick) change this to a Lease object instead
	// of the configmap once the Lease API has been GA for a full support
	// cycle (ie nine months).
	rl, err := resourcelock.New(
		resourcelock.ConfigMapsResourceLock,
		ctx.LeaderElectionConfig.Namespace,
		ctx.LeaderElectionConfig.Name,
		client.CoreV1(),
		coordinationClient,
		resourcelock.ResourceLockConfig{
			Identity:      resourceLockID,
			EventRecorder: recorder,
		},
	)
	check(err)

	// Make the leader elector, ready to be used in the Workgroup.
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: ctx.LeaderElectionConfig.LeaseDuration,
		RenewDeadline: ctx.LeaderElectionConfig.RenewDeadline,
		RetryPeriod:   ctx.LeaderElectionConfig.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				log.WithFields(logrus.Fields{
					"lock":     rl.Describe(),
					"identity": rl.Identity(),
				}).Info("elected leader")
				close(leaderOK)
			},
			OnStoppedLeading: func() {
				// The context being canceled will trigger a handler that will
				// deal with being deposed.
				close(deposed)
			},
		},
	})
	check(err)
	// AddContext will generate its own context.Context and pass it in,
	// managing the close process when the other goroutines are closed.
	g.AddContext(func(electionCtx context.Context) {
		log := log.WithField("context", "leaderelection")
		if !ctx.EnableLeaderElection {
			log.Info("Leader election disabled")
			// if leader election is disabled, signal the gRPC goroutine
			// to start serving and finsh up this context.
			// The Workgroup will handle leaving this running until everything
			// else closes down.
			close(leaderOK)
			<-electionCtx.Done()
		}

		log.WithFields(logrus.Fields{
			"configmapname":      ctx.LeaderElectionConfig.Name,
			"configmapnamespace": ctx.LeaderElectionConfig.Namespace,
		}).Info("started")

		le.Run(electionCtx)
		log.Info("stopped")
	})

	g.Add(func(stop <-chan struct{}) error {
		// If we get deposed as leader, shut it down.
		log := log.WithField("context", "leaderelection-deposer")
		select {
		case <-stop:
			// shut down
			log.Info("stopped")
		case <-deposed:
			log.WithFields(logrus.Fields{
				"lock":     rl.Describe(),
				"identity": rl.Identity(),
			}).Info("deposed as leader, shutting down")
		}
		return nil
	})
	// step 13. register our custom metrics and plumb into cache handler
	// and resource event handler.
	metrics := metrics.NewMetrics(registry)
	ch.Metrics = metrics
	eh.Metrics = metrics

	// step 14. create grpc handler and register with workgroup.
	// This will block until the program becomes the leader.
	g.Add(func(stop <-chan struct{}) error {
		log := log.WithField("context", "grpc")
		resources := map[string]cgrpc.Resource{
			ch.ClusterCache.TypeURL():  &ch.ClusterCache,
			ch.RouteCache.TypeURL():    &ch.RouteCache,
			ch.ListenerCache.TypeURL(): &ch.ListenerCache,
			et.TypeURL():               et,
			ch.SecretCache.TypeURL():   &ch.SecretCache,
		}
		opts := ctx.grpcOptions()
		s := cgrpc.NewAPI(log, resources, opts...)
		select {
		case <-stop:
			// shut down
			return nil
		case <-leaderOK:
			// we've become leader, open the listening socket
			addr := net.JoinHostPort(ctx.xdsAddr, strconv.Itoa(ctx.xdsPort))
			l, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}

			log = log.WithField("address", addr)
			if ctx.PermitInsecureGRPC {
				log = log.WithField("insecure", true)
			}

			log.Info("started")
			defer log.Info("stopped")

			go func() {
				<-stop
				s.Stop()
			}()

			return s.Serve(l)
		}
	})

	// step 15. Setup SIGTERM handler
	g.Add(func(stop <-chan struct{}) error {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM)
		select {
		case <-c:
			log.WithField("context", "sigterm-handler").Info("received SIGTERM, shutting down")
		case <-stop:
			// Do nothing. The group is shutting down.
		}
		return nil
	})

	// step 16. GO!
	return g.Run()
}

type informer interface {
	WaitForCacheSync(stopCh <-chan struct{}) map[reflect.Type]bool
	Start(stopCh <-chan struct{})
}

func startInformer(inf informer, log logrus.FieldLogger) func(stop <-chan struct{}) error {
	return func(stop <-chan struct{}) error {
		log.Println("waiting for cache sync")
		inf.WaitForCacheSync(stop)

		log.Println("started")
		defer log.Println("stopped")
		inf.Start(stop)
		<-stop
		return nil
	}
}
