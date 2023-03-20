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

//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/onsi/gomega/gexec"
	contour_api_v1alpha1 "github.com/projectcontour/contour/apis/projectcontour/v1alpha1"
	"github.com/projectcontour/contour/internal/ref"
	"github.com/projectcontour/contour/pkg/config"
	"gopkg.in/yaml.v3"
	apps_v1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	apimachinery_util_yaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnvoyDeploymentMode determines how Envoy is deployed (daemonset or deployment)
type EnvoyDeploymentMode string

const (
	DaemonsetMode  EnvoyDeploymentMode = "daemonset"
	DeploymentMode EnvoyDeploymentMode = "deployment"
)

type Deployment struct {
	// k8s client
	client client.Client

	// Command output is written to this writer.
	cmdOutputWriter io.Writer

	// Path to kube config to use with a local Contour.
	kubeConfig string
	// Hostname to use when running Contour locally.
	localContourHost string
	// Port for local Contour to bind to.
	localContourPort string
	// Path to Contour binary for use when running locally.
	contourBin string

	// Contour image to use in in-cluster deployment.
	contourImage string

	// EnvoyDeploymentMode determines how Envoy is deployed (daemonset or deployment)
	EnvoyDeploymentMode

	Namespace                 *v1.Namespace
	ContourServiceAccount     *v1.ServiceAccount
	EnvoyServiceAccount       *v1.ServiceAccount
	ContourConfigMap          *v1.ConfigMap
	CertgenServiceAccount     *v1.ServiceAccount
	CertgenRoleBinding        *rbac_v1.RoleBinding
	CertgenRole               *rbac_v1.Role
	CertgenJob                *batch_v1.Job
	ContourClusterRoleBinding *rbac_v1.ClusterRoleBinding
	ContourRoleBinding        *rbac_v1.RoleBinding
	ContourClusterRole        *rbac_v1.ClusterRole
	ContourRole               *rbac_v1.Role
	ContourService            *v1.Service
	EnvoyService              *v1.Service
	ContourDeployment         *apps_v1.Deployment
	EnvoyDaemonSet            *apps_v1.DaemonSet
	EnvoyDeployment           *apps_v1.Deployment

	// Optional volumes that will be attached to Envoy daemonset.
	EnvoyExtraVolumes      []v1.Volume
	EnvoyExtraVolumeMounts []v1.VolumeMount

	// Ratelimit deployment.
	RateLimitDeployment       *apps_v1.Deployment
	RateLimitService          *v1.Service
	RateLimitExtensionService *contour_api_v1alpha1.ExtensionService

	// Global External Authorization deployment.
	GlobalExtAuthDeployment       *apps_v1.Deployment
	GlobalExtAuthService          *v1.Service
	GlobalExtAuthExtensionService *contour_api_v1alpha1.ExtensionService
}

