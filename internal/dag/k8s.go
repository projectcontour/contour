// Copyright © 2018 Heptio
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

package dag

// ResourceEventHandler converts its embedded DAG into a classic cache.ResourceEventHandler.
type ResourceEventHandler struct {
	DAG
}

func (r *ResourceEventHandler) OnAdd(obj interface{}) IngressrouteStatus {
	r.Insert(obj)
	return r.Recompute()
}

func (r *ResourceEventHandler) OnUpdate(oldObj, newObj interface{}) IngressrouteStatus {
	r.Remove(oldObj)
	r.Insert(newObj)
	return r.Recompute()
}

func (r *ResourceEventHandler) OnDelete(obj interface{}) IngressrouteStatus {
	r.Remove(obj)
	return r.Recompute()
}
