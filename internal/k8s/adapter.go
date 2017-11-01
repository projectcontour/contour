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

// k8s containers adapters to watch k8s api servers.
package k8s

import (
	"time"

	"github.com/heptio/contour/internal/log"

	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// A ServiceCache holds v1.Services.
type ServiceCache interface {

	// AddService adds the Service to the ServiceCache.
	// If the Service is already present in the ServiceCache
	// it is replaced unconditionally.
	AddService(*v1.Service)

	// RemoveService removes the Service from the ServiceCache.
	RemoveService(*v1.Service)
}

// WatchServices creates a SharedInformer configured to populate sc with Services.
func WatchServices(client *kubernetes.Clientset, sc ServiceCache, l log.Logger) cache.SharedInformer {
	lw := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "services", v1.NamespaceAll, fields.Everything())
	sw := cache.NewSharedInformer(lw, new(v1.Service), 30*time.Minute)
	sw.AddEventHandler(&ServiceWatchAdapter{
		ServiceCache: sc,
		Logger:       l.WithPrefix("ServiceWatcherAapter"),
	})
	return sw
}

// A ServiceWatchAdapter implements cache.ResourceEventHandler to
// adapt a cache.SharedInformer to a ServiceCache implementation.
type ServiceWatchAdapter struct {
	ServiceCache
	log.Logger
}

func (swa *ServiceWatchAdapter) OnAdd(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		swa.Errorf("OnAdd expected %T, got %T: %#v", new(v1.Service), obj, obj)
		return
	}
	swa.AddService(svc)
}

func (swa *ServiceWatchAdapter) OnUpdate(_, newObj interface{}) {
	svc, ok := newObj.(*v1.Service)
	if !ok {
		swa.Errorf("OnUpdate expected %T, got %T: %#v", new(v1.Service), newObj, newObj)
		return
	}
	swa.AddService(svc)
}

func (swa *ServiceWatchAdapter) OnDelete(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		swa.Errorf("OnDelete expected %T, got %T: %#v", new(v1.Service), obj, obj)
		return
	}
	swa.RemoveService(svc)
}

// An EndpointsCache holds v1.Endpoints.
type EndpointsCache interface {

	// AddEndpoints adds the Endpoints to the EndpointsCache.
	AddEndpoints(*v1.Endpoints)

	// RemoveEndpoints removes the Endpoints from the EndpointsCache.
	RemoveEndpoints(*v1.Endpoints)
}

// WatchEndpoints creates a SharedInformer configured to populate ec with Endpoints.
func WatchEndpoints(client *kubernetes.Clientset, ec EndpointsCache, l log.Logger) cache.SharedInformer {
	lw := cache.NewListWatchFromClient(client.CoreV1().RESTClient(), "endpoints", v1.NamespaceAll, fields.Everything())
	ew := cache.NewSharedInformer(lw, new(v1.Endpoints), 30*time.Minute)
	ew.AddEventHandler(&EndpointsWatchAdapter{
		EndpointsCache: ec,
		Logger:         l.WithPrefix("EndpointsWatcherAdapter"),
	})
	return ew
}

// An EndpointsWatchAdapter implements cache.ResourceEventHandler to
// adapt a cache.SharedInformer to an EndpointsCache implementation.
type EndpointsWatchAdapter struct {
	EndpointsCache
	log.Logger
}

func (ewa *EndpointsWatchAdapter) OnAdd(obj interface{}) {
	ep, ok := obj.(*v1.Endpoints)
	if !ok {
		ewa.Errorf("OnAdd expected %T, got %T: %#v", new(v1.Endpoints), obj, obj)
		return
	}
	ewa.AddEndpoints(ep)
}

func (ewa *EndpointsWatchAdapter) OnUpdate(_, newObj interface{}) {
	ep, ok := newObj.(*v1.Endpoints)
	if !ok {
		ewa.Errorf("OnUpdate expected %T, got %T: %#v", new(v1.Endpoints), newObj, newObj)
		return
	}
	ewa.AddEndpoints(ep)
}

func (ewa *EndpointsWatchAdapter) OnDelete(obj interface{}) {
	ep, ok := obj.(*v1.Endpoints)
	if !ok {
		ewa.Errorf("OnDelete expected %T, got %T: %#v", new(v1.Endpoints), obj, obj)
		return
	}
	ewa.RemoveEndpoints(ep)
}

// An IngressCache holds v1beta1.Ingress.
type IngressCache interface {

	// AddIngress adds the Ingress to the IngressCache.
	AddIngress(*v1beta1.Ingress)

	// RemoveIngress removes the Ingress from the IngressCache.
	RemoveIngress(*v1beta1.Ingress)
}

// WatchIngress creates a SharedInformer configured to populate ic with Ingresses.
func WatchIngress(client *kubernetes.Clientset, ic IngressCache, l log.Logger) cache.SharedInformer {
	lw := cache.NewListWatchFromClient(client.ExtensionsV1beta1().RESTClient(), "ingresses", v1.NamespaceAll, fields.Everything())
	iw := cache.NewSharedInformer(lw, new(v1beta1.Ingress), 30*time.Minute)
	iw.AddEventHandler(&IngressWatchAdapter{
		IngressCache: ic,
		Logger:       l.WithPrefix("IngressWatchAdapter"),
	})
	return iw
}

// An IngressWatchAdapter implements cache.ResourceEventHandler to
// adapt a cache.SharedInformer to an IngressCache implementation.
type IngressWatchAdapter struct {
	IngressCache
	log.Logger
}

func (iwa *IngressWatchAdapter) OnAdd(obj interface{}) {
	i, ok := obj.(*v1beta1.Ingress)
	if !ok {
		iwa.Errorf("OnAdd expected %T, got %T: %#v", new(v1beta1.Ingress), obj, obj)
		return
	}
	iwa.AddIngress(i)
}

func (iwa *IngressWatchAdapter) OnUpdate(_, newObj interface{}) {
	i, ok := newObj.(*v1beta1.Ingress)
	if !ok {
		iwa.Errorf("OnUpdate expected %T, got %T: %#v", new(v1beta1.Ingress), newObj, newObj)
		return
	}
	iwa.AddIngress(i)
}

func (iwa *IngressWatchAdapter) OnDelete(obj interface{}) {
	i, ok := obj.(*v1beta1.Ingress)
	if !ok {
		iwa.Errorf("OnDelete expected %T, got %T: %#v", new(v1beta1.Ingress), obj, obj)
		return
	}
	iwa.RemoveIngress(i)
}