// UnmarshalResources unmarshals resources from rendered Contour manifest in
// order.
// Note: This will need to be updated if any new resources are added to the
// rendered deployment manifest.
func (d *Deployment) UnmarshalResources() error {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("could not get path to this source file (test/e2e/deployment.go)")
	}
	renderedDaemonsetManifestPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "render", "contour.yaml")
	daemonsetFile, err := os.Open(renderedDaemonsetManifestPath)
	if err != nil {
		return err
	}
	defer daemonsetFile.Close()
	decoder := apimachinery_util_yaml.NewYAMLToJSONDecoder(daemonsetFile)

	// Discard empty document.
	if err := decoder.Decode(new(struct{})); err != nil {
		return err
	}

	renderedDeploymentManifestPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "deployment", "03-envoy-deployment.yaml")
	deploymentFile, err := os.Open(renderedDeploymentManifestPath)
	if err != nil {
		return err
	}
	defer deploymentFile.Close()
	decoderDeployment := apimachinery_util_yaml.NewYAMLToJSONDecoder(deploymentFile)

	d.Namespace = new(v1.Namespace)
	d.ContourServiceAccount = new(v1.ServiceAccount)
	d.EnvoyServiceAccount = new(v1.ServiceAccount)
	d.ContourConfigMap = new(v1.ConfigMap)
	d.CertgenServiceAccount = new(v1.ServiceAccount)
	d.CertgenRoleBinding = new(rbac_v1.RoleBinding)
	d.CertgenRole = new(rbac_v1.Role)
	d.CertgenJob = new(batch_v1.Job)
	d.ContourClusterRoleBinding = new(rbac_v1.ClusterRoleBinding)
	d.ContourRoleBinding = new(rbac_v1.RoleBinding)
	d.ContourClusterRole = new(rbac_v1.ClusterRole)
	d.ContourRole = new(rbac_v1.Role)
	d.ContourService = new(v1.Service)
	d.EnvoyService = new(v1.Service)
	d.ContourDeployment = new(apps_v1.Deployment)
	d.EnvoyDaemonSet = new(apps_v1.DaemonSet)
	d.EnvoyDeployment = new(apps_v1.Deployment)
	objects := []interface{}{
		d.Namespace,
		d.ContourServiceAccount,
		d.EnvoyServiceAccount,
		d.ContourConfigMap,
		// CRDs are installed at cluster setup time so we can ignore them.
		&apiextensions_v1.CustomResourceDefinition{},
		&apiextensions_v1.CustomResourceDefinition{},
		&apiextensions_v1.CustomResourceDefinition{},
		&apiextensions_v1.CustomResourceDefinition{},
		&apiextensions_v1.CustomResourceDefinition{},
		d.CertgenServiceAccount,
		d.CertgenRoleBinding,
		d.CertgenRole,
		d.CertgenJob,
		d.ContourClusterRoleBinding,
		d.ContourRoleBinding,
		d.ContourClusterRole,
		d.ContourRole,
		d.ContourService,
		d.EnvoyService,
		d.ContourDeployment,
	}
	for _, o := range objects {
		if err := decoder.Decode(o); err != nil {
			return err
		}
	}

	switch d.EnvoyDeploymentMode {
	case DeploymentMode:
		if err := decoderDeployment.Decode(d.EnvoyDeployment); err != nil {
			return err
		}
	case DaemonsetMode:
		if err := decoder.Decode(d.EnvoyDaemonSet); err != nil {
			return err
		}
	}

	// ratelimit
	rateLimitExamplePath := filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "ratelimit")
	rateLimitDeploymentFile := filepath.Join(rateLimitExamplePath, "02-ratelimit.yaml")
	rateLimitExtSvcFile := filepath.Join(rateLimitExamplePath, "03-ratelimit-extsvc.yaml")

	rLDFile, err := os.Open(rateLimitDeploymentFile)
	if err != nil {
		return err
	}
	defer rLDFile.Close()
	decoder = apimachinery_util_yaml.NewYAMLToJSONDecoder(rLDFile)
	d.RateLimitDeployment = new(apps_v1.Deployment)
	if err := decoder.Decode(d.RateLimitDeployment); err != nil {
		return err
	}
	d.RateLimitService = new(v1.Service)
	if err := decoder.Decode(d.RateLimitService); err != nil {
		return err
	}

	rLESFile, err := os.Open(rateLimitExtSvcFile)
	if err != nil {
		return err
	}
	defer rLESFile.Close()
	decoder = apimachinery_util_yaml.NewYAMLToJSONDecoder(rLESFile)
	d.RateLimitExtensionService = new(contour_api_v1alpha1.ExtensionService)

	if err := decoder.Decode(d.RateLimitExtensionService); err != nil {
		return err
	}

	// Global external auth
	globalExtAuthExamplePath := filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "global-external-auth")
	globalExtAuthServerDeploymentFile := filepath.Join(globalExtAuthExamplePath, "01-authserver.yaml")
	globalExtAuthExtSvcFile := filepath.Join(globalExtAuthExamplePath, "02-globalextauth-extsvc.yaml")

	rGlobalExtAuthDeploymentFile, err := os.Open(globalExtAuthServerDeploymentFile)
	if err != nil {
		return err
	}
	defer rGlobalExtAuthDeploymentFile.Close()
	decoder = apimachinery_util_yaml.NewYAMLToJSONDecoder(rGlobalExtAuthDeploymentFile)
	d.GlobalExtAuthDeployment = new(apps_v1.Deployment)
	if err := decoder.Decode(d.GlobalExtAuthDeployment); err != nil {
		return err
	}
	d.GlobalExtAuthService = new(v1.Service)
	if err := decoder.Decode(d.GlobalExtAuthService); err != nil {
		return err
	}

	rGlobalExtAuthExtSvcFile, err := os.Open(globalExtAuthExtSvcFile)
	if err != nil {
		return err
	}
	defer rGlobalExtAuthExtSvcFile.Close()
	decoder = apimachinery_util_yaml.NewYAMLToJSONDecoder(rGlobalExtAuthExtSvcFile)
	d.GlobalExtAuthExtensionService = new(contour_api_v1alpha1.ExtensionService)

	return decoder.Decode(d.GlobalExtAuthExtensionService)
}

