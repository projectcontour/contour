// Copyright © 2017 Heptio
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

// Package k8s containers adapters to watch k8s api servers.
package k8s

import (
	"time"

	"github.com/heptio/contour/internal/log"
	"github.com/heptio/contour/internal/workgroup"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// WatchServices creates a SharedInformer for v1.Services and registers it with g.
func WatchServices(g *workgroup.Group, client *kubernetes.Clientset, l log.Logger, rs ...cache.ResourceEventHandler) {
	watch(g, client.CoreV1().RESTClient(), l, "services", new(v1.Service), rs...)
}

// WatchEndpoints creates a SharedInformer for v1.Endpoints and registers it with g.
func WatchEndpoints(g *workgroup.Group, client *kubernetes.Clientset, l log.Logger, rs ...cache.ResourceEventHandler) {
	watch(g, client.CoreV1().RESTClient(), l, "endpoints", new(v1.Endpoints), rs...)
}

// WatchIngress creates a SharedInformer for v1beta1.Ingress and registers it with g.
func WatchIngress(g *workgroup.Group, client *kubernetes.Clientset, l log.Logger, rs ...cache.ResourceEventHandler) {
	watch(g, client.ExtensionsV1beta1().RESTClient(), l, "ingresses", new(v1beta1.Ingress), rs...)
}

func watch(g *workgroup.Group, c cache.Getter, l log.Logger, resource string, objType runtime.Object, rs ...cache.ResourceEventHandler) {
	lw := cache.NewListWatchFromClient(c, resource, v1.NamespaceAll, fields.Everything())
	sw := cache.NewSharedInformer(lw, objType, 30*time.Minute)
	for _, r := range rs {
		sw.AddEventHandler(r)
	}
	g.Add(func(stop <-chan struct{}) {
		l := l.WithPrefix("watch(" + resource + ")")
		l.Infof("started")
		defer l.Infof("stopped")
		sw.Run(stop)
	})
}
