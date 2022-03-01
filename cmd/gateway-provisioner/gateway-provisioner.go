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
	"os"

	"github.com/projectcontour/contour/internal/provisioner/operator"
	"github.com/projectcontour/contour/internal/provisioner/parse"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	config := operator.DefaultConfig()

	flag.StringVar(&config.ContourImage, "contour-image", config.ContourImage,
		"The container image used for the managed Contour.")
	flag.StringVar(&config.EnvoyImage, "envoy-image", config.EnvoyImage,
		"The container image used for the managed Envoy.")
	flag.StringVar(&config.MetricsBindAddress, "metrics-addr", config.MetricsBindAddress, "The "+
		"address the metric endpoint binds to. It can be set to \"0\" to disable serving metrics.")
	flag.BoolVar(&config.LeaderElection, "enable-leader-election", config.LeaderElection,
		"Enable leader election for the operator. Enabling this will ensure there is only one active operator.")

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

	op, err := operator.New(ctrl.GetConfigOrDie(), config)
	if err != nil {
		setupLog.Error(err, "failed to create contour operator")
		os.Exit(1)
	}

	setupLog.Info("starting contour operator")
	if err := op.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "failed to start contour operator")
		os.Exit(1)
	}
}
