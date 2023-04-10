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

	"github.com/projectcontour/contour/internal/provisioner"
	"github.com/projectcontour/contour/internal/provisioner/controller"
	"github.com/projectcontour/contour/internal/provisioner/parse"
	"github.com/projectcontour/contour/pkg/config"

	"github.com/alecthomas/kingpin/v2"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func registerGatewayProvisioner(app *kingpin.Application) (*kingpin.CmdClause, *gatewayProvisionerConfig) {
	cmd := app.Command("gateway-provisioner", "Run contour gateway provisioner.")

	provisionerConfig := &gatewayProvisionerConfig{
		contourImage:          "ghcr.io/projectcontour/contour:main",
		envoyImage:            "docker.io/envoyproxy/envoy:v1.25.4",
		metricsBindAddress:    ":8080",
		leaderElection:        false,
		leaderElectionID:      "0d879e31.projectcontour.io",
		gatewayControllerName: "projectcontour.io/gateway-controller",
	}

	cmd.Flag("contour-image", "The container image used for the managed Contour.").
		Default(provisionerConfig.contourImage).
		StringVar(&provisionerConfig.contourImage)

	cmd.Flag("enable-leader-election", "Enable leader election for the gateway provisioner.").
		BoolVar(&provisionerConfig.leaderElection)

	cmd.Flag("envoy-image", "The container image used for the managed Envoy.").
		Default(provisionerConfig.envoyImage).
		StringVar(&provisionerConfig.envoyImage)

	cmd.Flag("gateway-controller-name", "The controller string to process GatewayClasses and Gateways for.").
		Default(provisionerConfig.gatewayControllerName).
		StringVar(&provisionerConfig.gatewayControllerName)

	cmd.Flag("leader-election-namespace", "The namespace in which the leader election resource will be created.").
		Default(config.GetenvOr("CONTOUR_PROVISIONER_NAMESPACE", "projectcontour")).
		StringVar(&provisionerConfig.leaderElectionNamespace)

	cmd.Flag("metrics-addr", "The address the metric endpoint binds to. It can be set to 0 to disable serving metrics.").
		Default(provisionerConfig.metricsBindAddress).
		StringVar(&provisionerConfig.metricsBindAddress)

	return cmd, provisionerConfig
}

type gatewayProvisionerConfig struct {
	// contourImage is the container image for the Contour container(s) managed
	// by the gateway provisioner.
	contourImage string

	// envoyImage is the container image for the Envoy container(s) managed
	// by the gateway provisioner.
	envoyImage string

	// metricsBindAddress is the TCP address that the gateway provisioner should bind to for
	// serving prometheus metrics. It can be set to "0" to disable the metrics serving.
	metricsBindAddress string

	// leaderElection determines whether or not to use leader election when starting
	// the gateway provisioner.
	leaderElection bool

	// leaderElectionID determines the name of the configmap that leader election will
	// use for holding the leader lock.
	leaderElectionID string

	// leaderElectionNamespace determines the namespace in which the leader
	// election resource will be created.
	leaderElectionNamespace string

	// gatewayControllerName defines the controller string that this gateway provisioner instance
	// will process GatewayClasses and Gateways for.
	gatewayControllerName string
}

func runGatewayProvisioner(config *gatewayProvisionerConfig) {
	setupLog := ctrl.Log.WithName("setup")

	for _, image := range []string{config.contourImage, config.envoyImage} {
		// Parse will not handle short digests.
		if err := parse.Image(image); err != nil {
			setupLog.Error(err, "invalid image reference", "value", image)
			os.Exit(1)
		}
	}

	setupLog.Info("using contour", "image", config.contourImage)
	setupLog.Info("using envoy", "image", config.envoyImage)

	mgr, err := createManager(ctrl.GetConfigOrDie(), config)
	if err != nil {
		setupLog.Error(err, "failed to create contour gateway provisioner")
		os.Exit(1)
	}

	setupLog.Info("starting contour gateway provisioner")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "failed to start contour gateway provisioner")
		os.Exit(1)
	}
}

// createManager creates a new manager from restConfig and provisionerConfig.
func createManager(restConfig *rest.Config, provisionerConfig *gatewayProvisionerConfig) (manager.Manager, error) {
	scheme, err := provisioner.CreateScheme()
	if err != nil {
		return nil, fmt.Errorf("error creating runtime scheme: %w", err)
	}

	mgr, err := ctrl.NewManager(restConfig, manager.Options{
		Scheme:                     scheme,
		LeaderElection:             provisionerConfig.leaderElection,
		LeaderElectionResourceLock: "leases",
		LeaderElectionID:           provisionerConfig.leaderElectionID,
		LeaderElectionNamespace:    provisionerConfig.leaderElectionNamespace,
		MetricsBindAddress:         provisionerConfig.metricsBindAddress,
		Logger:                     ctrl.Log.WithName("contour-gateway-provisioner"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	// Create and register the controllers with the manager.
	if _, err := controller.NewGatewayClassController(mgr, provisionerConfig.gatewayControllerName); err != nil {
		return nil, fmt.Errorf("failed to create gatewayclass controller: %w", err)
	}
	if _, err := controller.NewGatewayController(mgr, provisionerConfig.gatewayControllerName, provisionerConfig.contourImage, provisionerConfig.envoyImage); err != nil {
		return nil, fmt.Errorf("failed to create gateway controller: %w", err)
	}
	return mgr, nil
}
