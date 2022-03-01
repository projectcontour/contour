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

package contour

import (
	"context"
	"fmt"

	operatorv1alpha1 "github.com/projectcontour/contour-operator/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Config is the configuration of a Contour.
type Config struct {
	Name                      string
	Namespace                 string
	SpecNs                    string
	RemoveNs                  bool
	Replicas                  int32
	NetworkType               operatorv1alpha1.NetworkPublishingType
	NodePorts                 []operatorv1alpha1.NodePort
	GatewayControllerName     *string
	EnableExternalNameService *bool
}

// New makes a Contour object using the provided ns/name for the object's
// namespace/name, pubType for the network publishing type of Envoy, and
// Envoy container ports 8080/8443.
func New(cfg Config) *operatorv1alpha1.Contour {
	cntr := &operatorv1alpha1.Contour{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfg.Namespace,
			Name:      cfg.Name,
		},
		Spec: operatorv1alpha1.ContourSpec{
			Replicas: cfg.Replicas,
			Namespace: operatorv1alpha1.NamespaceSpec{
				Name:             cfg.SpecNs,
				RemoveOnDeletion: cfg.RemoveNs,
			},
			NetworkPublishing: operatorv1alpha1.NetworkPublishing{
				Envoy: operatorv1alpha1.EnvoyNetworkPublishing{
					Type: cfg.NetworkType,
					ContainerPorts: []operatorv1alpha1.ContainerPort{
						{
							Name:       "http",
							PortNumber: int32(8080),
						},
						{
							Name:       "https",
							PortNumber: int32(8443),
						},
					},
				},
			},
		},
	}
	if cfg.NetworkType == operatorv1alpha1.NodePortServicePublishingType && len(cfg.NodePorts) > 0 {
		cntr.Spec.NetworkPublishing.Envoy.NodePorts = cfg.NodePorts
	}
	if cfg.GatewayControllerName != nil {
		cntr.Spec.GatewayControllerName = cfg.GatewayControllerName
	}
	if cfg.EnableExternalNameService != nil {
		cntr.Spec.EnableExternalNameService = cfg.EnableExternalNameService
	}
	return cntr
}

// CurrentContour returns the current Contour for the provided ns/name.
func CurrentContour(ctx context.Context, cli client.Client, ns, name string) (*operatorv1alpha1.Contour, error) {
	cntr := &operatorv1alpha1.Contour{}
	key := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}
	if err := cli.Get(ctx, key, cntr); err != nil {
		return nil, err
	}
	return cntr, nil
}

// OtherContoursExist lists Contour objects in all namespaces, returning the list
// and true if any exist other than contour.
func OtherContoursExist(ctx context.Context, cli client.Client, contour *operatorv1alpha1.Contour) (bool, *operatorv1alpha1.ContourList, error) {
	contours := &operatorv1alpha1.ContourList{}
	if err := cli.List(ctx, contours); err != nil {
		return false, nil, fmt.Errorf("failed to list contours: %w", err)
	}
	if len(contours.Items) == 0 || len(contours.Items) == 1 && contours.Items[0].Name == contour.Name {
		return false, nil, nil
	}
	return true, contours, nil
}

// OtherContoursExistInSpecNs lists Contour objects in the same spec.namespace.name as contour,
// returning true if any exist.
func OtherContoursExistInSpecNs(ctx context.Context, cli client.Client, contour *operatorv1alpha1.Contour) (bool, error) {
	exist, contours, err := OtherContoursExist(ctx, cli, contour)
	if err != nil {
		return false, err
	}
	if exist {
		for _, c := range contours.Items {
			if c.Name == contour.Name && c.Namespace == contour.Namespace {
				// Skip the contour from the list that matches the provided contour.
				continue
			}
			if c.Spec.Namespace.Name == contour.Spec.Namespace.Name {
				return true, nil
			}
		}
	}
	return false, nil
}

// OwningSelector returns a label selector using "contour.operator.projectcontour.io/owning-contour-name"
// and "contour.operator.projectcontour.io/owning-contour-namespace" labels.
func OwningSelector(contour *operatorv1alpha1.Contour) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			operatorv1alpha1.OwningContourNameLabel: contour.Name,
			operatorv1alpha1.OwningContourNsLabel:   contour.Namespace,
		},
	}
}

// OwnerLabels returns owner labels for the provided contour.
func OwnerLabels(contour *operatorv1alpha1.Contour) map[string]string {
	return map[string]string{
		operatorv1alpha1.OwningContourNameLabel: contour.Name,
		operatorv1alpha1.OwningContourNsLabel:   contour.Namespace,
	}
}

// MakeNodePorts returns a nodeport slice using the ports key as the nodeport name
// and the ports value as the nodeport number.
func MakeNodePorts(ports map[string]int) []operatorv1alpha1.NodePort {
	nodePorts := []operatorv1alpha1.NodePort{}
	for k, v := range ports {
		p := operatorv1alpha1.NodePort{
			Name:       k,
			PortNumber: pointer.Int32Ptr(int32(v)),
		}
		nodePorts = append(nodePorts, p)
	}
	return nodePorts
}
