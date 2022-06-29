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
	"github.com/projectcontour/contour/internal/envoy"
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

	// Log-format applies to log format of all sub-commands.
	logFormat := app.Flag("log-format", "Log output format for Contour. Either text or json.").Default("text").Enum("text", "json")

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
	cli.Flag("node-id", "Node ID for the CLI client to use").Envar("CLI_NODE_ID").Default("ContourCLI").StringVar(&client.NodeId)
	cli.Flag("nack", "NACK all responses (for testing)").BoolVar(&client.Nack)
	cli.Flag("delta", "Use incremental xDS").BoolVar(&client.Delta)

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

	gatewayProvisioner, gatewayProvisionerConfig := registerGatewayProvisioner(app)

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
			watchDeltaStream(log, stream, resource_v3.ClusterType, resources, client.Nack, client.NodeId)
		} else {
			stream := client.ClusterStream()
			watchstream(log, stream, resource_v3.ClusterType, resources, client.Nack, client.NodeId)
		}
	case eds.FullCommand():
		if client.Delta {
			stream := client.DeltaEndpointStream()
			watchDeltaStream(log, stream, resource_v3.EndpointType, resources, client.Nack, client.NodeId)
		} else {
			stream := client.EndpointStream()
			watchstream(log, stream, resource_v3.EndpointType, resources, client.Nack, client.NodeId)
		}
	case lds.FullCommand():
		if client.Delta {
			stream := client.DeltaListenerStream()
			watchDeltaStream(log, stream, resource_v3.ListenerType, resources, client.Nack, client.NodeId)
		} else {
			stream := client.ListenerStream()
			watchstream(log, stream, resource_v3.ListenerType, resources, client.Nack, client.NodeId)
		}
	case rds.FullCommand():
		if client.Delta {
			stream := client.DeltaRouteStream()
			watchDeltaStream(log, stream, resource_v3.RouteType, resources, client.Nack, client.NodeId)
		} else {
			stream := client.RouteStream()
			watchstream(log, stream, resource_v3.RouteType, resources, client.Nack, client.NodeId)
		}
	case sds.FullCommand():
		if client.Delta {
			stream := client.DeltaRouteStream()
			watchDeltaStream(log, stream, resource_v3.SecretType, resources, client.Nack, client.NodeId)
		} else {
			stream := client.RouteStream()
			watchstream(log, stream, resource_v3.SecretType, resources, client.Nack, client.NodeId)
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
