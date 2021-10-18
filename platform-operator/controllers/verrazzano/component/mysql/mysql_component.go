// Copyright (c) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package mysql

import (
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/helm"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/config"
	"path/filepath"
)

// ComponentName is the name of the component
const ComponentName = "mysql"

var hc = helm.HelmComponent{
	ReleaseName:             ComponentName,
	ChartDir:                filepath.Join(config.GetThirdPartyDir(), ComponentName),
	ChartNamespace:          "keycloak",
	IgnoreNamespaceOverride: true,
	ValuesFile:              filepath.Join(config.GetHelmOverridesDir(), "mysql-values.yaml"),
}

// MySQLComponent represents an MySQL component
type MySQLComponent struct {
}

// Verify that MySQLComponent implements Component
var _ spi.Component = MySQLComponent{}

// NewComponent returns a new MySQL component
func NewComponent() spi.Component {
	return MySQLComponent{}
}

// --------------------------------------
// ComponentInfo interface functions
// --------------------------------------

// Log returns the logger for the context
func (k MySQLComponent) Name() string {
	return hc.Name()
}

// Log returns the logger for the context
func (k MySQLComponent) GetDependencies() []string {
	return hc.GetDependencies()
}

// IsReady Indicates whether or not a component is available and ready
func (k MySQLComponent) IsReady(context spi.ComponentContext) bool {
	return hc.IsReady(context)
}

// --------------------------------------
// ComponentInstaller interface functions
// --------------------------------------

// IsOperatorInstallSupported Returns true if the component supports install directly via the platform operator
// - scaffolding while we move components from the scripts to the operator
func (k MySQLComponent) IsOperatorInstallSupported() bool {
	return hc.IsOperatorInstallSupported()
}

// IsInstalled Indicates whether or not the component is installed
func (k MySQLComponent) IsInstalled(context spi.ComponentContext) (bool, error) {
	return hc.IsInstalled(context)
}

// PreInstall allows components to perform any pre-processing required prior to initial install
func (k MySQLComponent) PreInstall(context spi.ComponentContext) error {
	return hc.PreInstall(context)
}

// Install performs the initial install of a component
func (k MySQLComponent) Install(context spi.ComponentContext) error {
	return hc.Install(context)
}

// PostInstall allows components to perform any post-processing required after initial install
func (k MySQLComponent) PostInstall(context spi.ComponentContext) error {
	return hc.PostInstall(context)
}

// --------------------------------------
// ComponentUpgrader interface functions
// --------------------------------------

// PreUpgrade allows components to perform any pre-processing required prior to upgrading
func (k MySQLComponent) PreUpgrade(context spi.ComponentContext) error {
	return hc.PreUpgrade(context)
}

// Upgrade will upgrade the Verrazzano component specified in the CR.Version field
func (k MySQLComponent) Upgrade(context spi.ComponentContext) error {
	return hc.Upgrade(context)
}

// PostUpgrade allows components to perform any post-processing required after upgrading
func (k MySQLComponent) PostUpgrade(context spi.ComponentContext) error {
	return hc.PostUpgrade(context)
}

// GetSkipUpgrade returns the value of the SkipUpgrade field
// - Scaffolding for now during the Istio 1.10.2 upgrade process
func (k MySQLComponent) GetSkipUpgrade() bool {
	return hc.GetSkipUpgrade()
}