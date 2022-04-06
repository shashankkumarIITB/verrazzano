// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package authproxy

import (
	"context"
	"fmt"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/common"
	"io/fs"
	"io/ioutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"os"
	clipkg "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"

	"github.com/verrazzano/verrazzano/pkg/bom"
	vzapi "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/config"
	"github.com/verrazzano/verrazzano/platform-operator/internal/k8s/status"
	"github.com/verrazzano/verrazzano/platform-operator/internal/vzconfig"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

const (
	keycloakInClusterURL = "keycloak-http.keycloak.svc.cluster.local"
	tmpFilePrefix        = "authproxy-overrides-"
	tmpSuffix            = "yaml"
	tmpFileCreatePattern = tmpFilePrefix + "*." + tmpSuffix
	tmpFileCleanPattern  = tmpFilePrefix + ".*\\." + tmpSuffix
)

var (
	// For Unit test purposes
	writeFileFunc = ioutil.WriteFile
)

func resetWriteFileFunc() {
	writeFileFunc = ioutil.WriteFile
}

// isAuthProxyReady checks if the AuthProxy deployment is ready
func isAuthProxyReady(ctx spi.ComponentContext) bool {
	deployments := []types.NamespacedName{
		{
			Name:      ComponentName,
			Namespace: ComponentNamespace,
		},
	}
	prefix := fmt.Sprintf("Component %s", ctx.GetComponent())
	return status.DeploymentsAreReady(ctx.Log(), ctx.Client(), deployments, 1, prefix)
}

// AppendOverrides builds the set of verrazzano-authproxy overrides for the helm install
func AppendOverrides(ctx spi.ComponentContext, _ string, _ string, _ string, kvs []bom.KeyValue) ([]bom.KeyValue, error) {
	effectiveCR := ctx.EffectiveCR()
	// Overrides object to store any user overrides
	overrides := authProxyValues{}

	// Environment name
	overrides.Config = &configValues{
		EnvName: vzconfig.GetEnvName(effectiveCR),
	}

	// DNS Suffix
	dnsSuffix, err := vzconfig.GetDNSSuffix(ctx.Client(), effectiveCR)
	if err != nil {
		return nil, err
	}
	overrides.Config.DNSSuffix = dnsSuffix

	ingressType, err := vzconfig.GetServiceType(effectiveCR)
	if err != nil {
		return nil, err
	}
	switch ingressType {
	case vzapi.NodePort:
		for _, ports := range effectiveCR.Spec.Components.Ingress.Ports {
			if ports.Port == 443 {
				dnsSuffix = fmt.Sprintf("%s:%s", dnsSuffix, strconv.Itoa(int(ports.NodePort)))
			}
		}
	}

	overrides.Proxy = &proxyValues{
		OidcProviderHost:          fmt.Sprintf("keycloak.%s.%s", overrides.Config.EnvName, dnsSuffix),
		OidcProviderHostInCluster: keycloakInClusterURL,
	}

	// Image name and version
	err = loadImageSettings(ctx, &overrides)
	if err != nil {
		return nil, err
	}

	// DNS Values
	if isWildcardDNS, domain := getWildcardDNS(&effectiveCR.Spec); isWildcardDNS {
		overrides.DNS = &dnsValues{
			Wildcard: &wildcardDNSValues{
				Domain: domain,
			},
		}
	}

	// Kubernetes settings
	err = loadKubernetesSettings(ctx, &overrides)
	if err != nil {
		return nil, err
	}

	// Write the overrides file to a temp dir and add a helm file override argument
	overridesFileName, err := generateOverridesFile(ctx, &overrides)
	if err != nil {
		return kvs, ctx.Log().ErrorfNewErr("Failed generating AuthProxy overrides file: %v", err)
	}

	// Append any installArgs overrides in vzkvs after the file overrides to ensure precedence of those
	kvs = append(kvs, bom.KeyValue{Value: overridesFileName, IsFile: true})

	return kvs, nil
}

// GetHelmManagedResources returns a list of extra resource types and their namespaced names that are managed by the
// authproxy helm chart
func GetHelmManagedResources() []common.HelmManagedResource {
	return []common.HelmManagedResource{
		{Obj: &corev1.Service{}, NamespacedName: types.NamespacedName{Name: "verrazzano-authproxy-elasticsearch", Namespace: ComponentNamespace}},
		{Obj: &corev1.Secret{}, NamespacedName: types.NamespacedName{Name: "verrazzano-authproxy-secret", Namespace: ComponentNamespace}},
		{Obj: &corev1.ConfigMap{}, NamespacedName: types.NamespacedName{Name: "verrazzano-authproxy-config", Namespace: ComponentNamespace}},
		{Obj: &v1.Ingress{}, NamespacedName: types.NamespacedName{Name: "verrazzano-ingress", Namespace: ComponentNamespace}},
	}
}

// authproxyPreHelmOps ensures the authproxy associated resources are managed its helm install/upgrade executions by
// ensuring the resource policy of "keep" is removed (if it remains then helm is unable to delete these resources and
// they will become orphaned)
func authproxyPreHelmOps(ctx spi.ComponentContext) error {
	return reassociateResources(ctx.Client())
}

//reassociateResources updates the resources to ensure they are managed by this release/component.  The resource policy
// annotation is removed to ensure that helm manages the lifecycle of the resources (the resource policy annotation is
// added to ensure the resources are disassociated from the VZ chart which used to manage these resources)
func reassociateResources(cli clipkg.Client) error {
	namespacedName := types.NamespacedName{Name: ComponentName, Namespace: ComponentNamespace}
	name := types.NamespacedName{Name: ComponentName}
	objects := []controllerutil.Object{
		&corev1.ServiceAccount{},
		&corev1.Service{},
		&appsv1.Deployment{},
	}

	noNamespaceObjects := []controllerutil.Object{
		&rbacv1.ClusterRole{},
		&rbacv1.ClusterRoleBinding{},
	}

	// namespaced resources
	for _, obj := range objects {
		if _, err := removeResourcePolicyAnnotation(cli, obj, namespacedName); err != nil {
			return err
		}
	}

	// additional namespaced resources managed by this helm chart
	authProxyResources := GetHelmManagedResources()
	for _, managedResoure := range authProxyResources {
		if _, err := removeResourcePolicyAnnotation(cli, managedResoure.Obj, managedResoure.NamespacedName); err != nil {
			return err
		}
	}

	// cluster resources
	for _, obj := range noNamespaceObjects {
		if _, err := removeResourcePolicyAnnotation(cli, obj, name); err != nil {
			return err
		}
	}
	return nil
}

// removeResourcePolicyAnnotation removes the resource policy annotation to allow the resource to be managed by helm
func removeResourcePolicyAnnotation(cli clipkg.Client, obj controllerutil.Object, namespacedName types.NamespacedName) (controllerutil.Object, error) {
	if err := cli.Get(context.TODO(), namespacedName, obj); err != nil {
		if errors.IsNotFound(err) {
			return obj, nil
		}
		return obj, err
	}
	objMerge := clipkg.MergeFrom(obj.DeepCopyObject())
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return obj, nil
	}
	delete(annotations, "helm.sh/resource-policy")
	obj.SetAnnotations(annotations)
	return obj, cli.Patch(context.TODO(), obj, objMerge)
}