// Common case of updating object if exists, create otherwise.
func (d *Deployment) ensureResource(new, existing client.Object) error {
	if err := d.client.Get(context.TODO(), client.ObjectKeyFromObject(new), existing); err != nil {
		if api_errors.IsNotFound(err) {
			return d.client.Create(context.TODO(), new)
		}
		return err
	}
	new.SetResourceVersion(existing.GetResourceVersion())
	// If a v1.Service, pass along existing cluster IP and healthcheck node port.
	if newS, ok := new.(*v1.Service); ok {
		existingS := existing.(*v1.Service)
		newS.Spec.ClusterIP = existingS.Spec.ClusterIP
		newS.Spec.ClusterIPs = existingS.Spec.ClusterIPs
		newS.Spec.HealthCheckNodePort = existingS.Spec.HealthCheckNodePort
	}
	return d.client.Update(context.TODO(), new)
}

func (d *Deployment) EnsureNamespace() error {
	return d.ensureResource(d.Namespace, new(v1.Namespace))
}

func (d *Deployment) EnsureContourServiceAccount() error {
	return d.ensureResource(d.ContourServiceAccount, new(v1.ServiceAccount))
}

func (d *Deployment) EnsureEnvoyServiceAccount() error {
	return d.ensureResource(d.EnvoyServiceAccount, new(v1.ServiceAccount))
}

func (d *Deployment) EnsureContourConfigMap() error {
	return d.ensureResource(d.ContourConfigMap, new(v1.ConfigMap))
}

func (d *Deployment) EnsureCertgenServiceAccount() error {
	return d.ensureResource(d.CertgenServiceAccount, new(v1.ServiceAccount))
}

func (d *Deployment) EnsureCertgenRoleBinding() error {
	return d.ensureResource(d.CertgenRoleBinding, new(rbac_v1.RoleBinding))
}

func (d *Deployment) EnsureCertgenRole() error {
	return d.ensureResource(d.CertgenRole, new(rbac_v1.Role))
}

func (d *Deployment) EnsureCertgenJob() error {
	// Delete job if exists with same name, then create.
	tempJ := new(batch_v1.Job)
	jobDeleted := func() (bool, error) {
		return api_errors.IsNotFound(d.client.Get(context.TODO(), client.ObjectKeyFromObject(d.CertgenJob), tempJ)), nil
	}
	if ok, _ := jobDeleted(); !ok {
		if err := d.client.Delete(context.TODO(), tempJ); err != nil {
			return err
		}
	}
	if err := wait.PollImmediate(time.Millisecond*50, time.Minute, jobDeleted); err != nil {
		return err
	}
	return d.client.Create(context.TODO(), d.CertgenJob)
}

func (d *Deployment) EnsureContourClusterRoleBinding() error {
	return d.ensureResource(d.ContourClusterRoleBinding, new(rbac_v1.ClusterRoleBinding))
}

func (d *Deployment) EnsureContourRoleBinding() error {
	return d.ensureResource(d.ContourRoleBinding, new(rbac_v1.RoleBinding))
}

func (d *Deployment) EnsureContourClusterRole() error {
	return d.ensureResource(d.ContourClusterRole, new(rbac_v1.ClusterRole))
}

func (d *Deployment) EnsureContourRole() error {
	return d.ensureResource(d.ContourRole, new(rbac_v1.Role))
}

func (d *Deployment) EnsureContourService() error {
	return d.ensureResource(d.ContourService, new(v1.Service))
}

func (d *Deployment) EnsureEnvoyService() error {
	return d.ensureResource(d.EnvoyService, new(v1.Service))
}

func (d *Deployment) EnsureContourDeployment() error {
	return d.ensureResource(d.ContourDeployment, new(apps_v1.Deployment))
}

func (d *Deployment) WaitForContourDeploymentUpdated() error {
	// List pods with app label "contour" and check that pods are updated
	// with expected container image and in ready state.
	// We do this instead of checking Deployment status as it is possible
	// for it not to have been updated yet and replicas not yet been shut
	// down.

	if len(d.ContourDeployment.Spec.Template.Spec.Containers) != 1 {
		return errors.New("invalid Contour Deployment containers spec")
	}
	labelSelectAppContour := labels.SelectorFromSet(d.ContourDeployment.Spec.Selector.MatchLabels)
	updatedPods := func() (bool, error) {
		updatedPods := d.getPodsUpdatedWithContourImage(labelSelectAppContour, d.EnvoyDaemonSet.Namespace)
		return updatedPods == int(*d.ContourDeployment.Spec.Replicas), nil
	}
	return wait.PollImmediate(time.Millisecond*50, time.Minute, updatedPods)
}

