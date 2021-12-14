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
// +build e2e

package incluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func testLeaderElection(namespace string) {
	// This test is solely a check on the fact that we have set up leader
	// election resources as expected. This does not test that internal
	// components (e.g. status writers) are set up properly given a contour
	// instance's leader status. That should be tested via more granular
	// unit tests as it is difficult to observe e.g. which contour instance
	// has set status on an object.
	Specify("leader election resources are created as expected", func() {
		getLeaderID := func() (string, error) {
			type leaderInfo struct {
				HolderIdentity string
			}

			leaderElectionConfigMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "leader-elect",
					Namespace: f.Deployment.Namespace.Name,
				},
			}
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(leaderElectionConfigMap), leaderElectionConfigMap); err != nil {
				return "", err
			}

			leaderElectionLease := &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "leader-elect",
					Namespace: f.Deployment.Namespace.Name,
				},
			}
			if err := f.Client.Get(context.TODO(), client.ObjectKeyFromObject(leaderElectionLease), leaderElectionLease); err != nil {
				return "", err
			}

			var (
				infoRaw string
				found   bool
				li      leaderInfo
			)

			if infoRaw, found = leaderElectionConfigMap.Annotations["control-plane.alpha.kubernetes.io/leader"]; !found {
				return "", errors.New("leaderelection configmap did not have leader annotation: control-plane.alpha.kubernetes.io/leader")
			}
			if err := json.Unmarshal([]byte(infoRaw), &li); err != nil {
				return "", err
			}
			leaseHolder := pointer.StringDeref(leaderElectionLease.Spec.HolderIdentity, "")
			if leaseHolder != li.HolderIdentity {
				return "", fmt.Errorf("lease leader %q and configmap leader %q do not match", leaseHolder, li.HolderIdentity)
			}
			if !strings.HasPrefix(li.HolderIdentity, "contour-") {
				return "", fmt.Errorf("invalid leader name: %q", li.HolderIdentity)
			}
			return li.HolderIdentity, nil
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
				events := &corev1.EventList{}
				listOptions := &client.ListOptions{
					Namespace: f.Deployment.Namespace.Name,
				}
				if err := f.Client.List(context.TODO(), events, listOptions); err != nil {
					return false
				}
				foundEvents := map[string]struct{}{}
				for _, e := range events.Items {
					if e.Reason == "LeaderElection" && e.Source.Component == leader {
						foundEvents[e.InvolvedObject.Kind] = struct{}{}
					}
				}
				_, foundLease := foundEvents["Lease"]
				_, foundConfigMap := foundEvents["ConfigMap"]
				return foundLease && foundConfigMap
			}
		}

		require.Eventually(f.T(), findEventsForLeader(originalLeader), f.RetryTimeout, f.RetryInterval)

		// Delete contour leader pod.
		leaderPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
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
		leaderPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podNameFromLeaderID(newLeader),
				Namespace: f.Deployment.Namespace.Name,
			},
		}
		require.NoError(f.T(), f.Client.Get(context.TODO(), client.ObjectKeyFromObject(leaderPod), leaderPod))
	})
}
