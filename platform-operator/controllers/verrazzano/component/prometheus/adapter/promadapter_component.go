// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package adapter

import (
	"path/filepath"

	vzapi "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	"github.com/verrazzano/verrazzano/platform-operator/constants"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/helm"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/config"
)

// ComponentName is the name of the component
const ComponentName = "prometheus-adapter"

// ComponentNamespace is the namespace of the component
const ComponentNamespace = constants.VerrazzanoMonitoringNamespace

// ComponentJSONName is the json name of the component in the CRD
const ComponentJSONName = "prometheusAdapter"

const chartDir = "prometheus-community/prometheus-adapter"

type prometheusAdapterComponent struct {
	helm.HelmComponent
}

func NewComponent() spi.Component {
	return prometheusAdapterComponent{
		helm.HelmComponent{
			ReleaseName:             ComponentName,
			JSONName:                ComponentJSONName,
			ChartDir:                filepath.Join(config.GetThirdPartyDir(), chartDir),
			ChartNamespace:          ComponentNamespace,
			IgnoreNamespaceOverride: true,
			SupportsOperatorInstall: true,
			MinVerrazzanoVersion:    constants.VerrazzanoVersion1_3_0,
			ImagePullSecretKeyname:  "image.pullSecrets[0]",
			ValuesFile:              filepath.Join(config.GetHelmOverridesDir(), "prometheus-adapter-values.yaml"),
			Dependencies:            []string{},
			GetInstallOverridesFunc: GetOverrides,
		},
	}
}

// IsEnabled returns true if the Prometheus Adapter is enabled or if the component is not specified
// in the Verrazzano CR.
func (c prometheusAdapterComponent) IsEnabled(effectiveCR *vzapi.Verrazzano) bool {
	comp := effectiveCR.Spec.Components.PrometheusAdapter
	if comp == nil || comp.Enabled == nil {
		return false
	}
	return *comp.Enabled
}

// IsReady checks if the Prometheus Adapter deployment is ready
func (c prometheusAdapterComponent) IsReady(ctx spi.ComponentContext) bool {
	if c.HelmComponent.IsReady(ctx) {
		return isPrometheusAdapterReady(ctx)
	}
	return false
}

// PreInstall updates resources necessary for the Prometheus Adapter Component installation
func (c prometheusAdapterComponent) PreInstall(ctx spi.ComponentContext) error {
	return preInstall(ctx)
}

// MonitorOverrides checks whether monitoring of install overrides is enabled or not
func (c prometheusAdapterComponent) MonitorOverrides(ctx spi.ComponentContext) bool {
	if ctx.EffectiveCR().Spec.Components.PrometheusAdapter != nil {
		if ctx.EffectiveCR().Spec.Components.PrometheusAdapter.MonitorChanges != nil {
			return *ctx.EffectiveCR().Spec.Components.PrometheusAdapter.MonitorChanges
		}
		return true
	}
	return false
}
