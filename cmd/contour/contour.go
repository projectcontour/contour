// Copyright © 2017 Heptio
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
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/envoyproxy/go-control-plane/pkg/cache"
	clientset "github.com/heptio/contour/apis/generated/clientset/versioned"
	contourinformers "github.com/heptio/contour/apis/generated/informers/externalversions"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/debug"
	"github.com/heptio/contour/internal/envoy"
	"github.com/heptio/contour/internal/grpc"
	"github.com/heptio/contour/internal/httpsvc"
	"github.com/heptio/contour/internal/k8s"
	"github.com/heptio/contour/internal/metrics"
	"github.com/heptio/contour/internal/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var ingressrouteRootNamespaceFlag string

func main() {
	log := logrus.StandardLogger()
	app := kingpin.New("contour", "Heptio Contour Kubernetes ingress controller.")
	var config envoy.BootstrapConfig
	bootstrap := app.Command("bootstrap", "Generate bootstrap configuration.")
	path := bootstrap.Arg("path", "Configuration file.").Required().String()
	bootstrap.Flag("admin-address", "Envoy admin interface address").StringVar(&config.AdminAddress)
	bootstrap.Flag("admin-port", "Envoy admin interface port").IntVar(&config.AdminPort)
	bootstrap.Flag("xds-address", "xDS gRPC API address").StringVar(&config.XDSAddress)
	bootstrap.Flag("xds-port", "xDS gRPC API port").IntVar(&config.XDSGRPCPort)

	// Get the running namespace passed via ENV var from the Kubernetes Downward API
	config.Namespace = getEnv("CONTOUR_NAMESPACE", "heptio-contour")

	cli := app.Command("cli", "A CLI client for the Heptio Contour Kubernetes ingress controller.")
	var client Client
	cli.Flag("contour", "contour host:port.").Default("127.0.0.1:8001").StringVar(&client.ContourAddr)

	var resources []string
	cds := cli.Command("cds", "watch services.")
	cds.Arg("resources", "CDS resource filter").StringsVar(&resources)
	eds := cli.Command("eds", "watch endpoints.")
	eds.Arg("resources", "EDS resource filter").StringsVar(&resources)
	lds := cli.Command("lds", "watch listerners.")
	lds.Arg("resources", "LDS resource filter").StringsVar(&resources)
	rds := cli.Command("rds", "watch routes.")
	rds.Arg("resources", "RDS resource filter").StringsVar(&resources)
	sds := cli.Command("sds", "watch secrets.")
	sds.Arg("resources", "SDS resource filter").StringsVar(&resources)

	serve := app.Command("serve", "Serve xDS API traffic")
	inCluster := serve.Flag("incluster", "use in cluster configuration.").Bool()
	kubeconfig := serve.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).String()
	xdsAddr := serve.Flag("xds-address", "xDS gRPC API address").Default("127.0.0.1").String()
	xdsPort := serve.Flag("xds-port", "xDS gRPC API port").Default("8001").Int()
	statsAddress := serve.Flag("stats-address", "Envoy /stats interface address").Default("0.0.0.0").String()
	statsPort := serve.Flag("stats-port", "Envoy /stats interface port").Default("8002").Int()

	ch := contour.CacheHandler{
		FieldLogger: log.WithField("context", "CacheHandler"),
	}

	metricsvc := metrics.Service{
		Service: httpsvc.Service{
			FieldLogger: log.WithField("context", "metricsvc"),
		},
	}

	registry := prometheus.NewRegistry()
	metricsvc.Registry = registry

	// register detault process / go collectors
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	// register our custom metrics
	metrics := metrics.NewMetrics(registry)

	reh := contour.ResourceEventHandler{
		FieldLogger: log.WithField("context", "resourceEventHandler"),
		Notifier: &contour.HoldoffNotifier{
			Notifier:    &ch,
			FieldLogger: log.WithField("context", "HoldoffNotifier"),
			Metrics:     metrics,
		},
	}

	// configuration parameters for debug service
	debugsvc := debug.Service{
		Service: httpsvc.Service{
			FieldLogger: log.WithField("context", "debugsvc"),
		},
		// plumb the DAGAdapter's Builder through
		// to the debug handler
		Builder: &reh.Builder,
	}

	serve.Flag("debug-http-address", "address the debug http endpoint will bind to").Default("127.0.0.1").StringVar(&debugsvc.Addr)
	serve.Flag("debug-http-port", "port the debug http endpoint will bind to").Default("6060").IntVar(&debugsvc.Port)

	serve.Flag("http-address", "address the metrics http endpoint will bind to").Default("0.0.0.0").StringVar(&metricsvc.Addr)
	serve.Flag("http-port", "port the metrics http endpoint will bind to").Default("8000").IntVar(&metricsvc.Port)

	serve.Flag("envoy-http-access-log", "Envoy HTTP access log").Default(contour.DEFAULT_HTTP_ACCESS_LOG).StringVar(&ch.HTTPAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log").Default(contour.DEFAULT_HTTPS_ACCESS_LOG).StringVar(&ch.HTTPSAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests").Default("0.0.0.0").StringVar(&ch.HTTPAddress)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests").Default("0.0.0.0").StringVar(&ch.HTTPSAddress)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests").Default("8080").IntVar(&ch.HTTPPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests").Default("8443").IntVar(&ch.HTTPSPort)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners").BoolVar(&ch.UseProxyProto)
	serve.Flag("ingress-class-name", "Contour IngressClass name").StringVar(&reh.IngressClass)
	serve.Flag("ingressroute-root-namespaces", "Restrict contour to searching these namespaces for root ingress routes").StringVar(&ingressrouteRootNamespaceFlag)

	// TODO(youngnick) remove these for 0.14, see #1141
	// The following flags are no-ops, and the variables are used to print a message that they don't do anything
	// any more.
	var externalHTTPPort int
	var externalHTTPSPort int
	serve.Flag("envoy-external-http-port", "External port for HTTP requests").IntVar(&externalHTTPPort)
	serve.Flag("envoy-external-https-port", "External port for HTTPS requests").IntVar(&externalHTTPSPort)

	args := os.Args[1:]
	switch kingpin.MustParse(app.Parse(args)) {
	case bootstrap.FullCommand():
		writeBootstrapConfig(&config, *path)
	case cds.FullCommand():
		stream := client.ClusterStream()
		watchstream(stream, cache.ClusterType, resources)
	case eds.FullCommand():
		stream := client.EndpointStream()
		watchstream(stream, cache.EndpointType, resources)
	case lds.FullCommand():
		stream := client.ListenerStream()
		watchstream(stream, cache.ListenerType, resources)
	case rds.FullCommand():
		stream := client.RouteStream()
		watchstream(stream, cache.RouteType, resources)
	case sds.FullCommand():
		stream := client.RouteStream()
		watchstream(stream, cache.SecretType, resources)
	case serve.FullCommand():
		log.Infof("args: %v", args)
		var g workgroup.Group

		// Deprecation warnings for deprecated flags.
		if externalHTTPPort != 0 {
			log.Warn("--envoy-external-http-port will be removed in 0.14")
		}
		if externalHTTPSPort != 0 {
			log.Warn("--envoy-external-https-port will be removed in 0.14")
		}

		// client-go uses glog which requires initialisation as a side effect of calling
		// flag.Parse (see #118 and https://github.com/golang/glog/blob/master/glog.go#L679)
		// However kingpin owns our flag parsing, so we defer calling flag.Parse until
		// this point to avoid the Go flag package from rejecting flags which are defined
		// in kingpin. See #371
		flag.Parse()
		// Always let glog logs to stderr rather than files. Otherwise,
		// once it attempts to log to files under /tmp, will crash the
		// whole process since the path may not be accessible in
		// container environment. See #959
		_ = flag.Lookup("logtostderr").Value.Set("true")

		ch.ListenerCache = contour.NewListenerCache(*statsAddress, *statsPort)
		reh.IngressRouteRootNamespaces = parseRootNamespaces(ingressrouteRootNamespaceFlag)

		client, contourClient := newClient(*kubeconfig, *inCluster)
		metricsvc.Client = client

		// resync timer disabled for Contour
		coreInformers := coreinformers.NewSharedInformerFactory(client, 0)
		contourInformers := contourinformers.NewSharedInformerFactory(contourClient, 0)

		coreInformers.Core().V1().Services().Informer().AddEventHandler(&reh)
		coreInformers.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(&reh)
		coreInformers.Core().V1().Secrets().Informer().AddEventHandler(&reh)
		contourInformers.Contour().V1beta1().IngressRoutes().Informer().AddEventHandler(&reh)
		contourInformers.Contour().V1beta1().TLSCertificateDelegations().Informer().AddEventHandler(&reh)

		ch.IngressRouteStatus = &k8s.IngressRouteStatus{
			Client: contourClient,
		}

		// Endpoints updates are handled directly by the EndpointsTranslator
		// due to their high update rate and their orthogonal nature.
		et := &contour.EndpointsTranslator{
			FieldLogger: log.WithField("context", "endpointstranslator"),
		}
		coreInformers.Core().V1().Endpoints().Informer().AddEventHandler(et)

		g.Add(startInformer(coreInformers, log.WithField("context", "coreinformers")))
		g.Add(startInformer(contourInformers, log.WithField("context", "contourinformers")))

		ch.Metrics = metrics
		reh.Metrics = metrics

		g.Add(debugsvc.Start)
		g.Add(metricsvc.Start)

		g.Add(func(stop <-chan struct{}) error {
			log := log.WithField("context", "grpc")
			addr := net.JoinHostPort(*xdsAddr, strconv.Itoa(*xdsPort))
			l, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}

			s := grpc.NewAPI(log, map[string]grpc.Resource{
				ch.ClusterCache.TypeURL():  &ch.ClusterCache,
				ch.RouteCache.TypeURL():    &ch.RouteCache,
				ch.ListenerCache.TypeURL(): &ch.ListenerCache,
				et.TypeURL():               et,
				ch.SecretCache.TypeURL():   &ch.SecretCache,
			})
			log.Println("started")
			defer log.Println("stopped")
			return s.Serve(l)
		})
		_ = g.Run()
	default:
		app.Usage(args)
		os.Exit(2)
	}
}

func newClient(kubeconfig string, inCluster bool) (*kubernetes.Clientset, *clientset.Clientset) {
	var err error
	var config *rest.Config
	if kubeconfig != "" && !inCluster {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		check(err)
	} else {
		config, err = rest.InClusterConfig()
		check(err)
	}

	client, err := kubernetes.NewForConfig(config)
	check(err)
	contourClient, err := clientset.NewForConfig(config)
	check(err)
	return client, contourClient
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
		defer log.Println("stopping")
		inf.Start(stop)
		<-stop
		return nil
	}
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseRootNamespaces(rn string) []string {
	if rn == "" {
		return nil
	}
	var ns []string
	for _, s := range strings.Split(rn, ",") {
		ns = append(ns, strings.TrimSpace(s))
	}
	return ns
}

func getEnv(key, fallback string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		value = fallback
	}
	return value
}
