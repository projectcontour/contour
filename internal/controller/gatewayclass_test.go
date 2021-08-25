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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var _ = Describe("GatewayClass Controller", func() {

	Context("Managed GatewayClass", func() {
		It("Should surface admitted status", func() {
			key := types.NamespacedName{Name: "test-gatewayclass-" + rand.String(10)}

			admitted := &gatewayv1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: gatewayv1alpha1.GatewayClassSpec{Controller: gcController},
			}

			// Create
			Expect(cl.Create(context.Background(), admitted)).Should(Succeed())

			By("Expecting admitted status")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return isGatewayClassAdmitted(gc)
			}, timeout, interval).Should(BeTrue())

			// Delete
			By("Expecting successful deletion")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting delete to finish")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				return errors.IsNotFound(cl.Get(context.Background(), key, gc))
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Unmanaged GatewayClass", func() {
		It("Should surface not admitted status", func() {
			// Test a GatewayClass that should not be managed by Contour.
			key := types.NamespacedName{Name: "test-gatewayclass-" + rand.String(10)}
			waiting := &gatewayv1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: gatewayv1alpha1.GatewayClassSpec{Controller: "not-contour"},
			}

			// Create
			Expect(cl.Create(context.Background(), waiting)).Should(Succeed())

			By("Expecting not admitted status condition")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return isGatewayClassAdmitted(gc)
			}, timeout, interval).Should(BeFalse())

			// Delete
			By("Expecting successful deletion")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting delete to finish")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				return errors.IsNotFound(cl.Get(context.Background(), key, gc))
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Multiple GatewayClasses", func() {
		It("Should surface not admitted status on a younger GatewayClass", func() {
			olderKey := types.NamespacedName{Name: "test-gatewayclass-" + rand.String(10)}

			older := &gatewayv1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      olderKey.Name,
					Namespace: olderKey.Namespace,
				},
				Spec: gatewayv1alpha1.GatewayClassSpec{Controller: gcController},
			}

			// Create
			Expect(cl.Create(context.Background(), older)).Should(Succeed())

			By("Expecting admitted status on the older gateway class")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), olderKey, gc)
				return isGatewayClassAdmitted(gc)
			}, timeout, interval).Should(BeTrue())

			youngerKey := types.NamespacedName{Name: "test-gatewayclass-" + rand.String(10)}

			younger := &gatewayv1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      youngerKey.Name,
					Namespace: youngerKey.Namespace,
				},
				Spec: gatewayv1alpha1.GatewayClassSpec{Controller: gcController},
			}

			// Create
			Expect(cl.Create(context.Background(), younger)).Should(Succeed())

			By("Expecting not admitted status on the younger gateway class")
			Consistently(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), youngerKey, gc)
				return isGatewayClassAdmitted(gc)
			}, timeout, interval).Should(BeFalse())

			// Delete older gatewayclass
			By("Expecting successful deletion of the older gateway class")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), olderKey, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting delete to finish")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				return errors.IsNotFound(cl.Get(context.Background(), olderKey, gc))
			}, timeout, interval).Should(BeTrue())

			By("Expecting next oldest gateway class to become admitted")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), youngerKey, gc)
				return isGatewayClassAdmitted(gc)
			}, timeout, interval).Should(BeTrue())

			// Delete younger gatewayclass
			By("Expecting successful deletion of the younger gateway class")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), youngerKey, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting delete to finish")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				return errors.IsNotFound(cl.Get(context.Background(), youngerKey, gc))
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("GatewayClass with parametersRef", func() {
		It("With parametersRefs should be admitted", func() {
			key := types.NamespacedName{Name: "test-gatewayclass-" + rand.String(10)}

			admitted := &gatewayv1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: gatewayv1alpha1.GatewayClassSpec{
					Controller: gcController,
					ParametersRef: &gatewayv1alpha1.ParametersReference{
						Group: "foo",
						Kind:  "bar",
						Name:  "baz",
					},
				},
			}

			// Create
			Expect(cl.Create(context.Background(), admitted)).Should(Succeed())

			By("Expecting admitted status")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return isGatewayClassAdmitted(gc)
			}, timeout, interval).Should(BeTrue())

			// Delete
			By("Expecting successful deletion")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting delete to finish")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				return errors.IsNotFound(cl.Get(context.Background(), key, gc))
			}, timeout, interval).Should(BeTrue())
		})
	})
})