func (d *Deployment) EnsureEnvoyDaemonSet() error {
	return d.ensureResource(d.EnvoyDaemonSet, new(apps_v1.DaemonSet))
}

func (d *Deployment) EnsureEnvoyDeployment() error {
	return d.ensureResource(d.EnvoyDeployment, new(apps_v1.Deployment))
}

func (d *Deployment) WaitForEnvoyUpdated() error {
	if d.EnvoyDeploymentMode == DaemonsetMode {
		return d.waitForEnvoyDaemonSetUpdated()
	}
	return d.waitForEnvoyDeploymentUpdated()
}

func (d *Deployment) waitForEnvoyDaemonSetUpdated() error {
	labelSelectAppEnvoy := labels.SelectorFromSet(d.EnvoyDaemonSet.Spec.Selector.MatchLabels)
	updatedPods := func() (bool, error) {
		ds := &apps_v1.DaemonSet{}
		if err := d.client.Get(context.TODO(), types.NamespacedName{Name: d.EnvoyDaemonSet.Name, Namespace: d.EnvoyDaemonSet.Namespace}, ds); err != nil {
			return false, err
		}
		updatedPods := int(ds.Status.DesiredNumberScheduled)
		if len(ds.Spec.Template.Spec.Containers) > 1 {
			updatedPods = d.getPodsUpdatedWithContourImage(labelSelectAppEnvoy, d.EnvoyDaemonSet.Namespace)
		}
		return updatedPods == int(ds.Status.DesiredNumberScheduled) &&
			ds.Status.NumberReady > 0, nil
	}
	return wait.PollImmediate(time.Millisecond*50, time.Minute*3, updatedPods)
}

func (d *Deployment) waitForEnvoyDeploymentUpdated() error {
	labelSelectAppEnvoy := labels.SelectorFromSet(d.EnvoyDeployment.Spec.Selector.MatchLabels)
	updatedPods := func() (bool, error) {
		dp := new(apps_v1.Deployment)
		if err := d.client.Get(context.TODO(), client.ObjectKeyFromObject(d.EnvoyDeployment), dp); err != nil {
			return false, err
		}
		updatedPods := int(dp.Status.UpdatedReplicas)
		if len(dp.Spec.Template.Spec.Containers) > 1 {
			updatedPods = d.getPodsUpdatedWithContourImage(labelSelectAppEnvoy, d.EnvoyDaemonSet.Namespace)
		}
		return updatedPods == int(*d.EnvoyDeployment.Spec.Replicas) &&
			int(dp.Status.ReadyReplicas) == updatedPods &&
			int(dp.Status.UnavailableReplicas) == 0, nil
	}
	return wait.PollImmediate(time.Millisecond*50, time.Minute*3, updatedPods)
}

func (d *Deployment) getPodsUpdatedWithContourImage(labelSelector labels.Selector, namespace string) int {
	contourPodImage := d.ContourDeployment.Spec.Template.Spec.Containers[0].Image
	pods := new(v1.PodList)
	labelSelect := &client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     namespace,
	}
	if err := d.client.List(context.TODO(), pods, labelSelect); err != nil {
		return 0
	}
	updatedPods := 0
	for _, pod := range pods.Items {
		updated := false
		for _, container := range pod.Spec.Containers {
			if container.Image == contourPodImage {
				updated = true
			}
		}
		if !updated {
			continue
		}

		for _, cond := range pod.Status.Conditions {
			if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
				updatedPods++
			}
		}
	}
	return updatedPods
}

func (d *Deployment) EnsureRateLimitResources(namespace string, configContents string) error {
	setNamespace := d.Namespace.Name
	if len(namespace) > 0 {
		setNamespace = namespace
	}

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ratelimit-config",
			Namespace: setNamespace,
		},
		Data: map[string]string{
			"ratelimit-config.yaml": configContents,
		},
	}
	if err := d.ensureResource(configMap, new(v1.ConfigMap)); err != nil {
		return err
	}

	deployment := d.RateLimitDeployment.DeepCopy()
	deployment.Namespace = setNamespace
	if os.Getenv("IPV6_CLUSTER") == "true" {
		for i, c := range deployment.Spec.Template.Spec.Containers {
			if c.Name != "ratelimit" {
				continue
			}
			deployment.Spec.Template.Spec.Containers[i].Env = append(
				deployment.Spec.Template.Spec.Containers[i].Env,
				v1.EnvVar{Name: "HOST", Value: "::"},
				v1.EnvVar{Name: "GRPC_HOST", Value: "::"},
				v1.EnvVar{Name: "DEBUG_HOST", Value: "::"},
			)
		}
	}
	if err := d.ensureResource(deployment, new(apps_v1.Deployment)); err != nil {
		return err
	}

	service := d.RateLimitService.DeepCopy()
	service.Namespace = setNamespace
	if err := d.ensureResource(service, new(v1.Service)); err != nil {
		return err
	}

	extSvc := d.RateLimitExtensionService.DeepCopy()
	extSvc.Namespace = setNamespace
	return d.ensureResource(extSvc, new(contour_api_v1alpha1.ExtensionService))
}

