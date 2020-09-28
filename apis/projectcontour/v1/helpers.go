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

package v1

import (
	"fmt"
	"strings"
)

// AuthorizationConfigured returns whether authorization  is
// configured on this virtual host.
func (v *VirtualHost) AuthorizationConfigured() bool {
	return v.TLS != nil && v.Authorization != nil
}

// DisableAuthorization returns true if this virtual host disables
// authorization. If an authorization server is present, the default
// policy is to not disable.
func (v *VirtualHost) DisableAuthorization() bool {
	// No authorization, so it is disabled.
	if v.AuthorizationConfigured() {
		// No policy specified, default is to not disable.
		if v.Authorization.AuthPolicy == nil {
			return false
		}

		return v.Authorization.AuthPolicy.Disabled
	}

	return false
}

// AuthorizationContext returns the authorization policy context (if present).
func (v *VirtualHost) AuthorizationContext() map[string]string {
	if v.AuthorizationConfigured() {
		if v.Authorization.AuthPolicy != nil {
			return v.Authorization.AuthPolicy.Context
		}
	}

	return nil
}

// GetPrefixReplacements returns replacement prefixes from the path
// rewrite policy (if any).
func (r *Route) GetPrefixReplacements() []ReplacePrefix {
	if r.PathRewritePolicy != nil {
		return r.PathRewritePolicy.ReplacePrefix
	}
	return nil
}

// AuthorizationContext merges the parent context entries with the
// context from this Route. Common keys from the parent map will be
// overwritten by keys from the route. The parent map may be nil.
func (r *Route) AuthorizationContext(parent map[string]string) map[string]string {
	values := make(map[string]string, len(parent))

	for k, v := range parent {
		values[k] = v
	}

	if r.AuthPolicy != nil {
		for k, v := range r.AuthPolicy.Context {
			values[k] = v
		}
	}

	if len(values) == 0 {
		return nil
	}

	return values
}

// AddError adds an error-level Subcondition to the DetailedCondition.
// AddError will also update the DetailedCondition's state to take into account
// the error that's present.
// If a SubCondition with the given errorType exists, will change
// the reason to "MultipleReasons", and will append the given message
// to the existing SubCondition's message.
func (dc *DetailedCondition) AddError(errorType, reason, message string) {
	message = truncateLongMessage(message)

	// Update the condition so that it indicates there's at least one error
	// This needs to be here because conditions may be normal-true (positive)
	// polarity (like `Valid`), or abnormal-true (negative) polarity
	// (like `ErrorPresent`)
	if dc.IsPositivePolarity() {
		dc.Status = ConditionFalse
	} else {
		dc.Status = ConditionTrue
	}

	// Update the DetailedCondition Reason and message if the reason
	// is different to the current one. We have to namespace
	// the DetailedCondition reason by the errorType because we're
	// adding SubConditions; DetailedCondition Reason is a summary of the
	// SubCondition reasons.
	detailedReason := errorType + reason
	dc.updateReason(detailedReason, message)
	// Now we can actually upsert the error.
	i := getIndex(errorType, dc.Errors)

	if i == -1 {
		dc.Errors = append(dc.Errors, SubCondition{
			Type:    errorType,
			Status:  ConditionTrue,
			Message: message,
			Reason:  reason,
		})
		return
	}

	// If the error is already present, just update the reason and message.
	dc.Errors[i].updateReason(reason, message)

}

// AddErrorf adds an error-level Subcondition to the DetailedCondition, using
// fmt.Sprintf on the formatmsg and args params.
// If a SubCondition with the given errorType exists, will change
// the reason to "MultipleReasons", and will append the given message
// to the existing SubCondition's message.
func (dc *DetailedCondition) AddErrorf(errorType, reason, formatmsg string, args ...interface{}) {
	dc.AddError(errorType, reason, fmt.Sprintf(formatmsg, args...))
}

// GetError gets an error of the given errorType.
// Similar to a hash lookup, will return true in the second value if a match is
// found, and false otherwise.
func (dc *DetailedCondition) GetError(errorType string) (SubCondition, bool) {
	i := getIndex(errorType, dc.Errors)

	if i == -1 {
		return SubCondition{}, false
	}

	return dc.Errors[i], true
}

