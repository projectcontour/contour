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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bombsimon/logrusr/v4"
	"github.com/distribution/reference"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayapi_v1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	conformance_v1alpha1 "sigs.k8s.io/gateway-api/conformance/apis/v1alpha1"
	"sigs.k8s.io/gateway-api/conformance/tests"
	"sigs.k8s.io/gateway-api/conformance/utils/flags"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
	"sigs.k8s.io/yaml"
)

func TestGatewayConformance(t *testing.T) {
	log.SetLogger(logrusr.New(logrus.StandardLogger()))

	cfg, err := config.GetConfig()
	require.NoError(t, err)

	client, err := client.New(cfg, client.Options{})
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)

	require.NoError(t, gatewayapi_v1alpha2.AddToScheme(client.Scheme()))
	require.NoError(t, gatewayapi_v1beta1.AddToScheme(client.Scheme()))
	require.NoError(t, gatewayapi_v1.AddToScheme(client.Scheme()))

	cSuiteOptions := suite.Options{
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

			// Not implemented yet since it's functionally equivalent
			// to Timeouts.Request, to be enabled once Gateway API
			// supports retries.
			// See: https://github.com/projectcontour/contour/issues/6000
			tests.HTTPRouteTimeoutBackendRequest.ShortName,

			// Contour supports the positive-case functionality,
			// but there are some negative cases that aren't fully
			// implemented plus complications with the test setup itself.
			// See: https://github.com/projectcontour/contour/issues/5922
			tests.GatewayStaticAddresses.ShortName,
		},
		ExemptFeatures: sets.New(
			suite.SupportMesh,
		),
	}
	if os.Getenv("GENERATE_GATEWAY_CONFORMANCE_REPORT") == "true" {
		reportDir, ok := os.LookupEnv("GATEWAY_CONFORMANCE_REPORT_OUTDIR")
		require.True(t, ok, "GATEWAY_CONFORMANCE_REPORT_OUTDIR not set")

		image, ok := os.LookupEnv("CONTOUR_E2E_IMAGE")
		require.True(t, ok, "CONTOUR_E2E_IMAGE not set")

		imageRef, err := reference.Parse(image)
		require.NoErrorf(t, err, "CONTOUR_E2E_IMAGE invalid image ref: %s", imageRef)
		taggedImage, ok := imageRef.(reference.NamedTagged)
		require.True(t, ok)
		require.NotEmpty(t, taggedImage)

		// Workaround since the experimental suite doesn't properly
		// exclude tests we don't want to run using the ExemptFeatures
		// field.
		cSuiteOptions.EnableAllSupportedFeatures = false
		cSuiteOptions.SupportedFeatures = suite.AllFeatures.Delete(suite.MeshCoreFeatures.UnsortedList()...)

		cSuite, err := suite.NewExperimentalConformanceTestSuite(suite.ExperimentalConformanceOptions{
			Options: cSuiteOptions,
			Implementation: conformance_v1alpha1.Implementation{
				Organization: "projectcontour",
				Project:      "contour",
				URL:          "https://projectcontour.io/",
				Version:      taggedImage.Tag(),
				Contact:      []string{"@projectcontour/maintainers"},
			},
			ConformanceProfiles: sets.New[suite.ConformanceProfileName](
				suite.HTTPConformanceProfileName,
				suite.TLSConformanceProfileName,
			),
		})
		require.NoError(t, err)

		cSuite.Setup(t)
		require.NoError(t, cSuite.Run(t, tests.ConformanceTests))

		report, err := cSuite.Report()
		require.NoError(t, err, "failed generating conformance report")

		if gwAPIVersion := os.Getenv("GATEWAY_API_VERSION"); gwAPIVersion != "" {
			report.GatewayAPIVersion = gwAPIVersion
		}

		rawReport, err := yaml.Marshal(report)
		require.NoError(t, err)
		t.Logf("Conformance report:\n%s", string(rawReport))

		require.NoError(t, os.MkdirAll(reportDir, 0o755))
		outFile := filepath.Join(reportDir, fmt.Sprintf("projectcontour-contour-%d.yaml", time.Now().UnixNano()))
		require.NoError(t, os.WriteFile(outFile, rawReport, 0o600))
		t.Logf("Report written to: %s", outFile)
	} else {
		cSuite := suite.New(cSuiteOptions)
		cSuite.Setup(t)
		cSuite.Run(t, tests.ConformanceTests)
	}
}