func (d *Deployment) EnsureGlobalExternalAuthResources(namespace string) error {
	setNamespace := d.Namespace.Name
	if len(namespace) > 0 {
		setNamespace = namespace
	}

	deployment := d.GlobalExtAuthDeployment.DeepCopy()
	deployment.Namespace = setNamespace
	if err := d.ensureResource(deployment, new(apps_v1.Deployment)); err != nil {
		return err
	}

	service := d.GlobalExtAuthService.DeepCopy()
	service.Namespace = setNamespace
	if err := d.ensureResource(service, new(v1.Service)); err != nil {
		return err
	}

	extSvc := d.GlobalExtAuthExtensionService.DeepCopy()
	extSvc.Namespace = setNamespace

	return d.ensureResource(extSvc, new(contour_api_v1alpha1.ExtensionService))
}

// Convenience method for deploying the pieces of the deployment needed for
// testing Contour running locally, out of cluster.
// Includes:
// - namespace
// - Envoy service account
// - Envoy service
// - ConfigMap with Envoy bootstrap config
// - Envoy DaemonSet modified for local Contour xDS server
func (d *Deployment) EnsureResourcesForLocalContour() error {
	if err := d.EnsureNamespace(); err != nil {
		return err
	}
	if err := d.EnsureEnvoyServiceAccount(); err != nil {
		return err
	}
	if err := d.EnsureEnvoyService(); err != nil {
		return err
	}

	bFile, err := os.CreateTemp("", "bootstrap-*.json")
	if err != nil {
		return err
	}

	// Generate bootstrap config with Contour local address and plaintext
	// client config.
	bootstrapCmd := exec.Command( // nolint:gosec
		d.contourBin,
		"bootstrap",
		bFile.Name(),
		"--xds-address="+d.localContourHost,
		"--xds-port="+d.localContourPort,
		"--xds-resource-version=v3",
		"--admin-address=/admin/admin.sock",
	)

	session, err := gexec.Start(bootstrapCmd, d.cmdOutputWriter, d.cmdOutputWriter)
	if err != nil {
		return err
	}
	session.Wait()

	bootstrapContents, err := io.ReadAll(bFile)
	if err != nil {
		return err
	}
	defer func() {
		bFile.Close()
		os.RemoveAll(bFile.Name())
	}()

	bootstrapConfigMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "envoy-bootstrap",
			Namespace: d.Namespace.Name,
		},
		Data: map[string]string{
			"envoy.json": string(bootstrapContents),
		},
	}
	if err := d.ensureResource(bootstrapConfigMap, new(v1.ConfigMap)); err != nil {
		return err
	}

	if d.EnvoyDeploymentMode == DaemonsetMode {
		d.EnvoyDaemonSet.Spec.Template = d.mutatePodTemplate(d.EnvoyDaemonSet.Spec.Template)
		return d.EnsureEnvoyDaemonSet()
	}

	d.EnvoyDeployment.Spec.Template = d.mutatePodTemplate(d.EnvoyDeployment.Spec.Template)

	// The envoy deployment uses host ports, so can have at most
	// one replica per node, and our cluster only has one worker
	// node, so scale the deployment to 1.
	d.EnvoyDeployment.Spec.Replicas = ref.To(int32(1))

	return d.EnsureEnvoyDeployment()
}

