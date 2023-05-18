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

	"go.uber.org/automaxprocs/maxprocs"

	"github.com/alecthomas/kingpin/v2"
	resource_v3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/projectcontour/contour/internal/build"
	"github.com/projectcontour/contour/internal/envoy"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/sirupsen/logrus"
)

func main() {
	log := logrus.StandardLogger()
	k8s.InitLogging(k8s.LogWriterOption(log.WithField("context", "kubernetes")))

	// set GOMAXPROCS
	_, err := maxprocs.Set(maxprocs.Logger(log.Printf))
	if err != nil {
		log.WithError(err).Fatal("failed to set GOMAXPROCS")
	}

	// NOTE: when add a new subcommand, we'll have to remember to add it to 'TestOptionFlagsAreSorted'
	// to ensure the option flags in lexicographic order.

	app := kingpin.New("contour", "Contour Kubernetes ingress controller.")
	app.HelpFlag.Short('h')

	// Log-format applies to log format of all sub-commands.
	logFormat := app.Flag("log-format", "Log output format for Contour. Either text or json.").Default("text").Enum("text", "json")

	bootstrap, bootstrapCtx := registerBootstrap(app)

	certgenApp, certgenConfig := registerCertGen(app)

	cli, client := registerCli(app, log)

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

	envoyCmd := app.Command("envoy", "Sub-command for envoy actions.")

	// Add a "shutdown" command which initiates an Envoy shutdown sequence.
	sdmShutdown, sdmShutdownCtx := registerShutdown(envoyCmd, log)

	sdm, shutdownManagerCtx := registerShutdownManager(envoyCmd, log)

	gatewayProvisioner, gatewayProvisionerConfig := registerGatewayProvisioner(app)

	serve, serveCtx := registerServe(app)
	version := app.Command("version", "Build information for Contour.")

	args := os.Args[1:]
	cmd := kingpin.MustParse(app.Parse(args))

	switch *logFormat {
	case "text":
		log.SetFormatter(&logrus.TextFormatter{})
	case "json":
		log.SetFormatter(&logrus.JSONFormatter{})
	}

	switch cmd {
	case gatewayProvisioner.FullCommand():
		runGatewayProvisioner(gatewayProvisionerConfig)
	case sdm.FullCommand():
		doShutdownManager(shutdownManagerCtx)
	case sdmShutdown.FullCommand():
		sdmShutdownCtx.shutdownHandler()
	case bootstrap.FullCommand():
		if err := bootstrapCtx.XDSResourceVersion.Validate(); err != nil {
			log.WithError(err).Fatal("failed to parse bootstrap args")
		}
		if err := envoy.ValidAdminAddress(bootstrapCtx.AdminAddress); err != nil {
			log.WithField("flag", "--admin-address").WithError(err).Fatal("failed to parse bootstrap args")
		}
		if err := envoy_v3.WriteBootstrap(bootstrapCtx); err != nil {
			log.WithError(err).Fatal("failed to write bootstrap configuration")
		}
	case certgenApp.FullCommand():
		doCertgen(certgenConfig, log)
	case cds.FullCommand():
		if client.Delta {
			stream := client.DeltaClusterStream()
			watchDeltaStream(log, stream, resource_v3.ClusterType, resources, client.Nack, client.NodeID)
		} else {
			stream := client.ClusterStream()
			watchstream(log, stream, resource_v3.ClusterType, resources, client.Nack, client.NodeID)
		}
	case eds.FullCommand():
		if client.Delta {
			stream := client.DeltaEndpointStream()
			watchDeltaStream(log, stream, resource_v3.EndpointType, resources, client.Nack, client.NodeID)
		} else {
			stream := client.EndpointStream()
			watchstream(log, stream, resource_v3.EndpointType, resources, client.Nack, client.NodeID)
		}
	case lds.FullCommand():
		if client.Delta {
			stream := client.DeltaListenerStream()
			watchDeltaStream(log, stream, resource_v3.ListenerType, resources, client.Nack, client.NodeID)
		} else {
			stream := client.ListenerStream()
			watchstream(log, stream, resource_v3.ListenerType, resources, client.Nack, client.NodeID)
		}
	case rds.FullCommand():
		if client.Delta {
			stream := client.DeltaRouteStream()
			watchDeltaStream(log, stream, resource_v3.RouteType, resources, client.Nack, client.NodeID)
		} else {
			stream := client.RouteStream()
			watchstream(log, stream, resource_v3.RouteType, resources, client.Nack, client.NodeID)
		}
	case sds.FullCommand():
		if client.Delta {
			stream := client.DeltaRouteStream()
			watchDeltaStream(log, stream, resource_v3.SecretType, resources, client.Nack, client.NodeID)
		} else {
			stream := client.RouteStream()
			watchstream(log, stream, resource_v3.SecretType, resources, client.Nack, client.NodeID)
		}
	case serve.FullCommand():
		// Parse args a second time so cli flags are applied
		// on top of any values sourced from -c's config file.
		kingpin.MustParse(app.Parse(args))

		if serveCtx.Config.Debug {
			log.SetLevel(logrus.DebugLevel)
		}

		// Reinitialize with the target debug level.
		k8s.InitLogging(
			k8s.LogWriterOption(log.WithField("context", "kubernetes")),
			k8s.LogLevelOption(int(serveCtx.KubernetesDebug)),
		)

		log.Infof("args: %v", args)

		// Validate the result of applying the command-line
		// flags on top of the config file.
		if err := serveCtx.Config.Validate(); err != nil {
			log.WithError(err).Fatal("invalid configuration")
		}

		// Build out serve deps.
		serve, err := NewServer(log, serveCtx)
		if err != nil {
			log.WithError(err).Fatal("unable to initialize Server dependencies required to start Contour")
		}

		if err := serve.doServe(); err != nil {
			log.WithError(err).Fatal("Contour server failed")
		}
	case version.FullCommand():
		println(build.PrintBuildInfo())
	default:
		app.Usage(args)
		os.Exit(2)
	}

}
