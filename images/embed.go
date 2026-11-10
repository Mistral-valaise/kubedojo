// Package images embeds the sandbox image build contexts so a release binary
// can build them locally when they cannot be pulled.
package images

import "embed"

//go:embed all:node all:terminal
var FS embed.FS
