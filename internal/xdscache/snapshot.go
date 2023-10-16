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

package xdscache

import (
	"reflect"
	"sync"

	envoy_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	envoy_resource_v3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/google/uuid"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/sirupsen/logrus"
)

type Snapshotter interface {
	Generate(version string, resources map[envoy_resource_v3.Type][]envoy_types.Resource) error
}

// SnapshotHandler implements the xDS snapshot cache
// by responding to the OnChange() event causing a new
// snapshot to be created.
type SnapshotHandler struct {
	// resources holds the cache of xDS contents.
	resources map[envoy_resource_v3.Type]ResourceCache

	snapshotters []Snapshotter
	snapLock     sync.Mutex

	logrus.FieldLogger
}

// NewSnapshotHandler returns an instance of SnapshotHandler.
func NewSnapshotHandler(resources []ResourceCache, logger logrus.FieldLogger) *SnapshotHandler {
	return &SnapshotHandler{
		resources:   parseResources(resources),
		FieldLogger: logger,
	}
}

func (s *SnapshotHandler) AddSnapshotter(snap Snapshotter) {
	s.snapLock.Lock()
	defer s.snapLock.Unlock()

	s.snapshotters = append(s.snapshotters, snap)
}

// Refresh is called when the EndpointsTranslator updates values
// in its cache.
func (s *SnapshotHandler) Refresh() {
	s.generateNewSnapshot()
}

// OnChange is called when the DAG is rebuilt and a new snapshot is needed.
func (s *SnapshotHandler) OnChange(_ *dag.DAG) {
	s.generateNewSnapshot()
}

// generateNewSnapshot creates a new snapshot against
// the Contour XDS caches.
func (s *SnapshotHandler) generateNewSnapshot() {
	// Generate new snapshot version.
	version := uuid.NewString()

	// Convert caches to envoy xDS Resources.
	resources := map[envoy_resource_v3.Type][]envoy_types.Resource{
		envoy_resource_v3.EndpointType: asResources(s.resources[envoy_resource_v3.EndpointType].Contents()),
		envoy_resource_v3.ClusterType:  asResources(s.resources[envoy_resource_v3.ClusterType].Contents()),
		envoy_resource_v3.RouteType:    asResources(s.resources[envoy_resource_v3.RouteType].Contents()),
		envoy_resource_v3.ListenerType: asResources(s.resources[envoy_resource_v3.ListenerType].Contents()),
		envoy_resource_v3.SecretType:   asResources(s.resources[envoy_resource_v3.SecretType].Contents()),
		envoy_resource_v3.RuntimeType:  asResources(s.resources[envoy_resource_v3.RuntimeType].Contents()),
	}

	s.snapLock.Lock()
	defer s.snapLock.Unlock()

	for _, snap := range s.snapshotters {
		if err := snap.Generate(version, resources); err != nil {
			s.Errorf("failed to generate snapshot version %q: %s", version, err)
		}
	}
}

// asResources casts the given slice of values (that implement the envoy_types.Resource
// interface) to a slice of envoy_types.Resource. If the length of the slice is 0, it
// returns nil.
func asResources(messages any) []envoy_types.Resource {
	v := reflect.ValueOf(messages)
	if v.Len() == 0 {
		return nil
	}

	protos := make([]envoy_types.Resource, v.Len())

	for i := range protos {
		protos[i] = v.Index(i).Interface().(envoy_types.Resource)
	}

	return protos
}

// parseResources converts an []ResourceCache to a map[envoy_types.ResponseType]ResourceCache
// for faster indexing when creating new snapshots.
func parseResources(resources []ResourceCache) map[envoy_resource_v3.Type]ResourceCache {
	resourceMap := make(map[envoy_resource_v3.Type]ResourceCache, len(resources))

	for _, r := range resources {
		resourceMap[r.TypeURL()] = r
	}
	return resourceMap
}