// AddWarning adds an warning-level Subcondition to the DetailedCondition.
// If a SubCondition with the given warnType exists, will change
// the reason to "MultipleReasons", and will append the given message
// to the existing SubCondition's message.
func (dc *DetailedCondition) AddWarning(warnType, reason, message string) {
	message = truncateLongMessage(message)

	detailedReason := warnType + reason
	dc.updateReason(detailedReason, message)

	i := getIndex(warnType, dc.Warnings)

	if i == -1 {
		dc.Warnings = append(dc.Warnings, SubCondition{
			Type:    warnType,
			Status:  ConditionTrue,
			Reason:  reason,
			Message: message,
		})
		return
	}

	// If the warning is already present, just update the reason and message.
	dc.Warnings[i].updateReason(reason, message)
}

// AddWarningf adds an warning-level Subcondition to the DetailedCondition, using
// fmt.Sprintf on the formatmsg and args params.
// If a SubCondition with the given errorType exists, will change
// the reason to "MultipleReasons", and will append the given message
// to the existing SubCondition's message.
func (dc *DetailedCondition) AddWarningf(warnType, reason, formatmsg string, args ...interface{}) {
	dc.AddWarning(warnType, reason, fmt.Sprintf(formatmsg, args...))
}

// GetWarning gets an warning of the given warnType.
// Similar to a hash lookup, will return true in the second value if a match is
// found, and false otherwise.
func (dc *DetailedCondition) GetWarning(warnType string) (SubCondition, bool) {
	i := getIndex(warnType, dc.Warnings)

	if i == -1 {
		return SubCondition{}, false
	}

	return dc.Warnings[i], true
}

// updateReason updates a DetailedCondition's reason and message correctly
// if they are different to the existing ones.
// Note that this helper may be used to build the first iteration of a
// DetailedCondition, so it's possible that `Reason` and/or `Message` will
// be empty when it is called.
func (dc *DetailedCondition) updateReason(reason, message string) {
	if dc.Reason == "" {
		dc.Reason = reason
		dc.Message = message
		return
	}

	if dc.Reason != reason {
		dc.Reason = "MultipleProblems"
		dc.Message = "Multiple problems were found, see errors or warnings for details"
		return
	}

	// This case covers the same `Reason` being used multiple times.
	// The only case for this is if we're adding more details about multiple errors for the same reason.
	if dc.Message != message {
		// Only add the message if it's not already in there somewhere.
		if !strings.Contains(dc.Message, message) {
			dc.Message = dc.Message + ", " + message
		}
	}
}

// IsPositivePolarity returns true if the DetailedCondition is a positive-polarity
// condition like `Valid` or `Ready`, and false otherwise.
func (dc *DetailedCondition) IsPositivePolarity() bool {
	switch dc.Type {
	case ValidConditionType:
		return true
	default:
		return false
	}
}

func (sc *SubCondition) updateReason(reason, message string) {
	if sc.Reason == "" {
		sc.Reason = reason
		sc.Message = message
		return
	}

	if sc.Reason != reason {
		sc.Reason = "MultipleReasons"
	}

	if !strings.Contains(sc.Message, message) {
		sc.Message = sc.Message + ", " + message
	}

}

// getIndex checks if a SubCondition of type condType exists in the
// slice, and returns its index if so. If not, returns -1.
func getIndex(condType string, subconds []SubCondition) int {

	for i, cond := range subconds {
		if cond.Type == condType {
			return i
		}
	}
	return -1
}

// GetConditionFor returns the a pointer to the condition for a given type,
// or nil if there are none currently present.
func (status *HTTPProxyStatus) GetConditionFor(condType string) *DetailedCondition {

	for i, cond := range status.Conditions {
		if cond.Type == condType {
			return &status.Conditions[i]
		}
	}

	return nil
}

// LongMessageLength specifies the maximum size any message field should be.
// This is enforced on the apiserver side by CRD validation requirements.
const LongMessageLength = 32760

// truncateLongMessage truncates long message strings
// to near the max size.
func truncateLongMessage(message string) string {
	if len(message) > LongMessageLength {
		return message[:LongMessageLength]
	}
	return message
}
