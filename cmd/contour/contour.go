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
	"context"
	"fmt"
	"os"

	resource_v3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/build"
	"github.com/projectcontour/contour/internal/envoy"
	envoy_v3 "github.com/projectcontour/contour/internal/envoy/v3"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/sirupsen/logrus"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/leaderelection"
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

		// Build out serve deps.
		serve, err := NewServer(log, serveCtx)
		if err != nil {
			log.WithError(err).Fatal("unable to initialize Server dependencies required to start Contour")
		}

		// Get the ContourConfiguration CRD if specified
		contourConfiguration, err := getContourConfiguration(serveCtx, serve.clients)
		if err != nil {
			log.WithError(err).Fatal("error processing Contour Configuration")
		}

		// Set up workgroup runner.
		var contourGroup workgroup.Group

		// Register leadership election.
		var isLeader chan struct{}
		var le *leaderelection.LeaderElector
		var leaderChanged chan string
		if contourConfiguration.Spec.LeaderElection.DisableLeaderElection {
			isLeader = disableLeaderElection(log)
		} else {
			isLeader, leaderChanged, le = setupLeadershipElection(&contourGroup, log, contourConfiguration.Spec.LeaderElection, serve.clients)
		}

		// Start up Contour Group
		go func() {
			if err := contourGroup.Run(context.Background()); err != nil {
				log.WithError(err).Fatal("error running Contour Group")
			}
		}()

		if !contourConfiguration.Spec.LeaderElection.DisableLeaderElection {
			// wait for leader to be elected or found
			<-leaderChanged
		}

		// If ContourConfiguration is specified and LeaderElection is disabled, or we're the leader set status on the object.
		if len(serveCtx.contourConfigurationName) > 0 && (contourConfiguration.Spec.LeaderElection.DisableLeaderElection || le.IsLeader()) {
			// Set valid status on the ContourConfiguration object
			if err := serve.sh.SetValidContourConfigurationStatus(contourConfiguration); err != nil {
				log.WithError(err).Fatal("could not set status on the ContourConfiguration object")
			}
		}

		if err := serve.doServe(contourConfiguration, isLeader); err != nil {
			log.WithError(err).Fatal("Contour server failed")
		}
	case version.FullCommand():
		println(build.PrintBuildInfo())
	default:
		app.Usage(args)
		os.Exit(2)
	}

}

// getContourConfiguration returns a specified ContourConfiguration or converts a ServeContext into a ContourConfiguration.
func getContourConfiguration(serveCtx *serveContext, clients *k8s.Clients) (*contour_api_v1alpha1.ContourConfiguration, error) {
	if len(serveCtx.contourConfigurationName) > 0 {
		// Determine the name/namespace of the configuration resource utilizing the environment
		// variable "CONTOUR_NAMESPACE" which should exist on the Contour deployment.
		//
		// If the env variable is not present, it will default to "projectcontour".
		contourNamespace, found := os.LookupEnv("CONTOUR_NAMESPACE")
		if !found {
			contourNamespace = "projectcontour"
		}

		namespacedName := types.NamespacedName{Name: serveCtx.contourConfigurationName, Namespace: contourNamespace}
		client := clients.DynamicClient().Resource(contour_api_v1alpha1.ContourConfigurationGVR).Namespace(namespacedName.Namespace)

		// ensure the specified ContourConfiguration exists
		res, err := client.Get(context.Background(), namespacedName.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error getting contour configuration %s; %v", namespacedName, err)
		}

		var contourConfig contour_api_v1alpha1.ContourConfiguration
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object, &contourConfig); err != nil {
			return nil, fmt.Errorf("error converting contour configuration %s; %v", namespacedName, err)
		}

		return &contourConfig, nil
	}
	// No contour configuration passed, so convert the ServeContext into a ContourConfigurationSpec.
	return &contour_api_v1alpha1.ContourConfiguration{
		Spec: *serveCtx.convertToContourConfigurationSpec(),
	}, nil
}
