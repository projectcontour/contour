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

//go:build conformance

package gatewayapi

import (
	"testing"

	"github.com/bombsimon/logrusr/v4"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/conformance/tests"
	"sigs.k8s.io/gateway-api/conformance/utils/flags"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
)

func TestGatewayConformance(t *testing.T) {
	log.SetLogger(logrusr.New(logrus.StandardLogger()))

	cfg, err := config.GetConfig()
	require.NoError(t, err)

	client, err := client.New(cfg, client.Options{})
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)

	require.NoError(t, v1alpha2.AddToScheme(client.Scheme()))
	require.NoError(t, v1beta1.AddToScheme(client.Scheme()))

	cSuite := suite.New(suite.Options{
		Client: client,
		// This clientset is needed in addition to the client only because
		// controller-runtime client doesn't support non CRUD sub-resources yet (https://github.com/kubernetes-sigs/controller-runtime/issues/452).
		Clientset:                  clientset,
		GatewayClassName:           *flags.GatewayClassName,
		Debug:                      *flags.ShowDebug,
		CleanupBaseResources:       *flags.CleanupBaseResources,
		EnableAllSupportedFeatures: true,
		// Keep the list of skipped features in sync with
		// test/scripts/run-gateway-conformance.sh.
		SkipTests: []string{
			// Checks for the original request port in the returned Location
			// header which Envoy is stripping.
			// See: https://github.com/envoyproxy/envoy/issues/17318
			tests.HTTPRouteRedirectPortAndScheme.ShortName,
		},
		ExemptFeatures: sets.New(
			suite.SupportMesh,
		),
	})
	cSuite.Setup(t)
	cSuite.Run(t, tests.ConformanceTests)

}
