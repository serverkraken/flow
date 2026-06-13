// Package domain holds the pure value types and rules of flow.
// It must not import any I/O package.
package domain

import "errors"

var ErrInvalidUser = errors.New("invalid user")
