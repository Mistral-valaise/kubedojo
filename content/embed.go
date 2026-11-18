// Package content embeds the training content shipped with kubedojo.
package content

import "embed"

//go:embed path.yaml all:cka all:ckad all:cks
var FS embed.FS
