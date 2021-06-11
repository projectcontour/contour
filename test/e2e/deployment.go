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

package e2e

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	apps_v1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	apiextensions_v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Deployment struct {
	client                    client.Client
	Namespace                 *v1.Namespace
	ContourServiceAccount     *v1.ServiceAccount
	EnvoyServiceAccount       *v1.ServiceAccount
	ContourConfigMap          *v1.ConfigMap
	ExtensionServiceCRD       *apiextensions_v1.CustomResourceDefinition
	HTTPProxyCRD              *apiextensions_v1.CustomResourceDefinition
	TLSCertDelegationCRD      *apiextensions_v1.CustomResourceDefinition
	CertgenServiceAccount     *v1.ServiceAccount
	ContourRoleBinding        *rbac_v1.RoleBinding
	CertgenRole               *rbac_v1.Role
	CertgenJob                *batch_v1.Job
	ContourClusterRoleBinding *rbac_v1.ClusterRoleBinding
	ContourClusterRole        *rbac_v1.ClusterRole
	ContourService            *v1.Service
	EnvoyService              *v1.Service
	ContourDeployment         *apps_v1.Deployment
	EnvoyDaemonSet            *apps_v1.DaemonSet
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
	renderedManifestPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "render", "contour.yaml")
	file, err := os.Open(renderedManifestPath)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := yaml.NewYAMLToJSONDecoder(file)

	// Discard empty document.
	if err := decoder.Decode(new(struct{})); err != nil {
		return err
	}

	d.Namespace = new(v1.Namespace)
	d.ContourServiceAccount = new(v1.ServiceAccount)
	d.EnvoyServiceAccount = new(v1.ServiceAccount)
	d.ContourConfigMap = new(v1.ConfigMap)
	d.ExtensionServiceCRD = new(apiextensions_v1.CustomResourceDefinition)
	d.HTTPProxyCRD = new(apiextensions_v1.CustomResourceDefinition)
	d.TLSCertDelegationCRD = new(apiextensions_v1.CustomResourceDefinition)
	d.CertgenServiceAccount = new(v1.ServiceAccount)
	d.ContourRoleBinding = new(rbac_v1.RoleBinding)
	d.CertgenRole = new(rbac_v1.Role)
	d.CertgenJob = new(batch_v1.Job)
	d.ContourClusterRoleBinding = new(rbac_v1.ClusterRoleBinding)
	d.ContourClusterRole = new(rbac_v1.ClusterRole)
	d.ContourService = new(v1.Service)
	d.EnvoyService = new(v1.Service)
	d.ContourDeployment = new(apps_v1.Deployment)
	d.EnvoyDaemonSet = new(apps_v1.DaemonSet)
	objects := []interface{}{
		d.Namespace,
		d.ContourServiceAccount,
		d.EnvoyServiceAccount,
		d.ContourConfigMap,
		d.ExtensionServiceCRD,
		d.HTTPProxyCRD,
		d.TLSCertDelegationCRD,
		d.CertgenServiceAccount,
		d.ContourRoleBinding,
		d.CertgenRole,
		d.CertgenJob,
		d.ContourClusterRoleBinding,
		d.ContourClusterRole,
		d.ContourService,
		d.EnvoyService,
		d.ContourDeployment,
		d.EnvoyDaemonSet,
	}
	for _, o := range objects {
		if err := decoder.Decode(o); err != nil {
			return err
		}
	}

	return nil
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

func (d *Deployment) EnsureExtensionServiceCRD() error {
	return d.ensureResource(d.ExtensionServiceCRD, new(apiextensions_v1.CustomResourceDefinition))
}

func (d *Deployment) EnsureHTTPProxyCRD() error {
	return d.ensureResource(d.HTTPProxyCRD, new(apiextensions_v1.CustomResourceDefinition))
}

func (d *Deployment) EnsureTLSCertDelegationCRD() error {
	return d.ensureResource(d.TLSCertDelegationCRD, new(apiextensions_v1.CustomResourceDefinition))
}

func (d *Deployment) EnsureCertgenServiceAccount() error {
	return d.ensureResource(d.CertgenServiceAccount, new(v1.ServiceAccount))
}

func (d *Deployment) EnsureContourRoleBinding() error {
	return d.ensureResource(d.ContourRoleBinding, new(rbac_v1.RoleBinding))
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

func (d *Deployment) EnsureContourClusterRole() error {
	return d.ensureResource(d.ContourClusterRole, new(rbac_v1.ClusterRole))
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
	contourPodImage := d.ContourDeployment.Spec.Template.Spec.Containers[0].Image
	updatedPods := func() (bool, error) {
		pods := new(v1.PodList)
		labelSelectAppContour := &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(d.ContourDeployment.Spec.Selector.MatchLabels),
			Namespace:     d.ContourDeployment.Namespace,
		}
		if err := d.client.List(context.TODO(), pods, labelSelectAppContour); err != nil {
			return false, err
		}
		if pods == nil {
			return false, errors.New("failed to fetch Contour Deployment pods")
		}

		updatedPods := 0
		for _, pod := range pods.Items {
			if len(pod.Spec.Containers) != 1 {
				return false, errors.New("invalid Contour Deployment pod containers")
			}
			if pod.Spec.Containers[0].Image != contourPodImage {
				continue
			}
			for _, cond := range pod.Status.Conditions {
				if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
					updatedPods++
				}
			}
		}
		return updatedPods == int(*d.ContourDeployment.Spec.Replicas), nil
	}
	return wait.PollImmediate(time.Millisecond*50, time.Minute, updatedPods)
}

func (d *Deployment) EnsureEnvoyDaemonSet() error {
	return d.ensureResource(d.EnvoyDaemonSet, new(apps_v1.DaemonSet))
}

func (d *Deployment) WaitForEnvoyDaemonSetUpdated() error {
	daemonSetUpdated := func() (bool, error) {
		tempDS := new(apps_v1.DaemonSet)
		if err := d.client.Get(context.TODO(), client.ObjectKeyFromObject(d.EnvoyDaemonSet), tempDS); err != nil {
			return false, err
		}
		// This might work for now while we have only one worker node, but
		// if we expand to more, we will have to rethink this.
		return tempDS.Status.NumberReady > 0, nil
	}
	return wait.PollImmediate(time.Millisecond*50, time.Minute*3, daemonSetUpdated)
}
