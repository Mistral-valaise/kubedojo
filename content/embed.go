// Package content embeds the training content shipped with kubedojo.
package content

import "embed"

//go:embed all:cka all:ckad all:cks
var FS embed.FS
