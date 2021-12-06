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

package controller

import "sigs.k8s.io/controller-runtime/pkg/controller"

// Wrapper for that ensures controller-runtime Controllers
// are run by controller-runtime Manager, regardless of
// leader-election status. Controllers can be created as
// unmanaged and manually registered with a Manager using
// this wrapper, otherwise they will only be run when their
// Manager is elected leader.
type noLeaderElectionController struct {
	controller.Controller
}

func (*noLeaderElectionController) NeedLeaderElection() bool {
	return false
}
