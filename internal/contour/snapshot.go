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
	"fmt"
	"math"
	"sync"

	envoy_api_v2_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	xds "github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	"github.com/sirupsen/logrus"
)

// SnapshotHandler implements the xDS snapshot cache
type SnapshotHandler struct {
	mu sync.Mutex

	// xDSResources is a local cache representing the current state
	// of any xDS resource Contour is managing.
	xDSResources map[xds.ResponseType][]xds.Resource

	// SnapshotVersion holds the current version of the snapshot
	snapshotVersion int64

	// snapshotCache is a snapshot-based cache that maintains a single versioned
	// snapshot of responses for xDS resources that Contour manages
	snapshotCache cache.SnapshotCache

	logrus.FieldLogger
}

// NewSnapshotHandler returns an instance of SnapShotHandler
func NewSnapshotHandler(c cache.SnapshotCache, logger logrus.FieldLogger) *SnapshotHandler {
	return &SnapshotHandler{
		snapshotCache: c,
		FieldLogger:   logger,
		xDSResources:  make(map[xds.ResponseType][]xds.Resource, 5),
	}
}

// ConstantHash is a specialized node ID hasher used to allow
// any instance of Envoy to connect to Contour regardless of the
// service-node flag configured on Envoy.
type ConstantHash string

func (c ConstantHash) ID(*envoy_api_v2_core.Node) string {
	return string(c)
}

func (c ConstantHash) String() string {
	return string(c)
}

var _ cache.NodeHash = ConstantHash("")
var DefaultHash = ConstantHash("contour")

// UpdateSnapshot is called when any cache changes and
// Envoy should be updated with a new configuration.
//
// It does not take into account a specific cache changing.
// When called, all xDS resources are updated with a new version.
// Envoy sees this as a noop, but could be improved in future refactorings.
func (s *SnapshotHandler) UpdateSnapshot(xDSResources map[xds.ResponseType][]xds.Resource) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate new snapshot version.
	snapshotVersion := s.getNewSnapshotVersion()

	// Cache the new resources passed in for use in setting
	// a new snapshot.
	s.cacheResources(xDSResources)

	// Create an snapshot with all xDS resources.
	snapshot := cache.NewSnapshot(snapshotVersion,
		s.xDSResources[xds.Endpoint],
		s.xDSResources[xds.Cluster],
		s.xDSResources[xds.Route],
		s.xDSResources[xds.Listener],
		nil)

	// Update the Secrets xDS resource manually until a new version of go-control-plane is released.
	// ref: https://github.com/envoyproxy/go-control-plane/pull/314
	snapshot.Resources[xds.Secret] = cache.NewResources(snapshotVersion, s.xDSResources[xds.Secret])

	if err := s.snapshotCache.SetSnapshot(DefaultHash.String(), snapshot); err != nil {
		s.Errorf("UpdateSnapshot: Error setting snapshot: %q", err)
	}
}

func (s *SnapshotHandler) cacheResources(xDSResources map[xds.ResponseType][]xds.Resource) {
	// Store the value passed in to the local cache so
	// that it can be used when creating new snapshots.
	for key, value := range xDSResources {
		switch key {
		case xds.Cluster:
			s.xDSResources[xds.Cluster] = value
		case xds.Route:
			s.xDSResources[xds.Route] = value
		case xds.Listener:
			s.xDSResources[xds.Listener] = value
		case xds.Secret:
			s.xDSResources[xds.Secret] = value
		case xds.Endpoint:
			s.xDSResources[xds.Endpoint] = value
		default:
			s.Errorf("UpdateSnapshot: invalid xDS Resource type %q passed when updating local cache.", key)
		}
	}
}

func (s *SnapshotHandler) getNewSnapshotVersion() string {

	// Reset the snapshotVersion if it ever hits max size.
	if s.snapshotVersion == math.MaxInt64 {
		s.snapshotVersion = 0
	}

	// Increment the snapshot version & return as string.
	s.snapshotVersion++
	return fmt.Sprintf("%d", s.snapshotVersion)
}