// loadImageSettings loads the override values for the image name and version
func loadImageSettings(ctx spi.ComponentContext, overrides *authProxyValues) error {
	// Full image name
	bomFile, err := bom.NewBom(config.GetDefaultBOMFilePath())
	if err != nil {
		return err
	}
	images, err := bomFile.BuildImageOverrides("verrazzano")
	if err != nil {
		return err
	}

	for _, image := range images {
		if image.Key == "api.imageName" {
			overrides.ImageName = image.Value
		} else if image.Key == "api.imageVersion" {
			overrides.ImageVersion = image.Value
		}
	}
	if len(overrides.ImageName) == 0 {
		return ctx.Log().ErrorNewErr("Failed to find api.imageName in BOM")
	}
	if len(overrides.ImageVersion) == 0 {
		return ctx.Log().ErrorNewErr("Failed to find api.imageVersion in BOM")
	}

	return nil
}

// loadKubernetesSettings loads the override values for Kubernetes settings
func loadKubernetesSettings(ctx spi.ComponentContext, overrides *authProxyValues) error {
	effectiveCR := ctx.EffectiveCR()
	authProxyComponent := effectiveCR.Spec.Components.AuthProxy

	if authProxyComponent != nil {
		kubernetesSettings := authProxyComponent.Kubernetes
		if kubernetesSettings != nil {
			// Replicas
			if kubernetesSettings.Replicas > 0 {
				overrides.Replicas = kubernetesSettings.Replicas
			}
			// Affinity
			if kubernetesSettings.Affinity != nil {
				affinityYaml, err := yaml.Marshal(kubernetesSettings.Affinity)
				if err != nil {
					return err
				}
				overrides.Affinity = string(affinityYaml)
			}
		}
	}
	return nil
}

func generateOverridesFile(ctx spi.ComponentContext, overrides interface{}) (string, error) {
	bytes, err := yaml.Marshal(overrides)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(os.TempDir(), tmpFileCreatePattern)
	if err != nil {
		return "", err
	}

	overridesFileName := file.Name()
	if err := writeFileFunc(overridesFileName, bytes, fs.ModeAppend); err != nil {
		return "", err
	}
	ctx.Log().Debugf("Verrazzano install overrides file %s contents: %s", overridesFileName, string(bytes))
	return overridesFileName, nil
}

func getWildcardDNS(vz *vzapi.VerrazzanoSpec) (bool, string) {
	if vz.Components.DNS != nil && vz.Components.DNS.Wildcard != nil {
		return true, vz.Components.DNS.Wildcard.Domain
	}
	return false, ""
}