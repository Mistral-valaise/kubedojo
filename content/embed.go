// Package content embeds the training content shipped with kubedojo.
package content

import "embed"

//go:embed all:cka all:ckad all:cks all:linux
var FS embed.FS
