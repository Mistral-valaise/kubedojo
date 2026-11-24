// Package provisioning embeds the Grafana provisioning files of the
// single-container stack. The Helm chart reads the same files for
// lgtm.enabled, so `kubedojo up --observability` and the chart cannot drift.
package provisioning

import _ "embed"

// Datasources replaces the data sources file of grafana/otel-lgtm.
//
//go:embed datasources.yaml
var Datasources []byte

// DashboardProvider points Grafana at the mounted dashboards.
//
//go:embed dashboards-provider.yaml
var DashboardProvider []byte

// Tempo replaces Tempo's configuration file of grafana/otel-lgtm: the
// image's file with a higher limit for the size of an attribute.
//
//go:embed tempo.yaml
var Tempo []byte

// Where the files belong inside the grafana/otel-lgtm container.
const (
	DatasourcesPath       = "/otel-lgtm/grafana/conf/provisioning/datasources/grafana-datasources.yaml"
	DashboardProviderPath = "/otel-lgtm/grafana/conf/provisioning/dashboards/kubedojo.yaml"
	DashboardsPath        = "/otel-lgtm/kubedojo-dashboards"
	TempoPath             = "/otel-lgtm/tempo-config.yaml"
)
