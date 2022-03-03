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
	"flag"
	"fmt"
	"os"

	"github.com/projectcontour/contour/internal/provisioner/controller"
	"github.com/projectcontour/contour/internal/provisioner/parse"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func main() {
	config := DefaultConfig()

	flag.StringVar(&config.ContourImage, "contour-image", config.ContourImage,
		"The container image used for the managed Contour.")
	flag.StringVar(&config.EnvoyImage, "envoy-image", config.EnvoyImage,
		"The container image used for the managed Envoy.")
	flag.StringVar(&config.MetricsBindAddress, "metrics-addr", config.MetricsBindAddress,
		"The address the metric endpoint binds to. It can be set to \"0\" to disable serving metrics.")
	flag.BoolVar(&config.LeaderElection, "enable-leader-election", config.LeaderElection,
		"Enable leader election for the operator. Enabling this will ensure there is only one active operator.")
	flag.StringVar(&config.GatewayControllerName, "gateway-controller-name", config.GatewayControllerName,
		"The controller string to process GatewayClasses and Gateways for.")

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	setupLog := ctrl.Log.WithName("setup")

	for _, image := range []string{config.ContourImage, config.EnvoyImage} {
		// Parse will not handle short digests.
		if err := parse.Image(image); err != nil {
			setupLog.Error(err, "invalid image reference", "value", image)
			os.Exit(1)
		}
	}

	setupLog.Info("using contour", "image", config.ContourImage)
	setupLog.Info("using envoy", "image", config.EnvoyImage)

	mgr, err := createManager(ctrl.GetConfigOrDie(), config)
	if err != nil {
		setupLog.Error(err, "failed to create contour operator")
		os.Exit(1)
	}

	setupLog.Info("starting contour operator")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "failed to start contour operator")
		os.Exit(1)
	}
}

// Config is configuration of the operator.
type Config struct {
	// ContourImage is the container image for the Contour container(s) managed
	// by the operator.
	ContourImage string

	// EnvoyImage is the container image for the Envoy container(s) managed
	// by the operator.
	EnvoyImage string

	// MetricsBindAddress is the TCP address that the operator should bind to for
	// serving prometheus metrics. It can be set to "0" to disable the metrics serving.
	MetricsBindAddress string

	// LeaderElection determines whether or not to use leader election when starting
	// the operator.
	LeaderElection bool

	// LeaderElectionID determines the name of the configmap that leader election will
	// use for holding the leader lock.
	LeaderElectionID string

	// GatewayControllerName defines the controller string that this operator instance
	// will process GatewayClasses and Gateways for.
	GatewayControllerName string
}

const (
	DefaultContourImage           = "ghcr.io/projectcontour/contour:main"
	DefaultEnvoyImage             = "docker.io/envoyproxy/envoy:v1.21.1"
	DefaultMetricsAddr            = ":8080"
	DefaultEnableLeaderElection   = false
	DefaultEnableLeaderElectionID = "0d879e31.projectcontour.io"
	DefaultGatewayControllerName  = "projectcontour.io/gateway-provisioner"
)

// DefaultConfig returns an operator config using default values.
func DefaultConfig() *Config {
	return &Config{
		ContourImage:          DefaultContourImage,
		EnvoyImage:            DefaultEnvoyImage,
		MetricsBindAddress:    DefaultMetricsAddr,
		LeaderElection:        DefaultEnableLeaderElection,
		LeaderElectionID:      DefaultEnableLeaderElectionID,
		GatewayControllerName: DefaultGatewayControllerName,
	}
}

// createManager creates a new manager from restConfig and operatorConfig.
func createManager(restConfig *rest.Config, operatorConfig *Config) (manager.Manager, error) {
	scheme, err := createScheme()
	if err != nil {
		return nil, fmt.Errorf("error creating runtime scheme: %w", err)
	}

	mgr, err := ctrl.NewManager(restConfig, manager.Options{
		Scheme:             scheme,
		LeaderElection:     operatorConfig.LeaderElection,
		LeaderElectionID:   operatorConfig.LeaderElectionID,
		MetricsBindAddress: operatorConfig.MetricsBindAddress,
		Logger:             ctrl.Log.WithName("contour_operator"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}

	// Create and register the controllers with the operator manager.
	if _, err := controller.NewGatewayClassController(mgr, operatorConfig.GatewayControllerName); err != nil {
		return nil, fmt.Errorf("failed to create gatewayclass controller: %w", err)
	}
	if _, err := controller.NewGatewayController(mgr, operatorConfig.GatewayControllerName, operatorConfig.ContourImage, operatorConfig.EnvoyImage); err != nil {
		return nil, fmt.Errorf("failed to create gateway controller: %w", err)
	}
	return mgr, nil
}

func createScheme() (*runtime.Scheme, error) {
	// scheme contains all the API types necessary for the operator's dynamic
	// clients to work. Any new non-core types must be added here.
	//
	// NOTE: The discovery mechanism used by the client doesn't automatically
	// refresh, so only add types here that are guaranteed to exist before the
	// operator starts.
	scheme := runtime.NewScheme()

	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := gatewayv1alpha2.AddToScheme(scheme); err != nil {
		return nil, err
	}

	return scheme, nil
}
