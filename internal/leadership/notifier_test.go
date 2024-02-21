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

package leadership_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/projectcontour/contour/internal/leadership"
	"github.com/projectcontour/contour/internal/leadership/mocks"
)

func TestNotifier(t *testing.T) {
	toNotify1 := &mocks.NeedLeaderElectionNotification{}
	toNotify1.On("OnElectedLeader").Once()
	toNotify2 := &mocks.NeedLeaderElectionNotification{}
	toNotify2.On("OnElectedLeader").Once()

	notifier := &leadership.Notifier{
		ToNotify: []leadership.NeedLeaderElectionNotification{
			toNotify1,
			toNotify2,
		},
	}

	wg := new(sync.WaitGroup)
	ctx, cancel := context.WithCancel(context.Background())

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = notifier.Start(ctx)
	}()

	// Assert we don't return until cancel
	require.Never(t, func() bool {
		wg.Wait()
		return true
	}, time.Second, time.Millisecond*10)

	require.True(t, toNotify1.AssertExpectations(t))
	require.True(t, toNotify2.AssertExpectations(t))

	cancel()

	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, time.Second, time.Millisecond*10)
}

func TestNotifierRequiresLeaderElection(t *testing.T) {
	var notifier manager.LeaderElectionRunnable = &leadership.Notifier{}
	require.True(t, notifier.NeedLeaderElection())
}