func (d *Deployment) mutatePodTemplate(pts v1.PodTemplateSpec) v1.PodTemplateSpec {
	// Add bootstrap ConfigMap as volume and add envoy admin volume on Envoy pods (also removes cert volume).
	pts.Spec.Volumes = []v1.Volume{{
		Name: "envoy-config",
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: "envoy-bootstrap",
				},
			},
		},
	}, {
		Name: "envoy-admin",
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}}

	// Remove cert volume mount.
	pts.Spec.Containers[1].VolumeMounts = []v1.VolumeMount{
		pts.Spec.Containers[1].VolumeMounts[0], // Config mount
		pts.Spec.Containers[1].VolumeMounts[2], // Admin mount
	}

	pts.Spec.Volumes = append(pts.Spec.Volumes, d.EnvoyExtraVolumes...)
	pts.Spec.Containers[1].VolumeMounts = append(pts.Spec.Containers[1].VolumeMounts, d.EnvoyExtraVolumeMounts...)

	// Remove init container.
	pts.Spec.InitContainers = nil

	// Remove shutdown-manager container.
	pts.Spec.Containers = pts.Spec.Containers[1:]

	// Expose the metrics & admin interfaces via host port to test from outside the kind cluster.
	pts.Spec.Containers[0].Ports = append(pts.Spec.Containers[0].Ports,
		v1.ContainerPort{
			Name:          "metrics",
			ContainerPort: 8002,
			HostPort:      8002,
			Protocol:      v1.ProtocolTCP,
		})

	return pts
}

// DeleteResourcesForLocalContour ensures deletion of all resources
// created in the projectcontour namespace for running a local contour.
// This is done instead of deleting the entire namespace as a performance
// optimization, because deleting non-empty namespaces can take up to a
// couple minutes to complete.
func (d *Deployment) DeleteResourcesForLocalContour() error {
	for _, r := range []client.Object{
		d.ContourConfigMap,
		d.EnvoyService,
		d.EnvoyServiceAccount,
	} {
		if err := d.EnsureDeleted(r); err != nil {
			return err
		}
	}

	switch d.EnvoyDeploymentMode {
	case DaemonsetMode:
		if err := d.EnsureDeleted(d.EnvoyDaemonSet); err != nil {
			return err
		}
	case DeploymentMode:
		if err := d.EnsureDeleted(d.EnvoyDeployment); err != nil {
			return err
		}
	}

	return nil
}

// Starts local contour, applying arguments and marshaling config into config
// file. Returns running Contour command and config file so we can clean them
// up.
func (d *Deployment) StartLocalContour(config *config.Parameters, contourConfiguration *contour_api_v1alpha1.ContourConfiguration, additionalArgs ...string) (*gexec.Session, string, error) {

	var content []byte
	var configReferenceName string
	var contourServeArgs []string
	var err error

	// Look for the ENV variable to tell if this test run should use
	// the ContourConfiguration file or the ContourConfiguration CRD.
	if UsingContourConfigCRD() {
		port, _ := strconv.Atoi(d.localContourPort)

		contourConfiguration.Name = randomString(14)

		// Set the xds server to the defined testing port as well as enable insecure communication.
		contourConfiguration.Spec.XDSServer.Port = port
		contourConfiguration.Spec.XDSServer.Address = listenAllAddress()
		contourConfiguration.Spec.XDSServer.TLS = &contour_api_v1alpha1.TLS{
			Insecure: ref.To(true),
		}

		if err := d.client.Create(context.TODO(), contourConfiguration); err != nil {
			return nil, "", fmt.Errorf("could not create ContourConfiguration: %v", err)
		}

		contourServeArgs = append([]string{
			"serve",
			"--kubeconfig=" + d.kubeConfig,
			"--contour-config-name=" + contourConfiguration.Name,
			"--disable-leader-election",
		}, additionalArgs...)

		configReferenceName = contourConfiguration.Name
	} else {

		configFile, err := os.CreateTemp("", "contour-config-*.yaml")
		if err != nil {
			return nil, "", err
		}
		defer configFile.Close()

		content, err = yaml.Marshal(config)
		if err != nil {
			return nil, "", err
		}
		if err := os.WriteFile(configFile.Name(), content, 0600); err != nil {
			return nil, "", err
		}

		contourServeArgs = append([]string{
			"serve",
			"--xds-address=" + listenAllAddress(),
			"--xds-port=" + d.localContourPort,
			"--stats-address=" + listenAllAddress(),
			"--debug-http-address=" + localAddress(),
			"--http-address=" + listenAllAddress(),
			"--envoy-service-http-address=" + listenAllAddress(),
			"--envoy-service-https-address=" + listenAllAddress(),
			"--health-address=" + listenAllAddress(),
			"--insecure",
			"--kubeconfig=" + d.kubeConfig,
			"--config-path=" + configFile.Name(),
			"--disable-leader-election",
		}, additionalArgs...)

		configReferenceName = configFile.Name()
	}

	session, err := gexec.Start(exec.Command(d.contourBin, contourServeArgs...), d.cmdOutputWriter, d.cmdOutputWriter) // nolint:gosec
	if err != nil {
		return nil, "", err
	}
	return session, configReferenceName, nil
}

