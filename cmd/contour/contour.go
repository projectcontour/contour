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
	"os"

	resource_v3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/projectcontour/contour/internal/build"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	log := logrus.StandardLogger()
	k8s.InitLogging(k8s.LogWriterOption(log.WithField("context", "kubernetes")))

	app := kingpin.New("contour", "Contour Kubernetes ingress controller.")
	app.HelpFlag.Short('h')

	envoyCmd := app.Command("envoy", "Sub-command for envoy actions.")
	sdm, shutdownManagerCtx := registerShutdownManager(envoyCmd, log)

	bootstrap, bootstrapCtx := registerBootstrap(app)

	// Add a "shutdown" command which initiates an Envoy shutdown sequence.
	sdmShutdown, sdmShutdownCtx := registerShutdown(envoyCmd, log)

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
	case sdm.FullCommand():
		doShutdownManager(shutdownManagerCtx)
	case sdmShutdown.FullCommand():
		sdmShutdownCtx.shutdownHandler()
	case bootstrap.FullCommand():
		err := bootstrapCtx.XDSResourceVersion.Validate()
		if err != nil {
			log.WithError(err).Fatal("failed to parse bootstrap args")
		}
		if err := envoy_v3.WriteBootstrap(bootstrapCtx); err != nil {
			log.WithError(err).Fatal("failed to write bootstrap configuration")
		}
	case certgenApp.FullCommand():
		doCertgen(certgenConfig, log)
	case cds.FullCommand():
		stream := client.ClusterStream()
		watchstream(stream, resource_v3.ClusterType, resources)
	case eds.FullCommand():
		stream := client.EndpointStream()
		watchstream(stream, resource_v3.EndpointType, resources)
	case lds.FullCommand():
		stream := client.ListenerStream()
		watchstream(stream, resource_v3.ListenerType, resources)
	case rds.FullCommand():
		stream := client.RouteStream()
		watchstream(stream, resource_v3.RouteType, resources)
	case sds.FullCommand():
		stream := client.RouteStream()
		watchstream(stream, resource_v3.SecretType, resources)
	case serve.FullCommand():
		// Parse args a second time so cli flags are applied
		// on top of any values sourced from -c's config file.
		kingpin.MustParse(app.Parse(args))

		// Reinitialize with the target debug level.
		k8s.InitLogging(
			k8s.LogWriterOption(log.WithField("context", "kubernetes")),
			k8s.LogLevelOption(int(serveCtx.KubernetesDebug)),
		)

		if serveCtx.Config.Debug {
			log.SetLevel(logrus.DebugLevel)
		}

		log.Infof("args: %v", args)

		// Validate the result of applying the command-line
		// flags on top of the config file.
		if err := serveCtx.Config.Validate(); err != nil {
			log.WithError(err).Fatal("invalid configuration")
		}

		if err := doServe(log, serveCtx); err != nil {
			log.WithError(err).Fatal("Contour server failed")
		}
	case version.FullCommand():
		println(build.PrintBuildInfo())
	default:
		app.Usage(args)
		os.Exit(2)
	}

}
