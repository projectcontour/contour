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
	"strconv"
	"strings"

	clientset "github.com/heptio/contour/apis/generated/clientset/versioned"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/debug"
	"github.com/heptio/contour/internal/envoy"
	"github.com/heptio/contour/internal/grpc"
	"github.com/heptio/contour/internal/httpsvc"
	"github.com/heptio/contour/internal/k8s"
	"github.com/heptio/contour/internal/metrics"
	"github.com/heptio/workgroup"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var ingressrouteRootNamespaceFlag string

func main() {
	log := logrus.StandardLogger()
	app := kingpin.New("contour", "Heptio Contour Kubernetes ingress controller.")
	bootstrap := app.Command("bootstrap", "Generate bootstrap configuration.")

	var config envoy.ConfigWriter
	path := bootstrap.Arg("path", "Configuration file.").Required().String()
	bootstrap.Flag("admin-address", "Envoy admin interface address").StringVar(&config.AdminAddress)
	bootstrap.Flag("admin-port", "Envoy admin interface port").IntVar(&config.AdminPort)
	bootstrap.Flag("stats-address", "Envoy /stats interface address").StringVar(&config.StatsAddress)
	bootstrap.Flag("stats-port", "Envoy /stats interface port").IntVar(&config.StatsPort)
	bootstrap.Flag("xds-address", "xDS gRPC API address").StringVar(&config.XDSAddress)
	bootstrap.Flag("xds-port", "xDS gRPC API port").IntVar(&config.XDSGRPCPort)
	bootstrap.Flag("statsd-enabled", "enable statsd output").BoolVar(&config.StatsdEnabled)
	bootstrap.Flag("statsd-address", "statsd address").StringVar(&config.StatsdAddress)
	bootstrap.Flag("statsd-port", "statsd port").IntVar(&config.StatsdPort)

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

	serve := app.Command("serve", "Serve xDS API traffic")
	inCluster := serve.Flag("incluster", "use in cluster configuration.").Bool()
	kubeconfig := serve.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).String()
	xdsAddr := serve.Flag("xds-address", "xDS gRPC API address").Default("127.0.0.1").String()
	xdsPort := serve.Flag("xds-port", "xDS gRPC API port").Default("8001").Int()

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
	registry.MustRegister(prometheus.NewProcessCollector(os.Getpid(), ""))
	registry.MustRegister(prometheus.NewGoCollector())

	// register our custom metrics
	metrics := metrics.NewMetrics(registry)

	reh := contour.ResourceEventHandler{
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

	serve.Flag("debug-http-address", "address the debug http endpoint will bind too").Default("127.0.0.1").StringVar(&debugsvc.Addr)
	serve.Flag("debug-http-port", "port the debug http endpoint will bind too").Default("6060").IntVar(&debugsvc.Port)

	serve.Flag("http-address", "address the metrics http endpoint will bind too").Default("0.0.0.0").StringVar(&metricsvc.Addr)
	serve.Flag("http-port", "port the metrics http endpoint will bind too").Default("8000").IntVar(&metricsvc.Port)

	serve.Flag("envoy-http-access-log", "Envoy HTTP access log").Default(contour.DEFAULT_HTTP_ACCESS_LOG).StringVar(&ch.HTTPAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log").Default(contour.DEFAULT_HTTPS_ACCESS_LOG).StringVar(&ch.HTTPSAccessLog)
	serve.Flag("envoy-http-address", "Envoy HTTP listener address").StringVar(&ch.HTTPAddress)
	serve.Flag("envoy-https-address", "Envoy HTTPS listener address").StringVar(&ch.HTTPSAddress)
	serve.Flag("envoy-http-port", "Envoy HTTP listener port").IntVar(&ch.HTTPPort)
	serve.Flag("envoy-https-port", "Envoy HTTPS listener port").IntVar(&ch.HTTPSPort)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners").BoolVar(&ch.UseProxyProto)
	serve.Flag("ingress-class-name", "Contour IngressClass name").StringVar(&reh.IngressClass)
	serve.Flag("ingressroute-root-namespaces", "Restrict contour to searching these namespaces for root ingress routes").StringVar(&ingressrouteRootNamespaceFlag)

	args := os.Args[1:]
	switch kingpin.MustParse(app.Parse(args)) {
	case bootstrap.FullCommand():
		writeBootstrapConfig(&config, *path)
	case cds.FullCommand():
		stream := client.ClusterStream()
		watchstream(stream, clusterType, resources)
	case eds.FullCommand():
		stream := client.EndpointStream()
		watchstream(stream, endpointType, resources)
	case lds.FullCommand():
		stream := client.ListenerStream()
		watchstream(stream, listenerType, resources)
	case rds.FullCommand():
		stream := client.RouteStream()
		watchstream(stream, routeType, resources)
	case serve.FullCommand():
		log.Infof("args: %v", args)
		var g workgroup.Group

		// client-go uses glog which requires initialisation as a side effect of calling
		// flag.Parse (see #118 and https://github.com/golang/glog/blob/master/glog.go#L679)
		// However kingpin owns our flag parsing, so we defer calling flag.Parse until
		// this point to avoid the Go flag package from rejecting flags which are defined
		// in kingpin. See #371
		flag.Parse()

		reh.IngressRouteRootNamespaces = parseRootNamespaces(ingressrouteRootNamespaceFlag)

		client, contourClient := newClient(*kubeconfig, *inCluster)

		wl := log.WithField("context", "watch")
		k8s.WatchServices(&g, client, wl, &reh)
		k8s.WatchIngress(&g, client, wl, &reh)
		k8s.WatchSecrets(&g, client, wl, &reh)
		k8s.WatchIngressRoutes(&g, contourClient, wl, &reh)

		ch.IngressRouteStatus = &k8s.IngressRouteStatus{
			Client: contourClient,
		}

		// Endpoints updates are handled directly by the EndpointsTranslator
		// due to their high update rate and their orthogonal nature.
		et := &contour.EndpointsTranslator{
			FieldLogger: log.WithField("context", "endpointstranslator"),
		}
		k8s.WatchEndpoints(&g, client, wl, et)

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

			// Resource types in xDS v2.
			const (
				googleApis   = "type.googleapis.com/"
				typePrefix   = googleApis + "envoy.api.v2."
				endpointType = typePrefix + "ClusterLoadAssignment"
				clusterType  = typePrefix + "Cluster"
				routeType    = typePrefix + "RouteConfiguration"
				listenerType = typePrefix + "Listener"
			)
			s := grpc.NewAPI(log, map[string]grpc.Cache{
				clusterType:  &ch.ClusterCache,
				routeType:    &ch.RouteCache,
				listenerType: &ch.ListenerCache,
				endpointType: et,
			})
			log.Println("started")
			defer log.Println("stopped")
			return s.Serve(l)
		})
		g.Run()
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
