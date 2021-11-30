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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		getLeaderPodName := func() (string, error) {
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
			if !strings.HasPrefix(li.HolderIdentity, "contour-") {
				return "", fmt.Errorf("invalid leader name: %q", li.HolderIdentity)
			}
			return li.HolderIdentity, nil
		}

		originalLeader, err := getLeaderPodName()
		require.NoError(f.T(), err)

		// Delete contour leader pod.
		leaderPod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      originalLeader,
				Namespace: f.Deployment.Namespace.Name,
			},
		}
		require.NoError(f.T(), f.Client.Delete(context.TODO(), leaderPod))

		require.Eventually(f.T(), func() bool {
			leader, err := getLeaderPodName()
			if err != nil {
				return false
			}
			return leader != originalLeader
		}, 2*time.Minute, f.RetryInterval)
	})
}
