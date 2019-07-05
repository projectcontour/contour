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
	"os"
	"path/filepath"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

// registerServe registers the serve subcommand and flags
// with the Application provided.
func registerServe(app *kingpin.Application) (*kingpin.CmdClause, *serveContext) {
	var ctx serveContext
	serve := app.Command("serve", "Serve xDS API traffic")
	serve.Flag("incluster", "use in cluster configuration.").BoolVar(&ctx.inCluster)
	serve.Flag("kubeconfig", "path to kubeconfig (if not in running inside a cluster)").Default(filepath.Join(os.Getenv("HOME"), ".kube", "config")).StringVar(&ctx.kubeconfig)
	serve.Flag("xds-address", "xDS gRPC API address").Default("127.0.0.1").StringVar(&ctx.xdsAddr)
	serve.Flag("xds-port", "xDS gRPC API port").Default("8001").IntVar(&ctx.xdsPort)
	serve.Flag("stats-address", "Envoy /stats interface address").Default("0.0.0.0").StringVar(&ctx.statsAddr)
	serve.Flag("stats-port", "Envoy /stats interface port").Default("8002").IntVar(&ctx.statsPort)
	serve.Flag("contour-cafile", "CA bundle file name for serving gRPC with TLS").Envar("CONTOUR_CAFILE").StringVar(&ctx.caFile)
	serve.Flag("contour-cert-file", "Contour certificate file name for serving gRPC over TLS").Envar("CONTOUR_CERT_FILE").StringVar(&ctx.contourCert)
	serve.Flag("contour-key-file", "Contour key file name for serving gRPC over TLS").Envar("CONTOUR_KEY_FILE").StringVar(&ctx.contourKey)

	return serve, &ctx
}

type serveContext struct {
	inCluster                       bool
	kubeconfig                      string
	xdsAddr                         string
	xdsPort                         int
	statsAddr                       string
	statsPort                       int
	caFile, contourCert, contourKey string
}
