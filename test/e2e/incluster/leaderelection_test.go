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

//go:build e2e

package incluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/require"
	coordination_v1 "k8s.io/api/coordination/v1"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testLeaderElection() {
	// This test is solely a check on the fact that we have set up leader
	// election resources as expected. This does not test that internal
	// components (e.g. status writers) are set up properly given a contour
	// instance's leader status. That should be tested via more granular
	// unit tests as it is difficult to observe e.g. which contour instance
	// has set status on an object.
	Specify("leader election resources are created as expected", func() {
		getLeaderID := func() (string, error) {
			leaderElectionLease := &coordination_v1.Lease{
				ObjectMeta: meta_v1.ObjectMeta{
					Name:      "leader-elect",
					Namespace: f.Deployment.Namespace.Name,
				},
			}
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(leaderElectionLease), leaderElectionLease); err != nil {
				return "", err
			}

			leaseHolder := ptr.Deref(leaderElectionLease.Spec.HolderIdentity, "")
			if !strings.HasPrefix(leaseHolder, "contour-") {
				return "", fmt.Errorf("invalid leader name: %q", leaseHolder)
			}
			return leaseHolder, nil
		}

		podNameFromLeaderID := func(id string) string {
			require.Greater(f.T(), len(id), 37)
			return id[:len(id)-37]
		}

		var originalLeader string
		require.Eventually(f.T(), func() bool {
			var err error
			originalLeader, err = getLeaderID()
			return err == nil
		}, 2*time.Minute, f.RetryInterval)

		findEventsForLeader := func(leader string) func() bool {
			return func() bool {
				events := &core_v1.EventList{}
				listOptions := &client.ListOptions{
					Namespace: f.Deployment.Namespace.Name,
				}
				if err := f.Client.List(context.TODO(), events, listOptions); err != nil {
					return false
				}
				for _, e := range events.Items {
					if e.Reason == "LeaderElection" && e.Source.Component == leader && e.InvolvedObject.Kind == "Lease" {
						return true
					}
				}
				return false
			}
		}

		require.Eventually(f.T(), findEventsForLeader(originalLeader), f.RetryTimeout, f.RetryInterval)

		// Delete contour leader pod.
		leaderPod := &core_v1.Pod{
			ObjectMeta: meta_v1.ObjectMeta{
				// Chop off _UUID suffix
				Name:      podNameFromLeaderID(originalLeader),
				Namespace: f.Deployment.Namespace.Name,
			},
		}
		require.NoError(f.T(), f.Client.Delete(context.TODO(), leaderPod))

		newLeader := originalLeader
		require.Eventually(f.T(), func() bool {
			var err error
			newLeader, err = getLeaderID()
			if err != nil {
				return false
			}
			return newLeader != originalLeader
		}, 2*time.Minute, f.RetryInterval)

		require.Eventually(f.T(), findEventsForLeader(newLeader), f.RetryTimeout, f.RetryInterval)

		// Check leader pod exists.
		leaderPod = &core_v1.Pod{
			ObjectMeta: meta_v1.ObjectMeta{
				Name:      podNameFromLeaderID(newLeader),
				Namespace: f.Deployment.Namespace.Name,
			},
		}
		require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(leaderPod), leaderPod))
	})
}
