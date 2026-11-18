// Package images embeds the sandbox image build contexts so a release binary
// can build them locally when they cannot be pulled.
package images

import (
	"embed"
	"regexp"
)

//go:embed all:node all:terminal all:linux
var FS embed.FS

// RevLabel is the image label that carries the revision of an image's
// contents. The tag cannot: it names the Kubernetes version only.
const RevLabel = "dev.kubedojo.image-rev"

var revRe = regexp.MustCompile(`(?m)^LABEL\s+` + regexp.QuoteMeta(RevLabel) + `="?([^"\s]+)"?\s*$`)

// Rev returns the revision the Dockerfile in dir ("node", "terminal", "linux") declares,
// or "" if it declares none.
func Rev(dir string) string {
	b, err := FS.ReadFile(dir + "/Dockerfile")
	if err != nil {
		return ""
	}
	if m := revRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}