func listenAllAddress() string {
	if os.Getenv("IPV6_CLUSTER") == "true" {
		return "::"
	}
	return "0.0.0.0"
}

func localAddress() string {
	if os.Getenv("IPV6_CLUSTER") == "true" {
		return "::1"
	}
	return "127.0.0.1"
}

func (d *Deployment) StopLocalContour(contourCmd *gexec.Session, configFile string) error {

	// Look for the ENV variable to tell if this test run should use
	// the ContourConfiguration file or the ContourConfiguration CRD.
	if useContourConfiguration, variableFound := os.LookupEnv("USE_CONTOUR_CONFIGURATION_CRD"); variableFound && useContourConfiguration == "true" {
		cc := &contour_api_v1alpha1.ContourConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configFile,
				Namespace: "projectcontour",
			},
		}

		if err := d.client.Delete(context.TODO(), cc); err != nil {
			return fmt.Errorf("could not delete ContourConfiguration: %v", err)
		}
	}

	// Default timeout of 1s produces test flakes,
	// a minute should be more than enough to avoid them.
	contourCmd.Terminate().Wait(time.Minute)
	return os.RemoveAll(configFile)
}

// Convenience method for deploying the pieces of the deployment needed for
// testing Contour running in-cluster.
// Includes:
// - namespace
// - Contour service account
// - Envoy service account
// - Contour configmap
// - Certgen service account
// - Certgen role binding
// - Certgen role
// - Certgen job
// - Contour cluster role binding
// - Contour role binding
// - Contour cluster role
// - Contour role
// - Contour service
// - Envoy service
// - Contour deployment (only started if bool passed in is true)
// - Envoy DaemonSet
func (d *Deployment) EnsureResourcesForInclusterContour(startContourDeployment bool) error {
	fmt.Fprintf(d.cmdOutputWriter, "Deploying Contour with image: %s\n", d.contourImage)

	if err := d.EnsureNamespace(); err != nil {
		return err
	}
	if err := d.EnsureContourServiceAccount(); err != nil {
		return err
	}
	if err := d.EnsureEnvoyServiceAccount(); err != nil {
		return err
	}
	if err := d.EnsureContourConfigMap(); err != nil {
		return err
	}
	if err := d.EnsureCertgenServiceAccount(); err != nil {
		return err
	}
	if err := d.EnsureCertgenRoleBinding(); err != nil {
		return err
	}
	if err := d.EnsureCertgenRole(); err != nil {
		return err
	}
	// Update container image.
	if l := len(d.CertgenJob.Spec.Template.Spec.Containers); l != 1 {
		return fmt.Errorf("invalid certgen job containers, expected 1, got %d", l)
	}
	d.CertgenJob.Spec.Template.Spec.Containers[0].Image = d.contourImage
	d.CertgenJob.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	if err := d.EnsureCertgenJob(); err != nil {
		return err
	}
	if err := d.EnsureContourClusterRoleBinding(); err != nil {
		return err
	}
	if err := d.EnsureContourRoleBinding(); err != nil {
		return err
	}
	if err := d.EnsureContourClusterRole(); err != nil {
		return err
	}
	if err := d.EnsureContourRole(); err != nil {
		return err
	}
	if err := d.EnsureContourService(); err != nil {
		return err
	}
	if err := d.EnsureEnvoyService(); err != nil {
		return err
	}
	// Update container image.
	if l := len(d.ContourDeployment.Spec.Template.Spec.Containers); l != 1 {
		return fmt.Errorf("invalid contour deployment containers, expected 1, got %d", l)
	}
	d.ContourDeployment.Spec.Template.Spec.Containers[0].Image = d.contourImage
	d.ContourDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent
	if startContourDeployment {
		if err := d.EnsureContourDeployment(); err != nil {
			return err
		}
	}

	var envoyPodSpec *v1.PodSpec
	if d.EnvoyDeploymentMode == DeploymentMode {
		envoyPodSpec = &d.EnvoyDeployment.Spec.Template.Spec
	} else {
		envoyPodSpec = &d.EnvoyDaemonSet.Spec.Template.Spec
	}

	// Update container image.
	if l := len(envoyPodSpec.InitContainers); l != 1 {
		return fmt.Errorf("invalid envoy %s init containers, expected 1, got %d", d.EnvoyDeploymentMode, l)
	}
	envoyPodSpec.InitContainers[0].Image = d.contourImage
	envoyPodSpec.InitContainers[0].ImagePullPolicy = v1.PullIfNotPresent
	if l := len(envoyPodSpec.Containers); l != 2 {
		return fmt.Errorf("invalid envoy %s containers, expected 2, got %d", d.EnvoyDeploymentMode, l)
	}
	envoyPodSpec.Containers[0].Image = d.contourImage
	envoyPodSpec.Containers[0].ImagePullPolicy = v1.PullIfNotPresent

	if d.EnvoyDeploymentMode == DeploymentMode {
		// The envoy deployment uses host ports, so can have at most
		// one replica per node, and our cluster only has one worker
		// node, so scale the deployment to 1.
		d.EnvoyDeployment.Spec.Replicas = ref.To(int32(1))

		return d.EnsureEnvoyDeployment()
	}

	// Otherwise, we're deploying Envoy as a DaemonSet.
	return d.EnsureEnvoyDaemonSet()
}

