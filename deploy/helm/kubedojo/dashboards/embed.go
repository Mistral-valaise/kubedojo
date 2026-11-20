// Package dashboards embeds the Grafana dashboards shipped with the Helm
// chart so that `kubedojo up --observability` can load the same files locally.
//
// Where a dashboard lives says where its data comes from:
//
//	*.json            kubedojo's own metrics, logs and traces; they work everywhere
//	kubernetes/*.json kubelet/cAdvisor and kube-state-metrics; installed by the chart only
//	local/*.json      the Docker containers of a local kubedojo; loaded by the CLI only
//	lgtm/*.json       the collector of the single-container stack (CLI and lgtm.enabled)
package dashboards

import "embed"

//go:embed *.json kubernetes/*.json local/*.json lgtm/*.json
var FS embed.FS

// Local are the patterns of the dashboards `kubedojo up --observability` loads.
var Local = []string{"*.json", "local/*.json", "lgtm/*.json"}
