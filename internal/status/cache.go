package status

import (
	"fmt"

	projcontour "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/projectcontour/contour/internal/k8s"
	"k8s.io/apimachinery/pkg/types"
)

type ConditionType string

const ValidCondition ConditionType = "Valid"

// ProxyUpdate holds status updates for a particular HTTPProxy object
type ProxyUpdate struct {
	Object     projcontour.HTTPProxy
	Conditions map[ConditionType]*projcontour.DetailedCondition
}

// ConditionFor returns a DetailedCondition for a given ConditionType.
// Currently only "Valid" is supported.
func (pu *ProxyUpdate) ConditionFor(cond ConditionType) *projcontour.DetailedCondition {

	if cond == "" {
		return nil
	}

	dc, ok := pu.Conditions[cond]
	if !ok {
		newDc := &projcontour.DetailedCondition{}
		newDc.Type = string(cond)

		pu.Conditions[cond] = newDc
		return newDc
	}
	return dc
}

func (pu *ProxyUpdate) StatusMutatorFunc() k8s.StatusMutator {

	return k8s.StatusMutatorFunc(func(obj interface{}) interface{} {
		switch o := obj.(type) {
		case *projcontour.HTTPProxy:
			proxy := o.DeepCopy()

			for condType, cond := range pu.Conditions {
				condIndex := projcontour.GetConditionIndex(string(condType), proxy.Status.Conditions)
				if condIndex >= 0 {
					proxy.Status.Conditions[condIndex] = *cond
				} else {
					proxy.Status.Conditions = append(proxy.Status.Conditions, *cond)
				}

			}

			// Set the old status fields using the Valid DetailedCondition's details.
			// Other conditions are not relevant for these two fields.
			validCondIndex := projcontour.GetConditionIndex(string(ValidCondition), proxy.Status.Conditions)
			if validCondIndex >= 0 {
				validCond := proxy.Status.Conditions[validCondIndex]
				switch validCond.Status {
				case projcontour.ConditionTrue:
					orphanCond, orphaned := validCond.GetError(k8s.StatusOrphaned)
					if orphaned {
						proxy.Status.CurrentStatus = k8s.StatusOrphaned
						proxy.Status.CurrentStatus = orphanCond.Message
					}
					proxy.Status.CurrentStatus = k8s.StatusValid
					proxy.Status.Description = validCond.Reason + ": " + validCond.Message
				case projcontour.ConditionFalse:
					proxy.Status.CurrentStatus = k8s.StatusInvalid
					proxy.Status.Description = validCond.Reason + ": " + validCond.Message
				}
			}

			return proxy
		default:
			panic(fmt.Sprintf("Unsupported object %s/%s in status Address mutator",
				pu.Object.Namespace, pu.Object.Name,
			))
		}

	})
}

// NewCache creates a new Cache for holding status updates.
func NewCache() Cache {
	return Cache{
		proxyUpdates: make(map[types.NamespacedName]*ProxyUpdate),
	}
}

// Cache holds status updates from the DAG back towards Kubernetes.
// It holds a per-Kind cache, and is intended to be accessed with a
// KindAccessor.
type Cache struct {
	proxyUpdates map[types.NamespacedName]*ProxyUpdate
}

// ProxyAccessor returns a ProxyUpdate that allows a client to build up a list of
// errors and warnings to go onto the proxy as conditions, and a function to commit the change
// back to the cache when everything is done.
// The commit function pattern is used so that the ProxyUpdate does not need to know anything
// the cache internals.
func (c Cache) ProxyAccessor(proxy *projcontour.HTTPProxy) (*ProxyUpdate, func()) {

	pu := &ProxyUpdate{
		Object:     *proxy,
		Conditions: make(map[ConditionType]*projcontour.DetailedCondition),
	}

	return pu, func() {
		c.commitProxy(pu)
	}
}

func (c Cache) commitProxy(pu *ProxyUpdate) {
	if len((pu.Conditions)) == 0 {
		return
	}

	fullname := types.NamespacedName{
		Name:      pu.Object.Name,
		Namespace: pu.Object.Namespace,
	}

	c.proxyUpdates[fullname] = pu
}

// GetStatusUpdates returns a slice of StatusUpdates, ready to be sent off
// to the StatusUpdater by the event handler.
// As more kinds are handled by Cache, we'll update this method.
func (c Cache) GetStatusUpdates() []k8s.StatusUpdate {

	return c.getProxyStatusUpdates()
}

func (c Cache) getProxyStatusUpdates() []k8s.StatusUpdate {

	var psu []k8s.StatusUpdate

	for fullname, pu := range c.proxyUpdates {

		update := k8s.StatusUpdate{
			NamespacedName: fullname,
			Resource:       projcontour.HTTPProxyGVR,
			Mutator:        pu.StatusMutatorFunc(),
		}

		psu = append(psu, update)
	}
	return psu

}
