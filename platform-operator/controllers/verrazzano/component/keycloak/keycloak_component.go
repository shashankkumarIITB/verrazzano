// Copyright (c) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.
package keycloak

import (
	"context"
	"path/filepath"

	certmanager "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/helm"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/istio"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/mysql"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/config"
	"github.com/verrazzano/verrazzano/platform-operator/internal/k8s/status"
	"github.com/verrazzano/verrazzano/platform-operator/internal/vzconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ComponentName is the name of the component
const ComponentName = "keycloak"

// KeycloakComponent represents an Keycloak component
type KeycloakComponent struct {
	helm.HelmComponent
}

// Verify that KeycloakComponent implements Component
var _ spi.Component = KeycloakComponent{}

// NewComponent returns a new Keycloak component
func NewComponent() spi.Component {
	return KeycloakComponent{
		helm.HelmComponent{
			ReleaseName:             ComponentName,
			ChartDir:                filepath.Join(config.GetThirdPartyDir(), ComponentName),
			ChartNamespace:          ComponentName,
			IgnoreNamespaceOverride: true,
			//  Check on Image Pull Pull Key
			ValuesFile:              filepath.Join(config.GetHelmOverridesDir(), "keycloak-values.yaml"),
			Dependencies:            []string{istio.ComponentName, mysql.ComponentName},
			SupportsOperatorInstall: true,
			AppendOverridesFunc:     AppendKeycloakOverrides,
		},
	}
}

func (c KeycloakComponent) PreInstall(ctx spi.ComponentContext) error {
	// Check Verrazzano Secret. return error which will cause reque
	secret := &corev1.Secret{}
	err := ctx.Client().Get(context.TODO(), client.ObjectKey{
		Namespace: "verrazzano-system",
		Name:      "verrazzano",
	}, secret)
	if err != nil {
		ctx.Log().Errorf("Keycloak PreInstall: Error retrieving Verrazzano password: %s", err)
		return err
	}
	// Check MySQL Secret. return error which will cause reque
	secret = &corev1.Secret{}
	err = ctx.Client().Get(context.TODO(), client.ObjectKey{
		Namespace: "keycloak",
		Name:      "mysql",
	}, secret)
	if err != nil {
		ctx.Log().Errorf("Keycloak PreInstall: Error retrieving MySQL password: %s", err)
		return err
	}

	// Create secret for the keycloakadmin user if it doesn't exist
	err = createAuthSecret(ctx, "keycloak", "keycloak-http", "keycloakadmin")
	if err != nil {
		return err
	}

	return nil
}

func (c KeycloakComponent) PostInstall(ctx spi.ComponentContext) error {
	// Create secret for the verrazzano-prom-internal user
	err := createAuthSecret(ctx, "verrazzano-system", "verrazzano-prom-internal", "verrazzano-prom-internal")
	if err != nil {
		return err
	}

	// Create secret for the verrazzano-es-internal user
	err = createAuthSecret(ctx, "verrazzano-system", "verrazzano-es-internal", "verrazzano-es-internal")
	if err != nil {
		return err
	}
	// Create the verrazzano-system realm and populate it with users, groups, clients, etc.
	err = configureKeycloakRealms(ctx)
	if err != nil {
		return err
	}

	// If OCI DNS, update annotation on Keycloak Ingress
	if vzconfig.IsExternalDNSEnabled(ctx.EffectiveCR()) {
		err := updateKeycloakIngress(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

// PostUpgrade Keycloak-post-upgrade processing, create or update the Kiali ingress
func (c KeycloakComponent) PostUpgrade(ctx spi.ComponentContext) error {
	if err := c.HelmComponent.PostUpgrade(ctx); err != nil {
		return err
	}
	return updateKeycloakUris(ctx)
}

// IsEnabled Keycloak-specific enabled check for installation
func (c KeycloakComponent) IsEnabled(ctx spi.ComponentContext) bool {
	comp := ctx.EffectiveCR().Spec.Components.Keycloak
	if comp == nil || comp.Enabled == nil {
		return true
	}
	return *comp.Enabled
}

func (c KeycloakComponent) IsReady(ctx spi.ComponentContext) bool {
	// TLS cert from Cert Manager should be in Ready state
	certName := getCertName(ctx.EffectiveCR())
	certificate := &certmanager.Certificate{}
	namespacedName := types.NamespacedName{Name: certName, Namespace: ComponentName}
	if err := ctx.Client().Get(context.TODO(), namespacedName, certificate); err != nil {
		ctx.Log().Infof("Keycloak isReady: Failed to get Keycloak Certificate: %s", err)
		return false
	}
	if certificate.Status.Conditions == nil {
		ctx.Log().Infof("Keycloak IsReady: No Certificate Status conditions found")
		return false
	}

	condition := certificate.Status.Conditions[0]
	statefulsetName := []types.NamespacedName{
		{
			Namespace: ComponentName,
			Name:      ComponentName,
		},
	}
	return condition.Type == "Ready" &&
		status.StatefulsetReady(ctx.Log(), ctx.Client(), statefulsetName, 1)
}
