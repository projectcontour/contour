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
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"

	clientset "github.com/heptio/contour/pkg/generated/clientset/versioned"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/envoy"
	"github.com/heptio/contour/internal/grpc"
	"github.com/heptio/contour/internal/k8s"
	"github.com/heptio/contour/internal/workgroup"

	"github.com/sirupsen/logrus"
)

// this is necessary due to #113 wherein glog neccessitates a call to flag.Parse
// before any logging statements can be invoked. (See also https://github.com/golang/glog/blob/master/glog.go#L679)
// unsure why this seemingly unnecessary prerequisite is in place but there must be some sane reason.
func init() {
	flag.Parse()
}

func main() {
	log := logrus.StandardLogger()
	t := &contour.Translator{
		FieldLogger: log.WithField("context", "translator"),
	}

	app := kingpin.New("contour", "Heptio Contour Kubernetes ingress controller.")
	bootstrap := app.Command("bootstrap", "Generate bootstrap configuration.")

	var config envoy.ConfigWriter
	path := bootstrap.Arg("path", "Configuration file.").Required().String()
	bootstrap.Flag("admin-address", "Envoy admin interface address").StringVar(&config.AdminAddress)
	bootstrap.Flag("admin-port", "Envoy admin interface port").IntVar(&config.AdminPort)
	bootstrap.Flag("xds-address", "xDS gRPC API address").StringVar(&config.XDSAddress)
	bootstrap.Flag("xds-port", "xDS gRPC API port").IntVar(&config.XDSGRPCPort)

	serve := app.Command("serve", "Serve xDS API traffic")
	inCluster := serve.Flag("incluster", "use in cluster configuration.").Bool()
	kubeconfig := serve.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).String()
	xdsAddr := serve.Flag("xds-address", "xDS gRPC API address").Default("127.0.0.1").String()
	xdsPort := serve.Flag("xds-port", "xDS gRPC API port").Default("8001").Int()

	// translator configuration
	serve.Flag("envoy-http-address", "Envoy HTTP listener address").StringVar(&t.HTTPAddress)
	serve.Flag("envoy-https-address", "Envoy HTTPS listener address").StringVar(&t.HTTPSAddress)
	serve.Flag("envoy-http-port", "Envoy HTTP listener port").IntVar(&t.HTTPPort)
	serve.Flag("envoy-https-port", "Envoy HTTPS listener port").IntVar(&t.HTTPSPort)
	serve.Flag("use-proxy-protocol", "Use PROXY protocol for all listeners").BoolVar(&t.UseProxyProto)

	args := os.Args[1:]
	switch kingpin.MustParse(app.Parse(args)) {
	default:
		app.Usage(args)
		os.Exit(2)
	case bootstrap.FullCommand():
		writeBootstrapConfig(&config, *path)
	case serve.FullCommand():
		log.Infof("args: %v", args)
		var g workgroup.Group

		// buffer notifications to t to ensure they are handled sequentially.
		buf := k8s.NewBuffer(&g, t, log, 128)

		client, contourClient := newClient(*kubeconfig, *inCluster)

		wl := log.WithField("context", "watch")
		k8s.WatchServices(&g, client, wl, buf)
		k8s.WatchEndpoints(&g, client, wl, buf)
		k8s.WatchIngress(&g, client, wl, buf)
		k8s.WatchSecrets(&g, client, wl, buf)
		k8s.WatchRoutes(&g, contourClient, wl, buf)

		g.Add(func(stop <-chan struct{}) {
			log := log.WithField("context", "grpc")
			addr := net.JoinHostPort(*xdsAddr, strconv.Itoa(*xdsPort))
			l, err := net.Listen("tcp", addr)
			if err != nil {
				log.Errorf("could not listen on %s: %v", addr, err)
				return // TODO(dfc) should return the error not log it
			}
			s := grpc.NewAPI(log, t)
			log.Println("started")
			defer log.Println("stopped")
			s.Serve(l)
		})

		g.Run()
	}
}

type configWriter interface {
	WriteYAML(io.Writer) error
}

// writeBootstrapConfig writes a bootstrap configuration to the supplied path.
// If the path ends in .yaml, the configuration file will be in v2 YAML format.
func writeBootstrapConfig(config configWriter, path string) {
	f, err := os.Create(path)
	check(err)
	switch filepath.Ext(path) {
	case ".json":
		check(fmt.Errorf("JSON bootstrap configuration has been removed.\nPlease see https://github.com/heptio/contour/blob/master/docs/upgrade.md"))
	case ".yaml":
		err = config.WriteYAML(f)
		check(err)
	default:
		f.Close()
		check(fmt.Errorf("path %s must end in .yaml", path))
	}
	check(f.Close())
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