// DeleteResourcesForInclusterContour ensures deletion of all resources
// created in the projectcontour namespace for running a contour incluster.
// This is done instead of deleting the entire namespace as a performance
// optimization, because deleting non-empty namespaces can take up to a
// couple minutes to complete.
func (d *Deployment) DeleteResourcesForInclusterContour() error {
	// Also need to delete leader election resources to ensure
	// multiple test runs can be run cleanly.
	leaderElectionLease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "leader-elect",
			Namespace: d.Namespace.Name,
		},
	}

	var envoy client.Object
	if d.EnvoyDeploymentMode == DeploymentMode {
		envoy = d.EnvoyDeployment
	} else {
		envoy = d.EnvoyDaemonSet
	}

	for _, r := range []client.Object{
		envoy,
		d.ContourDeployment,
		leaderElectionLease,
		d.EnvoyService,
		d.ContourService,
		d.ContourRole,
		d.ContourClusterRole,
		d.ContourRoleBinding,
		d.ContourClusterRoleBinding,
		d.CertgenJob,
		d.CertgenRole,
		d.CertgenRoleBinding,
		d.CertgenServiceAccount,
		d.ContourConfigMap,
		d.EnvoyServiceAccount,
		d.ContourServiceAccount,
	} {
		if err := d.EnsureDeleted(r); err != nil {
			return err
		}
	}

	return nil
}

func (d *Deployment) DumpContourLogs() error {
	config, err := clientcmd.BuildConfigFromFlags("", d.kubeConfig)
	if err != nil {
		return err
	}
	coreClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	pods := new(v1.PodList)
	podListOptions := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(d.ContourDeployment.Spec.Selector.MatchLabels),
		Namespace:     d.ContourDeployment.Namespace,
	}
	if err := d.client.List(context.TODO(), pods, podListOptions); err != nil {
		return err
	}

	podLogOptions := &v1.PodLogOptions{
		Container: "contour",
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == v1.PodFailed {
			continue
		}

		fmt.Fprintln(d.cmdOutputWriter, "********** Start of log output for Contour pod:", pod.Name)
		req := coreClient.CoreV1().Pods(d.Namespace.Name).GetLogs(pod.Name, podLogOptions)
		logs, err := req.Stream(context.TODO())
		if err != nil {
			fmt.Fprintln(d.cmdOutputWriter, "Failed to get logs stream:", err)
			continue
		}
		defer logs.Close()
		if _, err := io.Copy(d.cmdOutputWriter, logs); err != nil {
			fmt.Fprintln(d.cmdOutputWriter, "Failed to copy logs:", err)
			continue
		}
		fmt.Fprintln(d.cmdOutputWriter, "********** End of log output for Contour pod:", pod.Name)
	}

	return nil
}

func (d *Deployment) EnsureDeleted(obj client.Object) error {
	// Delete the object; if it already doesn't exist,
	// then we're done.
	err := d.client.Delete(context.Background(), obj)
	if api_errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error deleting resource %T %s/%s: %v", obj, obj.GetNamespace(), obj.GetName(), err)
	}

	// Wait to ensure it's fully deleted.
	if err := wait.PollImmediate(100*time.Millisecond, time.Minute, func() (bool, error) {
		err := d.client.Get(context.Background(), client.ObjectKeyFromObject(obj), obj)
		if api_errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("error waiting for deletion of resource %T %s/%s: %v", obj, obj.GetNamespace(), obj.GetName(), err)
	}

	// Clear out resource version to ensure object can be used again.
	obj.SetResourceVersion("")

	return nil
}

func (d *Deployment) EnvoyResourceAndName() string {
	if d.EnvoyDeploymentMode == DeploymentMode {
		return "deployment/envoy"
	}

	return "daemonset/envoy"
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return ""
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret)
}
