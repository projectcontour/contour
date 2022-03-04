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
	"fmt"

	"github.com/go-logr/logr"
	"github.com/projectcontour/contour/internal/provisioner/model"
	"github.com/projectcontour/contour/internal/provisioner/objects"
	"github.com/projectcontour/contour/internal/provisioner/objects/configmap"
	"github.com/projectcontour/contour/internal/provisioner/objects/daemonset"
	"github.com/projectcontour/contour/internal/provisioner/objects/deployment"
	"github.com/projectcontour/contour/internal/provisioner/objects/job"
	"github.com/projectcontour/contour/internal/provisioner/objects/service"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ensurer struct {
	log          logr.Logger
	client       client.Client
	contourImage string
	envoyImage   string
}

func (e *ensurer) ensureContour(ctx context.Context, contour *model.Contour) []error {
	var errs []error

	handleResult := func(resource string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to ensure %s for contour %s/%s: %w", resource, contour.Namespace, contour.Name, err))
		} else {
			e.log.Info(fmt.Sprintf("ensured %s for contour", resource), "namespace", contour.Namespace, "name", contour.Name)
		}
	}

	handleResult("rbac", objects.EnsureRBAC(ctx, e.client, contour))

	if len(errs) > 0 {
		return errs
	}

	handleResult("configmap", configmap.EnsureConfigMap(ctx, e.client, contour))
	handleResult("job", job.EnsureJob(ctx, e.client, contour, e.contourImage))
	handleResult("deployment", deployment.EnsureDeployment(ctx, e.client, contour, e.contourImage))
	handleResult("daemonset", daemonset.EnsureDaemonSet(ctx, e.client, contour, e.contourImage, e.envoyImage))
	handleResult("contour service", service.EnsureContourService(ctx, e.client, contour))

	switch contour.Spec.NetworkPublishing.Envoy.Type {
	case model.LoadBalancerServicePublishingType, model.NodePortServicePublishingType, model.ClusterIPServicePublishingType:
		handleResult("envoy service", service.EnsureEnvoyService(ctx, e.client, contour))
	}

	return errs
}

func (e *ensurer) ensureContourDeleted(ctx context.Context, contour *model.Contour) []error {
	var errs []error

	handleResult := func(resource string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete %s for contour %s/%s: %w", resource, contour.Namespace, contour.Name, err))
		} else {
			e.log.Info(fmt.Sprintf("deleted %s for contour", resource), "namespace", contour.Namespace, "name", contour.Name)
		}
	}

	handleResult("envoy service", service.EnsureEnvoyServiceDeleted(ctx, e.client, contour))
	handleResult("service", service.EnsureContourServiceDeleted(ctx, e.client, contour))
	handleResult("daemonset", daemonset.EnsureDaemonSetDeleted(ctx, e.client, contour))
	handleResult("deployment", deployment.EnsureDeploymentDeleted(ctx, e.client, contour))
	handleResult("job", job.EnsureJobDeleted(ctx, e.client, contour, e.contourImage))
	handleResult("configmap", configmap.EnsureConfigMapDeleted(ctx, e.client, contour))
	handleResult("rbac", objects.EnsureRBACDeleted(ctx, e.client, contour))

	return errs
}
