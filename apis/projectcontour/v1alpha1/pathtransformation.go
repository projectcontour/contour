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

package v1alpha1

import "fmt"

// PathWithEscapedSlashesActionType defines the action to take when a request path
// contains escaped slash sequences (%2F, %2f, %5C and %5c). The action is applied
// before path normalization and merge slashes.
type PathWithEscapedSlashesActionType string

const (
	// KeepUnchangedPathWithEscapedSlashes keeps escaped slash sequences unchanged.
	// This is the default value.
	KeepUnchangedPathWithEscapedSlashes PathWithEscapedSlashesActionType = "keep_unchanged"

	// RejectRequestPathWithEscapedSlashes rejects the request with a 400 response.
	RejectRequestPathWithEscapedSlashes PathWithEscapedSlashesActionType = "reject_request"

	// UnescapeAndRedirectPathWithEscapedSlashes unescapes the sequences and responds
	// with a redirect to the normalized path. gRPC requests are rejected instead.
	UnescapeAndRedirectPathWithEscapedSlashes PathWithEscapedSlashesActionType = "unescape_and_redirect"

	// UnescapeAndForwardPathWithEscapedSlashes unescapes the sequences and forwards
	// the request. This should not be used if intermediaries perform path based
	// access control.
	UnescapeAndForwardPathWithEscapedSlashes PathWithEscapedSlashesActionType = "unescape_and_forward"
)

func (a PathWithEscapedSlashesActionType) Validate() error {
	switch a {
	case KeepUnchangedPathWithEscapedSlashes,
		RejectRequestPathWithEscapedSlashes,
		UnescapeAndRedirectPathWithEscapedSlashes,
		UnescapeAndForwardPathWithEscapedSlashes,
		"":
		return nil
	default:
		return fmt.Errorf("invalid path with escaped slashes action %q", a)
	}
}
