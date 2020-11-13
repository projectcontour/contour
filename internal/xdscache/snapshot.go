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
	"math"
	"reflect"
	"strconv"
	"sync"

	envoy_types "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	resource "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/sirupsen/logrus"
)

type Snapshotter interface {
	Generate(version string, resources map[envoy_types.ResponseType][]envoy_types.Resource) error
}

// SnapshotHandler implements the xDS snapshot cache
// by responding to the OnChange() event causing a new
// snapshot to be created.
type SnapshotHandler struct {
	// resources holds the cache of xDS contents.
	resources map[envoy_types.ResponseType]ResourceCache

	// snapshotVersion holds the current version of the snapshot.
	snapshotVersion int64

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
func (s *SnapshotHandler) OnChange(root *dag.DAG) {
	s.generateNewSnapshot()
}

// generateNewSnapshot creates a new snapshot against
// the Contour XDS caches.
func (s *SnapshotHandler) generateNewSnapshot() {
	// Generate new snapshot version.
	version := s.newSnapshotVersion()

	resources := map[envoy_types.ResponseType][]envoy_types.Resource{
		envoy_types.Endpoint: asResources(s.resources[envoy_types.Endpoint].Contents()),
		envoy_types.Cluster:  asResources(s.resources[envoy_types.Cluster].Contents()),
		envoy_types.Route:    asResources(s.resources[envoy_types.Route].Contents()),
		envoy_types.Listener: asResources(s.resources[envoy_types.Listener].Contents()),
		envoy_types.Secret:   asResources(s.resources[envoy_types.Secret].Contents()),
	}

	s.snapLock.Lock()
	defer s.snapLock.Unlock()

	for _, snap := range s.snapshotters {
		if err := snap.Generate(version, resources); err != nil {
			s.Errorf("failed to generate snapshot version %q: %s", version, err)
		}
	}
}

// newSnapshotVersion increments the current snapshotVersion
// and returns as a string.
func (s *SnapshotHandler) newSnapshotVersion() string {

	// Reset the snapshotVersion if it ever hits max size.
	if s.snapshotVersion == math.MaxInt64 {
		s.snapshotVersion = 0
	}

	// Increment the snapshot version & return as string.
	s.snapshotVersion++
	return strconv.FormatInt(s.snapshotVersion, 10)
}

// asResources casts the given slice of values (that implement the envoy_types.Resource
// interface) to a slice of envoy_types.Resource. If the length of the slice is 0, it
// returns nil.
func asResources(messages interface{}) []envoy_types.Resource {
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
func parseResources(resources []ResourceCache) map[envoy_types.ResponseType]ResourceCache {
	resourceMap := make(map[envoy_types.ResponseType]ResourceCache, len(resources))

	for _, r := range resources {
		switch r.TypeURL() {
		case resource.ClusterType:
			resourceMap[envoy_types.Cluster] = r
		case resource.RouteType:
			resourceMap[envoy_types.Route] = r
		case resource.ListenerType:
			resourceMap[envoy_types.Listener] = r
		case resource.SecretType:
			resourceMap[envoy_types.Secret] = r
		case resource.EndpointType:
			resourceMap[envoy_types.Endpoint] = r
		}
	}
	return resourceMap
}
