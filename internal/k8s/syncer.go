// Copyright © 2020 VMware
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
package k8s

import (
	"fmt"

	"k8s.io/client-go/tools/cache"
)

// InformerSyncList holds the functions to call to check that an informer is synced.
type InformerSyncList struct {
	syncers []cache.InformerSynced
}

// Add adds the sync function from an informer to InformerSyncList and returns the informer
// so that .AddEventHandler() can be called on it.
func (sl *InformerSyncList) Add(inf cache.SharedIndexInformer) cache.SharedIndexInformer {
	sl.syncers = append(sl.syncers, inf.HasSynced)
	return inf
}

// WaitForSync ensures that all the informers in the InformerSyncList are synced before returning.
func (sl *InformerSyncList) WaitForSync(stop <-chan struct{}) error {
	if !cache.WaitForCacheSync(stop, sl.syncers...) {
		return fmt.Errorf("error waiting for cache to sync")
	}
	return nil
}
