// Package dashboards embeds the Grafana dashboards shipped with the Helm
// chart so that `kubedojo up --observability` can load the same files locally.
package dashboards

import "embed"

//go:embed *.json
var FS embed.FS
