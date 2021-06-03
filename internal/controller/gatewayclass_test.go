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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

var _ = Describe("GatewayClass Controller", func() {
	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	Context("Managed GatewayClass", func() {
		It("Should surface admitted status", func() {

			gen, err := generateRandomString(10, charset)
			Expect(err).NotTo(HaveOccurred())

			key := types.NamespacedName{Name: "test-gatewayclass-" + gen}

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
				return isAdmitted(gc)
			}, timeout, interval).Should(BeTrue())

			// Delete
			By("Expecting to delete successfully")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting to delete finish")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				return cl.Get(context.Background(), key, gc)
			}, timeout, interval).ShouldNot(Succeed())
		})
	})
	Context("Unmanaged GatewayClass", func() {
		It("Should surface waiting status", func() {

			// Test a GatewayClass that should not be managed by Contour.
			gen, err := generateRandomString(10, charset)
			Expect(err).NotTo(HaveOccurred())

			key := types.NamespacedName{Name: "test-gatewayclass-" + gen}
			waiting := &gatewayv1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
				},
				Spec: gatewayv1alpha1.GatewayClassSpec{Controller: "not-contour"},
			}

			// Create
			Expect(cl.Create(context.Background(), waiting)).Should(Succeed())

			By("Expecting waiting status")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return isWaiting(gc)
			}, timeout, interval).Should(BeTrue())

			// Delete
			By("Expecting to delete successfully")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting to delete finish")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				return cl.Get(context.Background(), key, gc)
			}, timeout, interval).ShouldNot(Succeed())
		})
	})
	Context("Multiple GatewayClasses", func() {
		It("Should surface not admitted status", func() {

			gen, err := generateRandomString(10, charset)
			Expect(err).NotTo(HaveOccurred())

			admittedKey := types.NamespacedName{Name: "test-gatewayclass-" + gen}

			admitted := &gatewayv1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      admittedKey.Name,
					Namespace: admittedKey.Namespace,
				},
				Spec: gatewayv1alpha1.GatewayClassSpec{Controller: gcController},
			}

			// Create
			Expect(cl.Create(context.Background(), admitted)).Should(Succeed())

			By("Expecting admitted status")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), admittedKey, gc)
				return isAdmitted(gc)
			}, timeout, interval).Should(BeTrue())

			gen, err = generateRandomString(10, charset)
			Expect(err).NotTo(HaveOccurred())

			notAdmittedKey := types.NamespacedName{Name: "test-gatewayclass-" + gen}

			notAdmitted := &gatewayv1alpha1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      notAdmittedKey.Name,
					Namespace: notAdmittedKey.Namespace,
				},
				Spec: gatewayv1alpha1.GatewayClassSpec{Controller: gcController},
			}

			// Create
			Expect(cl.Create(context.Background(), notAdmitted)).Should(Succeed())

			By("Expecting not admitted status")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), notAdmittedKey, gc)
				return isAdmitted(gc)
			}, timeout, interval).Should(BeFalse())

			// Delete admitted gatewayclass
			By("Expecting to delete successfully")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), admittedKey, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting to delete finish")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				return cl.Get(context.Background(), admittedKey, gc)
			}, timeout, interval).ShouldNot(Succeed())

			// Delete non-admitted gatewayclass
			By("Expecting to delete successfully")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), notAdmittedKey, gc)
				return cl.Delete(context.Background(), gc)
			}, timeout, interval).Should(Succeed())

			By("Expecting to delete finish")
			Eventually(func() error {
				gc := &gatewayv1alpha1.GatewayClass{}
				return cl.Get(context.Background(), notAdmittedKey, gc)
			}, timeout, interval).ShouldNot(Succeed())
		})
	})
	Context("Managed GatewayClass", func() {
		It("With parameterRefs should not be admitted", func() {

			gen, err := generateRandomString(10, charset)
			Expect(err).NotTo(HaveOccurred())

			key := types.NamespacedName{Name: "test-gatewayclass-" + gen}

			unsupported := &gatewayv1alpha1.GatewayClass{
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
			Expect(cl.Create(context.Background(), unsupported)).Should(Succeed())

			By("Expecting not admitted status")
			Eventually(func() bool {
				gc := &gatewayv1alpha1.GatewayClass{}
				_ = cl.Get(context.Background(), key, gc)
				return isAdmitted(gc)
			}, timeout, interval).Should(BeFalse())
		})
	})
})
