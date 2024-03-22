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

// Package debug provides http endpoints for healthcheck, metrics,
// and pprof debugging.
package debug_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/projectcontour/contour/internal/debug"
)

func TestDebugServiceNotRequireLeaderElection(t *testing.T) {
	var s manager.LeaderElectionRunnable = &debug.Service{}
	require.False(t, s.NeedLeaderElection())
}
