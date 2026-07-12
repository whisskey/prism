// SPDX-FileCopyrightText: 2026 prism
// SPDX-License-Identifier: MIT

package lint

import "go/token"

// Issue is a non-fatal OpenAPI inference warning.
type Issue struct {
	Pos     token.Position
	Message string
}
