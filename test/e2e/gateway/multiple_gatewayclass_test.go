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

// +build e2e

package gateway

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega/gexec"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

// Tests in this block set up/tear down their own GatewayClasses and Gateways.
var _ = Describe("GatewayClass/Gateway admission tests", func() {
	var (
		contourCmd            *gexec.Session
		contourConfig         *config.Parameters
		contourConfigFile     string
		additionalContourArgs []string
		controllerName        string
	)

	BeforeEach(func() {
		controllerName = fmt.Sprintf("projectcontour.io/projectcontour/contour-%d", getRandomNumber())

		// Contour config file contents, can be modified in nested
		// BeforeEach.
		contourConfig = &config.Parameters{
			GatewayConfig: &config.GatewayParameters{
				ControllerName: controllerName,
			},
		}

		// Default contour serve command line arguments can be appended to in
		// nested BeforeEach.
		additionalContourArgs = []string{}
	})

	// JustBeforeEach is called after each of the nested BeforeEach are
	// called, so it is a final setup step before running a test.
	// A nested BeforeEach may have modified Contour config, so we wait
	// until here to start Contour.
	JustBeforeEach(func() {
		var err error
		contourCmd, contourConfigFile, err = f.Deployment.StartLocalContour(contourConfig, additionalContourArgs...)
		require.NoError(f.T(), err)

		// Wait for Envoy to be healthy.
		require.NoError(f.T(), f.Deployment.WaitForEnvoyDaemonSetUpdated())
	})

	AfterEach(func() {
		require.NoError(f.T(), f.Client.DeleteAllOf(context.Background(), &gatewayv1alpha1.GatewayClass{}))
		require.NoError(f.T(), f.Deployment.StopLocalContour(contourCmd, contourConfigFile))
	})

	f.NamespacedTest("gateway-multiple-gatewayclasses", func(namespace string) {
		Specify("only the oldest matching gatewayclass should be admitted", func() {
			newGatewayClass := func(name, controller string) *gatewayv1alpha1.GatewayClass {
				return &gatewayv1alpha1.GatewayClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
					},
					Spec: gatewayv1alpha1.GatewayClassSpec{
						Controller: controller,
					},
				}
			}

			// create a non-matching GC: should not be admitted
			nonMatching := newGatewayClass("non-matching-gatewayclass", "non-matching-controller")

			require.NoError(f.T(), f.Client.Create(context.Background(), nonMatching))
			require.Never(f.T(), func() bool {
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(nonMatching), nonMatching); err != nil {
					return true
				}
				return gatewayClassValid(nonMatching)
			}, 5*time.Second, time.Second)

			// create a matching GC: should be admitted
			oldest := newGatewayClass("oldest-matching-gatewayclass", controllerName)
			_, valid := f.CreateGatewayClassAndWaitFor(oldest, gatewayClassValid)
			require.True(f.T(), valid)

			// create another matching GC: should not be admitted since it's not oldest
			secondOldest := newGatewayClass("second-oldest-matching-gatewayclass", controllerName)
			_, notOldest := f.CreateGatewayClassAndWaitFor(secondOldest, func(gc *gatewayv1alpha1.GatewayClass) bool {
				for _, cond := range gc.Status.Conditions {
					if cond.Type == "Admitted" &&
						cond.Status == metav1.ConditionFalse &&
						cond.Reason == "Invalid" &&
						cond.Message == "Invalid GatewayClass: another older GatewayClass with the same Spec.Controller exists" {
						return true
					}
				}
				return false
			})
			require.True(f.T(), notOldest)

			// double-check that the oldest matching GC is still admitted
			require.NoError(f.T(), f.Client.Get(context.Background(), k8s.NamespacedNameOf(oldest), oldest))
			require.True(f.T(), gatewayClassValid(oldest))

			// delete the first matching GC: second one should now be admitted
			require.NoError(f.T(), f.Client.Delete(context.Background(), oldest))
			require.Eventually(f.T(), func() bool {
				if err := f.Client.Get(context.Background(), k8s.NamespacedNameOf(secondOldest), secondOldest); err != nil {
					return false
				}
				return gatewayClassValid(secondOldest)
			}, f.RetryTimeout, f.RetryInterval)
		})
	})
})
