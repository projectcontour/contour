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

package status

import (
	"fmt"
	"time"

	contour_api_v1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	projectcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type ProxyStatus string

const (
	ProxyStatusValid    ProxyStatus = "valid"
	ProxyStatusInvalid  ProxyStatus = "invalid"
	ProxyStatusOrphaned ProxyStatus = "orphaned"
)

// ProxyUpdate holds status updates for a particular HTTPProxy object
type ProxyUpdate struct {
	Fullname       types.NamespacedName
	Generation     int64
	TransitionTime v1.Time
	Vhost          string

	// Conditions holds all the DetailedConditions to add to the object
	// keyed by the Type (since that's what the apiserver will end up
	// doing.)
	Conditions map[ConditionType]*projectcontour.DetailedCondition
}

// ProxyAccessor returns a ProxyUpdate that allows a client to build up a list of
// errors and warnings to go onto the proxy as conditions, and a function to commit the change
// back to the cache when everything is done.
// The commit function pattern is used so that the ProxyUpdate does not need to know anything
// the cache internals.
func (c *Cache) ProxyAccessor(proxy *contour_api_v1.HTTPProxy) (*ProxyUpdate, func()) {
	pu := &ProxyUpdate{
		Fullname:       k8s.NamespacedNameOf(proxy),
		Generation:     proxy.Generation,
		TransitionTime: metav1.NewTime(time.Now()),
		Conditions:     make(map[ConditionType]*contour_api_v1.DetailedCondition),
	}

	return pu, func() {
		c.commitProxy(pu)
	}
}

func (c *Cache) commitProxy(pu *ProxyUpdate) {
	if len(pu.Conditions) == 0 {
		return
	}

	_, ok := c.proxyUpdates[pu.Fullname]
	if ok {
		// When we're committing, if we already have a Valid Condition with an error, and we're trying to
		// set the object back to Valid, skip the commit, as we've visited too far down.
		// If this is removed, the status reporting for when a parent delegates to a child that delegates to itself
		// will not work. Yes, I know, problems everywhere. I'm sorry.
		// TODO(youngnick)#2968: This issue has more details.
		if c.proxyUpdates[pu.Fullname].Conditions[ValidCondition].Status == contour_api_v1.ConditionFalse {
			if pu.Conditions[ValidCondition].Status == contour_api_v1.ConditionTrue {
				return
			}
		}
	}
	c.proxyUpdates[pu.Fullname] = pu
}

// ConditionFor returns a DetailedCondition for a given ConditionType.
// Currently only "Valid" is used.
func (pu *ProxyUpdate) ConditionFor(cond ConditionType) *projectcontour.DetailedCondition {
	dc, ok := pu.Conditions[cond]
	if !ok {
		newDc := &projectcontour.DetailedCondition{}
		newDc.Type = string(cond)
		newDc.ObservedGeneration = pu.Generation
		if cond == ValidCondition {
			newDc.Status = projectcontour.ConditionTrue
			newDc.Reason = "Valid"
			newDc.Message = "Valid HTTPProxy"
		} else {
			newDc.Status = projectcontour.ConditionFalse
		}
		pu.Conditions[cond] = newDc
		return newDc
	}
	return dc

}

func (pu *ProxyUpdate) Mutate(obj interface{}) interface{} {
	o, ok := obj.(*projectcontour.HTTPProxy)
	if !ok {
		panic(fmt.Sprintf("Unsupported %T object %s/%s in status mutator",
			obj, pu.Fullname.Namespace, pu.Fullname.Name,
		))
	}

	proxy := o.DeepCopy()

	for condType, cond := range pu.Conditions {
		cond.ObservedGeneration = pu.Generation
		cond.LastTransitionTime = pu.TransitionTime

		currCond := proxy.Status.GetConditionFor(string(condType))
		if currCond == nil {
			proxy.Status.Conditions = append(proxy.Status.Conditions, *cond)
			continue
		}

		// Don't update the condition if our observation is stale.
		if currCond.ObservedGeneration > cond.ObservedGeneration {
			continue
		}

		cond.DeepCopyInto(currCond)

	}

	// Set the old status fields using the Valid DetailedCondition's details.
	// Other conditions are not relevant for these two fields.
	validCond := proxy.Status.GetConditionFor(projectcontour.ValidConditionType)

	switch validCond.Status {
	case projectcontour.ConditionTrue:
		// TODO(youngnick): bring the string(ProxyStatusValid) constants in here?
		proxy.Status.CurrentStatus = string(ProxyStatusValid)
		proxy.Status.Description = validCond.Message
	case projectcontour.ConditionFalse:
		if orphanCond, ok := validCond.GetError(projectcontour.ConditionTypeOrphanedError); ok {
			proxy.Status.CurrentStatus = string(ProxyStatusOrphaned)
			proxy.Status.Description = orphanCond.Message
			break
		}
		proxy.Status.CurrentStatus = string(ProxyStatusInvalid)

		// proxy.Status.Description = validCond.Reason + ": " + validCond.Message
		proxy.Status.Description = validCond.Message
	}

	return proxy

}
