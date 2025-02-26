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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	apps_v1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	rbac_v1 "k8s.io/api/rbac/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	apimachinery_util_yaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Provisioner struct {
	// k8s client
	client client.Client

	// Command output is written to this writer.
	cmdOutputWriter io.Writer

	contourImage string

	Namespace                     *core_v1.Namespace
	ServiceAccount                *core_v1.ServiceAccount
	ProvisionerClusterRole        *rbac_v1.ClusterRole
	LeaderElectionRole            *rbac_v1.Role
	LeaderElectionRoleBinding     *rbac_v1.RoleBinding
	ProvisionerClusterRoleBinding *rbac_v1.ClusterRoleBinding
	Deployment                    *apps_v1.Deployment
}

// UnmarshalResources unmarshals resources from rendered Contour manifest in
// order.
// Note: This will need to be updated if any new resources are added to the
// rendered deployment manifest.
func (p *Provisioner) UnmarshalResources() error {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return errors.New("could not get path to this source file (test/e2e/provisioner.go)")
	}

	files, err := os.ReadDir(filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "gateway-provisioner"))
	if err != nil {
		return err
	}

	var yaml []byte
	for _, fi := range files {
		contents, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "examples", "gateway-provisioner", fi.Name()))
		if err != nil {
			return err
		}

		yaml = append(yaml, contents...)
	}

	decoder := apimachinery_util_yaml.NewYAMLToJSONDecoder(bytes.NewBuffer(yaml))

	p.Namespace = new(core_v1.Namespace)
	p.ServiceAccount = new(core_v1.ServiceAccount)
	p.ProvisionerClusterRole = new(rbac_v1.ClusterRole)
	p.LeaderElectionRole = new(rbac_v1.Role)
	p.LeaderElectionRoleBinding = new(rbac_v1.RoleBinding)
	p.ProvisionerClusterRoleBinding = new(rbac_v1.ClusterRoleBinding)
	p.Deployment = new(apps_v1.Deployment)

	objects := []any{
		p.Namespace,
		p.ServiceAccount,
		p.ProvisionerClusterRole,
		p.LeaderElectionRole,
		p.LeaderElectionRoleBinding,
		p.ProvisionerClusterRoleBinding,
		p.Deployment,
	}
	for _, o := range objects {
		if err := decoder.Decode(o); err != nil {
			return err
		}
	}

	return nil
}

// Common case of updating object if exists, create otherwise.
func (p *Provisioner) ensureResource(newResource, existingResource client.Object) error {
	if err := p.client.Get(context.TODO(), client.ObjectKeyFromObject(newResource), existingResource); err != nil {
		if api_errors.IsNotFound(err) {
			return p.client.Create(context.TODO(), newResource)
		}
		return err
	}
	newResource.SetResourceVersion(existingResource.GetResourceVersion())
	// If a core_v1.Service, pass along existing cluster IP and healthcheck node port.
	if newS, ok := newResource.(*core_v1.Service); ok {
		existingS := existingResource.(*core_v1.Service)
		newS.Spec.ClusterIP = existingS.Spec.ClusterIP
		newS.Spec.ClusterIPs = existingS.Spec.ClusterIPs
		newS.Spec.HealthCheckNodePort = existingS.Spec.HealthCheckNodePort
	}
	return p.client.Update(context.TODO(), newResource)
}

// Convenience method for deploying the pieces of the deployment needed for
// testing Contour running in-cluster.
func (p *Provisioner) EnsureResourcesForInclusterProvisioner() error {
	fmt.Fprintf(p.cmdOutputWriter, "Deploying gateway provisioner with image: %s\n", p.contourImage)

	type resource struct {
		new, existing client.Object
	}

	resources := []resource{
		{new: p.Namespace, existing: new(core_v1.Namespace)},
		{new: p.ServiceAccount, existing: new(core_v1.ServiceAccount)},
		{new: p.ProvisionerClusterRole, existing: new(rbac_v1.ClusterRole)},
		{new: p.LeaderElectionRole, existing: new(rbac_v1.Role)},
		{new: p.LeaderElectionRoleBinding, existing: new(rbac_v1.RoleBinding)},
		{new: p.ProvisionerClusterRoleBinding, existing: new(rbac_v1.ClusterRoleBinding)},
		{new: p.Deployment, existing: new(apps_v1.Deployment)},
	}

	if l := len(p.Deployment.Spec.Template.Spec.Containers); l != 1 {
		return fmt.Errorf("invalid gateway provisioner deployment containers, expected 1, got %d", l)
	}

	p.Deployment.Spec.Template.Spec.Containers[0].Image = p.contourImage
	p.Deployment.Spec.Template.Spec.Containers[0].ImagePullPolicy = core_v1.PullIfNotPresent

	// Set the --contour-image flag to the CI image

	idx := -1
	for i, arg := range p.Deployment.Spec.Template.Spec.Containers[0].Args {
		if strings.Contains(arg, "--contour-image") {
			idx = i
			break
		}
	}

	if idx >= 0 {
		p.Deployment.Spec.Template.Spec.Containers[0].Args[idx] = "--contour-image=" + p.contourImage
	} else {
		p.Deployment.Spec.Template.Spec.Containers[0].Args = append(p.Deployment.Spec.Template.Spec.Containers[0].Args, "--contour-image="+p.contourImage)
	}

	for _, resource := range resources {
		if err := p.ensureResource(resource.new, resource.existing); err != nil {
			return err
		}
	}

	return nil
}

// DeleteResourcesForInclusterContour ensures deletion of all resources
// created in the projectcontour namespace for running a contour incluster.
// This is done instead of deleting the entire namespace as a performance
// optimization, because deleting non-empty namespaces can take up to a
// couple minutes to complete.
func (p *Provisioner) DeleteResourcesForInclusterProvisioner() error {
	for _, r := range []client.Object{
		p.Deployment,
		p.ServiceAccount,
		p.ProvisionerClusterRole,
		p.LeaderElectionRole,
		p.LeaderElectionRoleBinding,
		p.ProvisionerClusterRoleBinding,
		p.Namespace,
	} {
		if err := p.EnsureDeleted(r); err != nil {
			return err
		}
	}

	return nil
}

func (p *Provisioner) EnsureDeleted(obj client.Object) error {
	// Delete the object; if it already doesn't exist,
	// then we're done.
	err := p.client.Delete(context.Background(), obj)
	if api_errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("error deleting resource %T %s/%s: %v", obj, obj.GetNamespace(), obj.GetName(), err)
	}

	// Wait to ensure it's fully deleted.
	if err := wait.PollUntilContextTimeout(context.Background(), 100*time.Millisecond, time.Minute, true, func(ctx context.Context) (bool, error) {
		err := p.client.Get(ctx, client.ObjectKeyFromObject(obj), obj)
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
