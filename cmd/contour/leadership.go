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

package main

import (
	"context"
	"os"

	"github.com/google/uuid"
	"github.com/projectcontour/contour/internal/k8s"
	"github.com/projectcontour/contour/internal/workgroup"
	"github.com/projectcontour/contour/pkg/config"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func disableLeaderElection(log logrus.FieldLogger) chan struct{} {
	log.Info("Leader election disabled")

	leader := make(chan struct{})
	close(leader)
	return leader
}

// setupLeadershipElection registers leadership workers with the group and returns
// a channel which will become ready when this process becomes the leader, or, in the
// event that leadership election is disabled, the channel will be ready immediately.
func setupLeadershipElection(
	g *workgroup.Group,
	log logrus.FieldLogger,
	conf *config.LeaderElectionParameters,
	clients *k8s.Clients, updateNow func(),
) chan struct{} {
	le, leader, deposed := newLeaderElector(log, conf, clients)

	g.AddContext(func(electionCtx context.Context) {
		log.WithFields(logrus.Fields{
			"configmapname":      conf.Name,
			"configmapnamespace": conf.Namespace,
		}).Info("started leader election")

		le.Run(electionCtx)
		log.Info("stopped leader election")
	})

	g.Add(func(stop <-chan struct{}) error {
		log := log.WithField("context", "leaderelection")
		for {
			select {
			case <-stop:
				// shut down
				log.Info("stopped leader election")
				return nil
			case <-leader:
				log.Info("elected as leader, triggering rebuild")
				updateNow()

				// disable this case
				leader = nil
			case <-deposed:
				// If we get deposed as leader, shut it down.
				log.Info("deposed as leader, shutting down")
				return nil
			}
		}
	})

	return leader
}

// newLeaderElector creates a new leaderelection.LeaderElector and associated
// channels by which to observe elections and depositions.
func newLeaderElector(
	log logrus.FieldLogger,
	conf *config.LeaderElectionParameters,
	clients *k8s.Clients,
) (*leaderelection.LeaderElector, chan struct{}, chan struct{}) {
	log = log.WithField("context", "leaderelection")
	// leaderOK will block gRPC startup until it's closed.
	leaderOK := make(chan struct{})
	// deposed is closed by the leader election callback when
	// we are deposed as leader so that we can clean up.
	deposed := make(chan struct{})

	rl := newResourceLock(log, conf, clients)

	// Make the leader elector, ready to be used in the Workgroup.
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: conf.LeaseDuration,
		RenewDeadline: conf.RenewDeadline,
		RetryPeriod:   conf.RetryPeriod,
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
	if err != nil {
		log.WithError(err).Fatal("failed to create leader elector")
	}
	return le, leaderOK, deposed
}

// newResourceLock creates a new resourcelock.Interface based on the Pod's name,
// or a uuid if the name cannot be determined.
func newResourceLock(log logrus.FieldLogger, conf *config.LeaderElectionParameters, clients *k8s.Clients) resourcelock.Interface {
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
		conf.Namespace,
		conf.Name,
		clients.ClientSet().CoreV1(),
		clients.ClientSet().CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: resourceLockID,
		},
	)
	if err != nil {
		log.WithError(err).
			WithField("name", conf.Name).
			WithField("namespace", conf.Namespace).
			WithField("identity", resourceLockID).
			Fatal("failed to create new resource lock")
	}
	return rl
}
