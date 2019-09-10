// Copyright © 2019 VMware
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

package main

import (
	"context"
	"os"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// newLeaderElector creates a new leaderelection.LeaderElector and associated
// channels by which to observe elections and depositions.
func newLeaderElector(log logrus.FieldLogger, ctx *serveContext, client *kubernetes.Clientset, coordinationClient *coordinationv1.CoordinationV1Client) (*leaderelection.LeaderElector, chan struct{}, chan struct{}) {

	// leaderOK will block gRPC startup until it's closed.
	leaderOK := make(chan struct{})
	// deposed is closed by the leader election callback when
	// we are deposed as leader so that we can clean up.
	deposed := make(chan struct{})

	rl := newResourceLock(ctx, client, coordinationClient)

	// Make the leader elector, ready to be used in the Workgroup.
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: ctx.LeaderElectionConfig.LeaseDuration,
		RenewDeadline: ctx.LeaderElectionConfig.RenewDeadline,
		RetryPeriod:   ctx.LeaderElectionConfig.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(_ context.Context) {
				log.WithFields(logrus.Fields{
					"lock":     rl.Describe(),
					"identity": rl.Identity(),
				}).Info("elected leader")
				close(leaderOK)
			},
			OnStoppedLeading: func() {
				// The context being canceled will trigger a handler that will
				// deal with being deposed.
				close(deposed)
			},
		},
	})
	check(err)
	return le, leaderOK, deposed
}

// newResourceLock creates a new resourcelock.Interface based on the Pod's name,
// or a uuid if the name cannot be determined.
func newResourceLock(ctx *serveContext, client *kubernetes.Clientset, coordinationClient *coordinationv1.CoordinationV1Client) resourcelock.Interface {
	resourceLockID, found := os.LookupEnv("POD_NAME")
	if !found {
		resourceLockID = uuid.New().String()
	}

	rl, err := resourcelock.New(
		// TODO(youngnick) change this to a Lease object instead
		// of the configmap once the Lease API has been GA for a full support
		// cycle (ie nine months).
		// Figure out the resource lock ID
		resourcelock.ConfigMapsResourceLock,
		ctx.LeaderElectionConfig.Namespace,
		ctx.LeaderElectionConfig.Name,
		client.CoreV1(),
		coordinationClient,
		resourcelock.ResourceLockConfig{
			Identity: resourceLockID,
		},
	)
	check(err)
	return rl
}
