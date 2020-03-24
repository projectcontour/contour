// Copyright © 2020 VMware
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

package k8s

import (
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
)

// Clients holds the various API clients required by Contour.
type Clients struct {
	core         *kubernetes.Clientset
	coordination *coordinationv1.CoordinationV1Client
	dynamic      dynamic.Interface
}

// NewClients returns a new set of the various API clients required
// by Contour using the supplied kubeconfig path, or the cluster
// environment variables if inCluster is true.
func NewClients(kubeconfig string, inCluster bool) (*Clients, error) {
	config, err := newRestConfig(kubeconfig, inCluster)
	if err != nil {
		return nil, err
	}

	var clients Clients
	clients.core, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	clients.coordination, err = coordinationv1.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	clients.dynamic, err = dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &clients, nil
}

func newRestConfig(kubeconfig string, inCluster bool) (*rest.Config, error) {
	if kubeconfig != "" && !inCluster {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

// note: 0 means resync timers are disabled
const resyncInterval time.Duration = 0

// NewInformerFactory returns a new SharedInformerFactory for the core
// Kubernetes API types.
func (c *Clients) NewInformerFactory() informers.SharedInformerFactory {
	return informers.NewSharedInformerFactory(c.core, resyncInterval)
}

// NewInformerFactoryForNamespace returns a new SharedInformerFactory
// for the core Kubernetes API types for the namespace supplied.
func (c *Clients) NewInformerFactoryForNamespace(namespace string) informers.SharedInformerFactory {
	return informers.NewSharedInformerFactoryWithOptions(c.core, resyncInterval, informers.WithNamespace(namespace))
}

// NewDynamicInformerFactory returns a new DynamicSharedInformerFactory for
// use with any registered Kubernetes API type.
func (c *Clients) NewDynamicInformerFactory() dynamicinformer.DynamicSharedInformerFactory {
	return dynamicinformer.NewDynamicSharedInformerFactory(c.dynamic, resyncInterval)
}

// ClientSet returns the Kubernetes Core v1 ClientSet.
func (c *Clients) ClientSet() *kubernetes.Clientset {
	return c.core
}

// CoordinationClient returns the Kubernets Core v1 coordination client.
func (c *Clients) CoordinationClient() *coordinationv1.CoordinationV1Client {
	return c.coordination
}

// DynamicClient returns the Dyanmic client.
func (c *Clients) DynamicClient() dynamic.Interface {
	return c.dynamic
}
