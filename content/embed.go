// Package content embeds the training content shipped with kubedojo.
package content

import "embed"

//go:embed path.yaml all:cka all:ckad all:cks all:kcna all:kcsa
var FS embed.FS
