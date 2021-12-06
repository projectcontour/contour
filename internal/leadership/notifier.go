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

package leadership

import "context"

type NeedLeaderElectionNotification interface {
	OnElectedLeader()
}

// Notifier is controller-runtime manager runnable that can be used to
// notify other components when leader election has occurred for the current
// manager.
type Notifier struct {
	ToNotify []NeedLeaderElectionNotification
}

func (n *Notifier) NeedLeaderElection() bool {
	return true
}

func (n *Notifier) Start(ctx context.Context) error {
	for _, t := range n.ToNotify {
		go t.OnElectedLeader()
	}
	<-ctx.Done()
	return nil
}
