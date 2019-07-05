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
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"

	"github.com/envoyproxy/go-control-plane/pkg/cache"
	clientset "github.com/heptio/contour/apis/generated/clientset/versioned"
	contourinformers "github.com/heptio/contour/apis/generated/informers/externalversions"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/debug"
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

func main() {
	log := logrus.StandardLogger()
	app := kingpin.New("contour", "Heptio Contour Kubernetes ingress controller.")

	bootstrap, bootstrapCtx := registerBootstrap(app)

	cli := app.Command("cli", "A CLI client for the Heptio Contour Kubernetes ingress controller.")
	var client Client
	cli.Flag("contour", "contour host:port.").Default("127.0.0.1:8001").StringVar(&client.ContourAddr)
	cli.Flag("cafile", "CA bundle file for connecting to a TLS-secured Contour").Envar("CLI_CAFILE").StringVar(&client.CAFile)
	cli.Flag("cert-file", "Client certificate file for connecting to a TLS-secured Contour").Envar("CLI_CERT_FILE").StringVar(&client.ClientCert)
	cli.Flag("key-file", "Client key file for connecting to a TLS-secured Contour").Envar("CLI_KEY_FILE").StringVar(&client.ClientKey)

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

	ch := contour.CacheHandler{
		FieldLogger: log.WithField("context", "CacheHandler"),
	}

	reh := contour.ResourceEventHandler{
		FieldLogger: log.WithField("context", "resourceEventHandler"),
		Notifier: &contour.HoldoffNotifier{
			Notifier:    &ch,
			FieldLogger: log.WithField("context", "HoldoffNotifier"),
		},
	}

	serve, serveCtx := registerServe(app)

	serve.Flag("envoy-http-access-log", "Envoy HTTP access log").Default(contour.DEFAULT_HTTP_ACCESS_LOG).StringVar(&ch.HTTPAccessLog)
	serve.Flag("envoy-https-access-log", "Envoy HTTPS access log").Default(contour.DEFAULT_HTTPS_ACCESS_LOG).StringVar(&ch.HTTPSAccessLog)
	serve.Flag("envoy-service-http-address", "Kubernetes Service address for HTTP requests").Default("0.0.0.0").StringVar(&ch.HTTPAddress)
	serve.Flag("envoy-service-https-address", "Kubernetes Service address for HTTPS requests").Default("0.0.0.0").StringVar(&ch.HTTPSAddress)
	serve.Flag("envoy-service-http-port", "Kubernetes Service port for HTTP requests").Default("8080").IntVar(&ch.HTTPPort)
	serve.Flag("envoy-service-https-port", "Kubernetes Service port for HTTPS requests").Default("8443").IntVar(&ch.HTTPSPort)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners").BoolVar(&ch.UseProxyProto)
	serve.Flag("ingress-class-name", "Contour IngressClass name").StringVar(&reh.IngressClass)

	args := os.Args[1:]
	switch kingpin.MustParse(app.Parse(args)) {
	case bootstrap.FullCommand():
		doBootstrap(bootstrapCtx)
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

		ch.ListenerCache = contour.NewListenerCache(serveCtx.statsAddr, serveCtx.statsPort)
		reh.IngressRouteRootNamespaces = serveCtx.ingressRouteRootNamespaces()

		client, contourClient := newClient(serveCtx.kubeconfig, serveCtx.inCluster)

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

		registry := prometheus.NewRegistry()
		// register detault process / go collectors
		registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
		registry.MustRegister(prometheus.NewGoCollector())

		metricsvc := metrics.Service{
			Service: httpsvc.Service{
				Addr:        serveCtx.metricsAddr,
				Port:        serveCtx.metricsPort,
				FieldLogger: log.WithField("context", "metricsvc"),
			},
			Client:   client,
			Registry: registry,
		}
		g.Add(metricsvc.Start)

		debugsvc := debug.Service{
			Service: httpsvc.Service{
				Addr:        serveCtx.debugAddr,
				Port:        serveCtx.debugPort,
				FieldLogger: log.WithField("context", "debugsvc"),
			},
			KubernetesCache: &reh.KubernetesCache,
		}
		g.Add(debugsvc.Start)

		// register our custom metrics
		metrics := metrics.NewMetrics(registry)
		ch.Metrics = metrics
		reh.Metrics = metrics

		g.Add(func(stop <-chan struct{}) error {
			log := log.WithField("context", "grpc")
			addr := net.JoinHostPort(serveCtx.xdsAddr, strconv.Itoa(serveCtx.xdsPort))

			var l net.Listener
			var err error
			tlsconfig := serveCtx.tlsconfig()
			if tlsconfig != nil {
				log.Info("Setting up TLS for gRPC")
				l, err = tls.Listen("tcp", addr, tlsconfig)
				if err != nil {
					return err
				}
			} else {
				l, err = net.Listen("tcp", addr)
				if err != nil {
					return err
				}
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
