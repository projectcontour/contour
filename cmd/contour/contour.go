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

	"github.com/heptio/contour/internal/debug"
	clientset "github.com/heptio/contour/internal/generated/clientset/versioned"
	"github.com/heptio/workgroup"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/envoy"
	"github.com/heptio/contour/internal/grpc"
	"github.com/heptio/contour/internal/k8s"

	"github.com/sirupsen/logrus"
)

func main() {
	log := logrus.StandardLogger()
	app := kingpin.New("contour", "Heptio Contour Kubernetes ingress controller.")
	bootstrap := app.Command("bootstrap", "Generate bootstrap configuration.")

	var config envoy.ConfigWriter
	path := bootstrap.Arg("path", "Configuration file.").Required().String()
	bootstrap.Flag("admin-address", "Envoy admin interface address").StringVar(&config.AdminAddress)
	bootstrap.Flag("admin-port", "Envoy admin interface port").IntVar(&config.AdminPort)
	bootstrap.Flag("stats-address", "Envoy /stats interface address").IntVar(&config.StatsAddress)
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

	// configuration parameters for debug service
	debug := debug.Service{
		FieldLogger: log.WithField("context", "debugsvc"),
	}

	serve.Flag("debug address", "address the /debug/pprof endpoint will bind too").Default("127.0.0.1").StringVar(&debug.Addr)
	serve.Flag("debug port", "port the /debug/pprof endpoint will bind too").Default("8000").IntVar(&debug.Port)

	// translator and DAGAdapter configuration
	var da contour.DAGAdapter

	serve.Flag("envoy-http-access-log", "Envoy HTTP access log").Default(contour.DEFAULT_HTTP_ACCESS_LOG).StringVar(&da.HTTPAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log").Default(contour.DEFAULT_HTTPS_ACCESS_LOG).StringVar(&da.HTTPSAccessLog)
	serve.Flag("envoy-http-address", "Envoy HTTP listener address").StringVar(&da.HTTPAddress)
	serve.Flag("envoy-https-address", "Envoy HTTPS listener address").StringVar(&da.HTTPSAddress)
	serve.Flag("envoy-http-port", "Envoy HTTP listener port").IntVar(&da.HTTPPort)
	serve.Flag("envoy-https-port", "Envoy HTTPS listener port").IntVar(&da.HTTPSPort)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners").BoolVar(&da.UseProxyProto)
	serve.Flag("ingress-class-name", "Contour IngressClass name").StringVar(&da.IngressClass)
	serve.Flag("ingressroute-root-namespaces", "Restrict contour to searching these namespaces for root ingress routes").StringsVar(&da.IngressRouteRootNamespaces)

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

		// setup DAG Adapter and debug handler
		debug.DAG = &da.ResourceEventHandler.DAG

		// client-go uses glog which requires initialisation as a side effect of calling
		// flag.Parse (see #118 and https://github.com/golang/glog/blob/master/glog.go#L679)
		// However kingpin owns our flag parsing, so we defer calling flag.Parse until
		// this point to avoid the Go flag package from rejecting flags which are defined
		// in kingpin. See #371
		flag.Parse()
		client, contourClient := newClient(*kubeconfig, *inCluster)

		wl := log.WithField("context", "watch")
		k8s.WatchServices(&g, client, wl, &da)
		k8s.WatchIngress(&g, client, wl, &da)
		k8s.WatchSecrets(&g, client, wl, &da)
		k8s.WatchIngressRoutes(&g, contourClient, wl, &da)

		// Endpoints updates are handled directly by the EndpointsTranslator
		// due to their high update rate and their orthogonal nature.
		et := &contour.EndpointsTranslator{
			FieldLogger: log.WithField("context", "endpointstranslator"),
		}
		k8s.WatchEndpoints(&g, client, wl, et)

		g.Add(debug.Start)

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
				clusterType:  &da.ClusterCache,
				routeType:    &da.RouteCache,
				listenerType: &da.ListenerCache,
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
