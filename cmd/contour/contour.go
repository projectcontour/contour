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
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gorilla/handlers"
	"github.com/heptio/contour/internal/contour"
	"github.com/heptio/contour/internal/envoy"
	"github.com/heptio/contour/internal/grpc"
	"github.com/heptio/contour/internal/json"
	"github.com/heptio/contour/internal/k8s"
	"github.com/heptio/contour/internal/log/stdlog"
	"github.com/heptio/contour/internal/workgroup"

	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

const (
	V1_API_ADDRESS = "127.0.0.1:8000" // v1 JSON RPC
	V2_API_ADDRESS = "127.0.0.1:8001" // v2 gRPC
)

func main() {
	app := kingpin.New("contour", "Heptio Contour Kubernetes ingress controller.")
	bootstrap := app.Command("bootstrap", "Generate bootstrap configuration.")

	path := bootstrap.Arg("path", "Configuration file.").Required().String()

	serve := app.Command("serve", "Serve xDS API traffic")
	var config contour.Config
	inCluster := serve.Flag("incluster", "use in cluster configuration.").Bool()
	kubeconfig := serve.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).String()
	debug := serve.Flag("debug", "enable v1 REST API request logging.").Bool()

	args := os.Args[1:]
	switch kingpin.MustParse(app.Parse(args)) {
	default:
		app.Usage(args)
		os.Exit(2)
	case bootstrap.FullCommand():
		writeBootstrapConfig(*path)
	case serve.FullCommand():
		logger := stdlog.New(os.Stdout, os.Stderr, 0)
		config.Logger = logger.WithPrefix("translator")
		client := newClient(*kubeconfig, *inCluster)

		// REST v1 support
		ds := json.DataSource{
			Logger: logger.WithPrefix("DataSource"),
		}

		// gRPC v2 support
		t := contour.NewTranslator(&config)

		var g workgroup.Group

		// buffer notifications to t to ensure they are handled sequentially.
		buf := k8s.NewBuffer(&g, t, logger, 128)
		k8s.WatchServices(&g, client, logger, &ds, buf)
		k8s.WatchEndpoints(&g, client, logger, &ds, buf)
		k8s.WatchIngress(&g, client, logger, &ds, buf)
		k8s.WatchSecrets(&g, client, logger, buf) // don't deliver to &ds, the rest api doesn't know how to process secrets

		g.Add(func(stop <-chan struct{}) {
			logger := logger.WithPrefix("JSONAPI")
			api := json.NewAPI(logger, &ds)
			if *debug {
				// enable request logging if --debug enabled
				api = handlers.LoggingHandler(os.Stdout, api)
			}
			srv := &http.Server{
				Handler:      api,
				Addr:         V1_API_ADDRESS,
				WriteTimeout: 15 * time.Second,
				ReadTimeout:  15 * time.Second,
			}
			go srv.ListenAndServe() // run server in another goroutine
			logger.Infof("started, listening on %v", srv.Addr)
			defer logger.Infof("stopped")
			<-stop                             // wait for stop signal
			srv.Shutdown(context.Background()) // shutdown and wait for server to exit
		})

		g.Add(func(stop <-chan struct{}) {
			logger := logger.WithPrefix("gRPCAPI")
			l, err := net.Listen("tcp", V2_API_ADDRESS)
			if err != nil {
				logger.Errorf("could not listen on %s: %v", V2_API_ADDRESS, err)
				return // TODO(dfc) should return the error not log it
			}
			s := grpc.NewAPI(logger, t)
			logger.Infof("started")
			defer logger.Infof("stopped")
			s.Serve(l)
		})

		g.Run()
	}
}

// writeBootstrapConfig writes a bootstrap configuration to the supplied path.
// If the path ends in .json, the configuration file will be in v1 JSON format.
// If the path ends in .yaml, the configuration file will be in v2 YAML format.
func writeBootstrapConfig(path string) {
	config := envoy.ConfigWriter{}
	f, err := os.Create(path)
	check(err)
	switch filepath.Ext(path) {
	case ".json":
		err = config.WriteJSON(f)
		check(err)
	case ".yaml":
		err = config.WriteYAML(f)
		check(err)
	default:
		f.Close()
		check(fmt.Errorf("path %s must end in one of .json or .yaml", path))
	}
	check(f.Close())
}

func newClient(kubeconfig string, inCluster bool) *kubernetes.Clientset {
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
	return client
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
