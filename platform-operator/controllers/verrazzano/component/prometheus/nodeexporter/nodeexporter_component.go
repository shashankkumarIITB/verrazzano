// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package nodeexporter

import (
	"fmt"
	"path/filepath"

	"github.com/verrazzano/verrazzano/pkg/bom"
	vzapi "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	"github.com/verrazzano/verrazzano/platform-operator/constants"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/helm"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/config"
	"github.com/verrazzano/verrazzano/platform-operator/internal/vzconfig"
)

// ComponentName is the name of the component
const ComponentName = "prometheus-node-exporter"

// ComponentNamespace is the namespace of the component
const ComponentNamespace = constants.VerrazzanoMonitoringNamespace

// ComponentJSONName is the json name of the component in the CRD
const ComponentJSONName = "prometheusNodeExporter"

const chartDir = "prometheus-community/prometheus-node-exporter"

var valuesFile = fmt.Sprintf("%s-values.yaml", ComponentName)

type prometheusNodeExporterComponent struct {
	helm.HelmComponent
}

func NewComponent() spi.Component {
	return prometheusNodeExporterComponent{
		helm.HelmComponent{
			ReleaseName:             ComponentName,
			JSONName:                ComponentJSONName,
			ChartDir:                filepath.Join(config.GetThirdPartyDir(), chartDir),
			ChartNamespace:          ComponentNamespace,
			IgnoreNamespaceOverride: true,
			SupportsOperatorInstall: true,
			MinVerrazzanoVersion:    constants.VerrazzanoVersion1_3_0,
			ImagePullSecretKeyname:  "serviceAccount.imagePullSecrets[0].name",
			ValuesFile:              filepath.Join(config.GetHelmOverridesDir(), valuesFile),
			Dependencies:            []string{},
			AppendOverridesFunc:     AppendOverrides,
			GetInstallOverridesFunc: GetOverrides,
		},
	}
}

// IsEnabled returns true if the Prometheus Node-Exporter is enabled or if the component is not specified
// in the Verrazzano CR.
func (c prometheusNodeExporterComponent) IsEnabled(effectiveCR *vzapi.Verrazzano) bool {
	comp := effectiveCR.Spec.Components.PrometheusNodeExporter
	if comp == nil || comp.Enabled == nil {
		return false
	}
	return *comp.Enabled
}

// IsReady checks if the Prometheus Node-Exporter deployment is ready
func (c prometheusNodeExporterComponent) IsReady(ctx spi.ComponentContext) bool {
	if c.HelmComponent.IsReady(ctx) {
		return isPrometheusNodeExporterReady(ctx)
	}
	return false
}

// PreInstall updates resources necessary for the Prometheus Node-Exporter Component installation
func (c prometheusNodeExporterComponent) PreInstall(ctx spi.ComponentContext) error {
	return preInstall(ctx)
}

// AppendOverrides appends install overrides for the Prometheus Node Exporter component's Helm chart
func AppendOverrides(ctx spi.ComponentContext, _ string, _ string, _ string, kvs []bom.KeyValue) ([]bom.KeyValue, error) {
	// Only enable the node exporter's ServiceMonitor if Prometheus Operator is enabled in this install
	ctx.Log().Debug("Appending service monitor override for the Prometheus Node Exporter component")
	if vzconfig.IsPrometheusOperatorEnabled(ctx.EffectiveCR()) {
		kvs = append(kvs, bom.KeyValue{
			Key: "prometheus.monitor.enabled", Value: "true",
		})
	}
	return kvs, nil
}

// MonitorOverrides checks whether monitoring of install overrides is enabled or not
func (c prometheusNodeExporterComponent) MonitorOverrides(ctx spi.ComponentContext) bool {
	if ctx.EffectiveCR().Spec.Components.PrometheusNodeExporter != nil {
		if ctx.EffectiveCR().Spec.Components.PrometheusNodeExporter.MonitorChanges != nil {
			return *ctx.EffectiveCR().Spec.Components.PrometheusNodeExporter.MonitorChanges
		}
		return true
	}
	return false
}
