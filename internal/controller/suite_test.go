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
	"crypto/rand"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/bombsimon/logrusr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	charset      = "abcdefghijklmnopqrstuvwxyz"
	gcController = "test.io/contour"
)

var (
	cl       client.Client
	mgr      ctrl.Manager
	testEnv  *envtest.Environment
	timeout  = time.Second * 10
	interval = time.Second * 1
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Runtime Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	log := logrus.New()
	log.Out = GinkgoWriter
	log.Level = logrus.DebugLevel
	logf.SetLogger(logrusr.NewLogger(log))

	By("Bootstrapping the test environment")
	gatewayCRDs := filepath.Join("..", "..", "examples", "gateway", "00-crds.yaml")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{gatewayCRDs},
	}

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = scheme.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = gatewayv1alpha1.Install(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Setup a Manager
	mgr, err = ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	err = (&gatewayClassReconciler{
		client:     mgr.GetClient(),
		log:        log,
		controller: gcController,
	}).SetupWithManager(mgr)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		err = mgr.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	cl = mgr.GetClient()
	Expect(cl).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("Expecting the test environment teardown to complete")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

// SetupWithManager adds the controller manager
func (r *gatewayClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.GatewayClass{}).
		Complete(r)
}

// generateRandomString returns a securely generated random string with the provided
// length. It will return an error if the system's secure random number generator
// fails to function correctly, in which case the caller should not continue.
func generateRandomString(length int, charset string) (string, error) {
	ret := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		ret[i] = charset[num.Int64()]
	}

	return string(ret), nil
}

// isAdmitted returns true if gc status is "Admitted".
func isAdmitted(gc *gatewayv1alpha1.GatewayClass) bool {
	for _, c := range gc.Status.Conditions {
		if c.Type == string(gatewayv1alpha1.GatewayClassConditionStatusAdmitted) &&
			c.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

// isWaiting returns true if gc status is "Waiting".
func isWaiting(gc *gatewayv1alpha1.GatewayClass) bool {
	for _, c := range gc.Status.Conditions {
		if c.Type == string(gatewayv1alpha1.GatewayClassConditionStatusAdmitted) &&
			c.Status == metav1.ConditionFalse &&
			c.Reason == string(gatewayv1alpha1.GatewayClassNotAdmittedWaiting) {
			return true
		}
	}

	return false
}
