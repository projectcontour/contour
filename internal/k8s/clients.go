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

package k8s

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/discovery"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Clients holds the various API clients required by Contour.
type Clients struct {
	core      *kubernetes.Clientset
	dynamic   dynamic.Interface
	discovery *discovery.DiscoveryClient

	apiResources *APIResources
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

	clients.dynamic, err = dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	clients.discovery, err = discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	// populate the apiResources cache
	clients.apiResources, err = clients.serverResources()
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

type InformerFactory = dynamicinformer.DynamicSharedInformerFactory

// NewInformerFactory returns a new InformerFactory for
// use with any registered Kubernetes API type.
func (c *Clients) NewInformerFactory() InformerFactory {
	return dynamicinformer.NewDynamicSharedInformerFactory(c.dynamic, resyncInterval)
}

// NewInformerFactoryForNamespace returns a new InformerFactory bound to the given namespace.
func (c *Clients) NewInformerFactoryForNamespace(ns string) InformerFactory {
	return dynamicinformer.NewFilteredDynamicSharedInformerFactory(c.dynamic, resyncInterval, ns, nil)
}

// ClientSet returns the Kubernetes Core v1 ClientSet.
func (c *Clients) ClientSet() *kubernetes.Clientset {
	return c.core
}

// DynamicClient returns the dynamic client.
func (c *Clients) DynamicClient() dynamic.Interface {
	return c.dynamic
}

// APIResources holds a map of API, GroupVersionResources, that exist in the Kubernetes cluster
type APIResources struct {
	serverResources map[schema.GroupVersionResource]struct{}
}

// serverResources returns the list of all the resources supported
// by the API server. Note that this method guarantees to populate the
// Group and Version fields in the result.
func (c *Clients) serverResources() (*APIResources, error) {
	_, apiList, err := c.discovery.ServerGroupsAndResources()
	if err != nil {
		return nil, err
	}

	gvrs, err := discovery.GroupVersionResources(apiList)
	return &APIResources{
		serverResources: gvrs,
	}, err
}

// ResourceExists returns true if all of the GroupVersionResources
// passed exists in the cluster.
func (c *Clients) ResourceExists(gvr ...schema.GroupVersionResource) bool {
	for _, r := range gvr {
		if _, ok := c.apiResources.serverResources[r]; !ok {
			return false
		}
	}
	return true
}
