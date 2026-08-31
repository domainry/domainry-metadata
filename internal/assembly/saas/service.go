// Package saas reserves the standalone Metadata composition boundary.
// Metadata is currently wired by Runtime only as an embedded Module; a remote
// service must not be advertised until the SDK defines its SaaS contract.
package saas

import "errors"

var ErrNotSupported = errors.New("Metadata SaaS topology is not supported by the current SDK")
