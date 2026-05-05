// Package assets is a thin wrapper exposing the embedded FS containing the
// runtime CSS, Datastar JS bundle, and heroicon SVG subset.
package assets

import "embed"

// FS holds the embedded static-asset tree (dist/, js/, icons/).
// It is used by handlers.MountStatic to serve files at /_assets/*.
//
//go:embed all:dist all:js all:icons
var FS embed.FS
