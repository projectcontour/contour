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
	"fmt"
	"os"

	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v2"
	"github.com/projectcontour/contour/internal/build"
	"github.com/projectcontour/contour/internal/envoy"
	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/klog"
)

func init() {
	// even though we don't use it directly, some of our dependencies use klog
	// so we must initialize it here to ensure that klog is set to log to stderr
	// and not to a file.
	// yes, this is gross, the klog authors are monsters.
	klog.InitFlags(nil)
}

func main() {
	log := logrus.StandardLogger()
	app := kingpin.New("contour", "Contour Kubernetes ingress controller.")
	app.HelpFlag.Short('h')

	envoyCmd := app.Command("envoy", "Sub-command for envoy actions.")
	shutdownManager, shutdownManagerCtx := registerShutdownManager(envoyCmd, log)

	bootstrap, bootstrapCtx := registerBootstrap(app)
	certgenApp, certgenConfig := registerCertGen(app)

	cli := app.Command("cli", "A CLI client for the Contour Kubernetes ingress controller.")
	var client Client
	cli.Flag("contour", "Contour host:port.").Default("127.0.0.1:8001").StringVar(&client.ContourAddr)
	cli.Flag("cafile", "CA bundle file for connecting to a TLS-secured Contour.").Envar("CLI_CAFILE").StringVar(&client.CAFile)
	cli.Flag("cert-file", "Client certificate file for connecting to a TLS-secured Contour.").Envar("CLI_CERT_FILE").StringVar(&client.ClientCert)
	cli.Flag("key-file", "Client key file for connecting to a TLS-secured Contour.").Envar("CLI_KEY_FILE").StringVar(&client.ClientKey)

	var resources []string
	cds := cli.Command("cds", "Watch services.")
	cds.Arg("resources", "CDS resource filter").StringsVar(&resources)
	eds := cli.Command("eds", "Watch endpoints.")
	eds.Arg("resources", "EDS resource filter").StringsVar(&resources)
	lds := cli.Command("lds", "Watch listeners.")
	lds.Arg("resources", "LDS resource filter").StringsVar(&resources)
	rds := cli.Command("rds", "Watch routes.")
	rds.Arg("resources", "RDS resource filter").StringsVar(&resources)
	sds := cli.Command("sds", "Watch secrets.")
	sds.Arg("resources", "SDS resource filter").StringsVar(&resources)

	serve, serveCtx := registerServe(app)
	version := app.Command("version", "Build information for Contour.")

	args := os.Args[1:]
	switch kingpin.MustParse(app.Parse(args)) {
	case shutdownManager.FullCommand():
		doShutdownManager(shutdownManagerCtx)
	case bootstrap.FullCommand():
		check(envoy.WriteBootstrap(bootstrapCtx))
	case certgenApp.FullCommand():
		doCertgen(certgenConfig)
	case cds.FullCommand():
		stream := client.ClusterStream()
		watchstream(stream, resource.ClusterType, resources)
	case eds.FullCommand():
		stream := client.EndpointStream()
		watchstream(stream, resource.EndpointType, resources)
	case lds.FullCommand():
		stream := client.ListenerStream()
		watchstream(stream, resource.ListenerType, resources)
	case rds.FullCommand():
		stream := client.RouteStream()
		watchstream(stream, resource.RouteType, resources)
	case sds.FullCommand():
		stream := client.RouteStream()
		watchstream(stream, resource.SecretType, resources)
	case serve.FullCommand():
		// parse args a second time so cli flags are applied
		// on top of any values sourced from -c's config file.
		_, err := app.Parse(args)
		check(err)
		if serveCtx.Debug {
			log.SetLevel(logrus.DebugLevel)
		}
		log.Infof("args: %v", args)
		check(doServe(log, serveCtx))
	case version.FullCommand():
		println(build.PrintBuildInfo())
	default:
		app.Usage(args)
		os.Exit(2)
	}

}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
