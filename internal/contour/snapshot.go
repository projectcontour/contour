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
	"github.com/envoyproxy/go-control-plane/pkg/cache/v2"
	"github.com/projectcontour/contour/internal/dag"
	"github.com/sirupsen/logrus"
)

// SnapshotHandler implements the xDS snapshot cache
type SnapshotHandler struct {
	// resources holds the cache of xDS contents.
	resources []ResourceCache

	// snapshotCache is a snapshot-based cache that maintains a single versioned
	// snapshot of responses for xDS resources that Contour manages
	snapshotCache cache.SnapshotCache

	logrus.FieldLogger
}

// NewSnapshotHandler returns an instance of SnapShotHandler
func NewSnapshotHandler(c cache.SnapshotCache, resources []ResourceCache, logger logrus.FieldLogger) *SnapshotHandler {
	return &SnapshotHandler{
		snapshotCache: c,
		resources:     resources,
		FieldLogger:   logger,
	}
}

// OnChange is called when the DAG is rebuilt
// and a new snapshot is needed.
func (s *SnapshotHandler) OnChange(root *dag.DAG) {
}
