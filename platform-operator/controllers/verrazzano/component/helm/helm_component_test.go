// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package helm

import (
	"bytes"
	"errors"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"os/exec"
	"strings"
	"testing"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/assert"
	"github.com/verrazzano/verrazzano/pkg/bom"
	"github.com/verrazzano/verrazzano/pkg/helm"
	"github.com/verrazzano/verrazzano/pkg/log/vzlog"
	"github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	"github.com/verrazzano/verrazzano/platform-operator/constants"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/istio"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/config"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8scheme "k8s.io/client-go/kubernetes/scheme"
	clipkg "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Needed for unit tests
var fakeOverrides string

// helmFakeRunner is used to test helm without actually running an OS exec command
type helmFakeRunner struct {
}

const testBomFilePath = "../../testdata/test_bom.json"

// genericHelmTestRunner is used to run generic OS commands with expected results
type genericHelmTestRunner struct {
	stdOut []byte
	stdErr []byte
	err    error
}

var testScheme = runtime.NewScheme()

func init() {
	_ = k8scheme.AddToScheme(testScheme)
	//_ = clientgoscheme.AddToScheme(testScheme)
	_ = v1alpha1.AddToScheme(testScheme)
	_ = certv1.AddToScheme(testScheme)
	// +kubebuilder:scaffold:testScheme
}

// Run genericHelmTestRunner executor
func (r genericHelmTestRunner) Run(cmd *exec.Cmd) (stdout []byte, stderr []byte, err error) {
	return r.stdOut, r.stdErr, r.err
}

// TestGetName tests the component name
// GIVEN a Verrazzano component
//  WHEN I call Name
//  THEN the correct Verrazzano name is returned
func TestGetName(t *testing.T) {
	comp := HelmComponent{
		ReleaseName: "release1",
	}

	a := assert.New(t)
	a.Equal("release1", comp.Name(), "Wrong component name")
}

// TestUpgrade tests the component upgrade
// GIVEN a component
//  WHEN I call Upgrade
//  THEN the upgrade returns success and passes the correct values to the upgrade function
func TestUpgrade(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{
		ReleaseName:             "rancher",
		ChartDir:                "ChartDir",
		ChartNamespace:          "chartNS",
		IgnoreNamespaceOverride: true,
		ValuesFile:              "ValuesFile",
	}

	// This string is built from the Key:Value arrary returned by the bom.buildImageOverrides() function
	fakeOverrides = "rancherImageTag=v2.5.7-20210407205410-1c7b39d0c,rancherImage=ghcr.io/verrazzano/rancher"

	config.SetDefaultBomFilePath(testBomFilePath)
	helm.SetCmdRunner(helmFakeRunner{})
	defer helm.SetDefaultRunner()
	SetUpgradeFunc(fakeUpgrade)
	defer SetDefaultUpgradeFunc()
	helm.SetChartStatusFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartStatusDeployed, nil
	})
	defer helm.SetDefaultChartStatusFunction()
	err := comp.Upgrade(spi.NewFakeContext(newFakeClient(), &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false))
	a.NoError(err, "Upgrade returned an error")
}

func newFakeClient() clipkg.Client {
	client := fake.NewFakeClientWithScheme(k8scheme.Scheme,
		&corev1.Secret{ObjectMeta: v1.ObjectMeta{Name: constants.GlobalImagePullSecName, Namespace: "default"}},
	)
	return client
}

// TestUpgradeIsInstalledUnexpectedError tests the component upgrade
// GIVEN a component
//  WHEN I call Upgrade and the chart status function returns an error
//  THEN the upgrade returns an error
func TestUpgradeIsInstalledUnexpectedError(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{}

	SetUpgradeFunc(func(_ vzlog.VerrazzanoLogger, releaseName string, namespace string, chartDir string, wait bool, dryRun bool, overrides helm.HelmOverrides) (stdout []byte, stderr []byte, err error) {
		return nil, nil, nil
	})
	defer SetDefaultUpgradeFunc()

	helm.SetCmdRunner(genericHelmTestRunner{
		stdOut: []byte(""),
		stdErr: []byte("What happened?"),
		err:    fmt.Errorf("Unexpected error"),
	})
	defer helm.SetDefaultRunner()

	err := comp.Upgrade(spi.NewFakeContext(newFakeClient(), &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false))
	a.Error(err)
}

// TestUpgradeReleaseNotInstalled tests the component upgrade
// GIVEN a component
//  WHEN I call Upgrade and the chart is not installed
//  THEN the upgrade returns no error
func TestUpgradeReleaseNotInstalled(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{}

	SetUpgradeFunc(func(_ vzlog.VerrazzanoLogger, releaseName string, namespace string, chartDir string, wait bool, dryRun bool, overrides helm.HelmOverrides) (stdout []byte, stderr []byte, err error) {
		return nil, nil, nil
	})
	helm.SetCmdRunner(helmFakeRunner{})
	defer helm.SetDefaultRunner()
	config.SetDefaultBomFilePath(testBomFilePath)
	defer config.SetDefaultBomFilePath("")

	err := comp.Upgrade(spi.NewFakeContext(newFakeClient(), &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false))
	a.NoError(err)
}

// TestUpgradeWithEnvOverrides tests the component upgrade
// GIVEN a component
//  WHEN I call Upgrade when the registry and repo overrides are set
//  THEN the upgrade returns success and passes the correct values to the upgrade function
func TestUpgradeWithEnvOverrides(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{
		ReleaseName:             "rancher",
		ChartDir:                "ChartDir",
		ChartNamespace:          "chartNS",
		IgnoreNamespaceOverride: true,
		ValuesFile:              "ValuesFile",
		AppendOverridesFunc:     istio.AppendIstioOverrides,
	}

	_ = os.Setenv(constants.RegistryOverrideEnvVar, "myreg.io")
	defer func() { _ = os.Unsetenv(constants.RegistryOverrideEnvVar) }()

	_ = os.Setenv(constants.ImageRepoOverrideEnvVar, "myrepo")
	defer func() { _ = os.Unsetenv(constants.ImageRepoOverrideEnvVar) }()

	// This string is built from the Key:Value arrary returned by the bom.buildImageOverrides() function
	fakeOverrides = "rancherImageTag=v2.5.7-20210407205410-1c7b39d0c,rancherImage=myreg.io/myrepo/verrazzano/rancher,global.hub=myreg.io/myrepo/verrazzano"

	config.SetDefaultBomFilePath(testBomFilePath)
	helm.SetCmdRunner(helmFakeRunner{})
	defer helm.SetDefaultRunner()
	SetUpgradeFunc(fakeUpgrade)
	defer SetDefaultUpgradeFunc()
	helm.SetChartStatusFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartStatusDeployed, nil
	})
	defer helm.SetDefaultChartStatusFunction()
	err := comp.Upgrade(spi.NewFakeContext(newFakeClient(), &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false))
	a.NoError(err, "Upgrade returned an error")
}

// TestInstall tests the component install
// GIVEN a component
//  WHEN I call Install and the chart is not installed
//  THEN the install runs and returns no error
func TestInstall(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{
		ReleaseName:             "rancher",
		ChartDir:                "ChartDir",
		ChartNamespace:          "chartNS",
		IgnoreNamespaceOverride: true,
		ValuesFile:              "ValuesFile",
	}

	client := fake.NewClientBuilder().WithScheme(k8scheme.Scheme).Build()

	// This string is built from the Key:Value arrary returned by the bom.buildImageOverrides() function
	fakeOverrides = "rancherImageTag=v2.5.7-20210407205410-1c7b39d0c,rancherImage=ghcr.io/verrazzano/rancher"

	config.SetDefaultBomFilePath(testBomFilePath)
	helm.SetCmdRunner(helmFakeRunner{})
	defer helm.SetDefaultRunner()
	SetUpgradeFunc(fakeUpgrade)
	defer SetDefaultUpgradeFunc()
	helm.SetChartStatusFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartNotFound, nil
	})
	defer helm.SetDefaultChartStatusFunction()
	helm.SetChartStateFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartNotFound, nil
	})
	defer helm.SetDefaultChartStateFunction()
	err := comp.Install(spi.NewFakeContext(client, &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false))
	a.NoError(err, "Upgrade returned an error")
}

// TestInstallWithFileOverride tests the component install
// GIVEN a component
//  WHEN I call Install and the chart is not installed and has a custom overrides
//  THEN the overrides struct is populated correctly and there are no errors
func TestInstallWithAllOverride(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{
		ReleaseName:             "rancher",
		ChartDir:                "ChartDir",
		ChartNamespace:          "chartNS",
		IgnoreNamespaceOverride: true,
		ValuesFile:              "ValuesFile",
		AppendOverridesFunc: func(context spi.ComponentContext, releaseName string, namespace string, chartDir string, kvs []bom.KeyValue) ([]bom.KeyValue, error) {
			kvs = append(kvs, bom.KeyValue{Key: "", Value: "my-overrides.yaml", IsFile: true})
			kvs = append(kvs, bom.KeyValue{Key: "setKey", Value: "setValue"})
			kvs = append(kvs, bom.KeyValue{Key: "setStringKey", Value: "setStringValue", SetString: true})
			kvs = append(kvs, bom.KeyValue{Key: "setFileKey", Value: "setFileValue", SetFile: true})
			return kvs, nil
		},
	}

	client := fake.NewClientBuilder().WithScheme(k8scheme.Scheme).Build()

	// This string is built from the Key:Value arrary returned by the bom.buildImageOverrides() function
	fakeOverrides = "rancherImageTag=v2.5.7-20210407205410-1c7b39d0c,rancherImage=ghcr.io/verrazzano/rancher"

	config.SetDefaultBomFilePath(testBomFilePath)
	helm.SetCmdRunner(helmFakeRunner{})
	defer helm.SetDefaultRunner()

	SetUpgradeFunc(func(log vzlog.VerrazzanoLogger, releaseName string, namespace string, chartDir string, wait bool, dryRun bool, overrides helm.HelmOverrides) (stdout []byte, stderr []byte, err error) {
		a.Contains(overrides.FileOverrides, "my-overrides.yaml", "Overrides file not found")
		a.Contains(overrides.SetOverrides, "setKey=setValue", "Incorrect --set overrides")
		a.Contains(overrides.SetStringOverrides, "setStringKey=setStringValue", "Incorrect --set overrides")
		a.Contains(overrides.SetFileOverrides, "setFileKey=setFileValue", "Incorrect --set overrides")
		return fakeUpgrade(log, releaseName, namespace, chartDir, wait, dryRun, overrides)
	})
	defer SetDefaultUpgradeFunc()

	helm.SetChartStatusFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartNotFound, nil
	})
	defer helm.SetDefaultChartStatusFunction()

	helm.SetChartStateFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartNotFound, nil
	})
	defer helm.SetDefaultChartStateFunction()

	err := comp.Install(spi.NewFakeContext(client, &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false))
	a.NoError(err, "Install returned an error")
}

// TestInstallPreviousFailure tests the component install
// GIVEN a component
//  WHEN I call Install and the chart release is in a failed status
//  THEN the chart is uninstalled and then re-installed
func TestInstallPreviousFailure(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{
		ReleaseName:             "rancher",
		ChartDir:                "ChartDir",
		ChartNamespace:          "chartNS",
		IgnoreNamespaceOverride: true,
		ValuesFile:              "ValuesFile",
	}

	client := fake.NewClientBuilder().WithScheme(k8scheme.Scheme).Build()

	// This string is built from the Key:Value arrary returned by the bom.buildImageOverrides() function
	fakeOverrides = "rancherImageTag=v2.5.7-20210407205410-1c7b39d0c,rancherImage=ghcr.io/verrazzano/rancher"

	config.SetDefaultBomFilePath(testBomFilePath)
	helm.SetCmdRunner(helmFakeRunner{})
	defer helm.SetDefaultRunner()
	SetUpgradeFunc(fakeUpgrade)
	defer SetDefaultUpgradeFunc()
	helm.SetChartStatusFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartNotFound, nil
	})
	defer helm.SetDefaultChartStatusFunction()
	helm.SetChartStateFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartStatusFailed, nil
	})
	defer helm.SetDefaultChartStateFunction()
	err := comp.Install(spi.NewFakeContext(client, &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false))
	a.NoError(err, "Upgrade returned an error")
}

// TestInstallWithPreInstallFunc tests the component install
// GIVEN a component
//  WHEN I call Install and the component returns KVs from a preinstall func hook
//  THEN the chart is installed with the additional preInstall helm values
func TestInstallWithPreInstallFunc(t *testing.T) {
	a := assert.New(t)

	preInstallKVPairs := []bom.KeyValue{
		{Key: "preInstall1", Value: "value1"},
		{Key: "preInstall2", Value: "value2"},
	}

	comp := HelmComponent{
		ReleaseName:             "rancher",
		ChartDir:                "ChartDir",
		ChartNamespace:          "chartNS",
		IgnoreNamespaceOverride: true,
		ValuesFile:              "ValuesFile",
		AppendOverridesFunc: func(context spi.ComponentContext, releaseName string, namespace string, chartDir string, kvs []bom.KeyValue) ([]bom.KeyValue, error) {
			return preInstallKVPairs, nil

		},
	}

	client := fake.NewClientBuilder().WithScheme(k8scheme.Scheme).Build()

	// This string is built from the Key:Value arrary returned by the bom.buildImageOverrides() function,
	// plus values returned from the preInstall function if present
	var buffer bytes.Buffer
	buffer.WriteString("rancherImageTag=v2.5.7-20210407205410-1c7b39d0c,rancherImage=ghcr.io/verrazzano/rancher,")
	for i, kv := range preInstallKVPairs {
		buffer.WriteString(kv.Key)
		buffer.WriteString("=")
		buffer.WriteString(kv.Value)
		if i != len(preInstallKVPairs)-1 {
			buffer.WriteString(",")
		}
	}
	expectedOverridesString := buffer.String()

	config.SetDefaultBomFilePath(testBomFilePath)
	helm.SetCmdRunner(helmFakeRunner{})
	defer helm.SetDefaultRunner()
	SetUpgradeFunc(func(_ vzlog.VerrazzanoLogger, releaseName string, namespace string, chartDir string, wait bool, dryRun bool, overrides helm.HelmOverrides) (stdout []byte, stderr []byte, err error) {
		if overrides.SetOverrides != expectedOverridesString {
			return nil, nil, fmt.Errorf("Unexpected overrides string %s, expected %s", overrides, expectedOverridesString)
		}
		return []byte{}, []byte{}, nil
	})
	defer SetDefaultUpgradeFunc()
	helm.SetChartStatusFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartNotFound, nil
	})
	defer helm.SetDefaultChartStatusFunction()
	helm.SetChartStateFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartNotFound, nil
	})
	defer helm.SetDefaultChartStateFunction()
	err := comp.Install(spi.NewFakeContext(client, &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false))
	a.NoError(err, "Upgrade returned an error")
}

// TestOperatorInstallSupported tests IsOperatorInstallSupported
// GIVEN a component
//  WHEN I call IsOperatorInstallSupported
//  THEN the correct Value based on the component definition is returned
func TestOperatorInstallSupported(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{
		SupportsOperatorInstall: true,
	}
	a.True(comp.IsOperatorInstallSupported())
	a.False(HelmComponent{}.IsOperatorInstallSupported())
}

// TestGetDependencies tests GetDependencies
// GIVEN a component
//  WHEN I call GetDependencies
//  THEN the correct Value based on the component definition is returned
func TestGetDependencies(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{
		Dependencies: []string{"comp1", "comp2"},
	}
	a.Equal([]string{"comp1", "comp2"}, comp.GetDependencies())
	a.Nil(HelmComponent{}.GetDependencies())
}

// TestGetDependencies tests IsInstalled
// GIVEN a component
//  WHEN I call GetDependencies
//  THEN true is returned if it the helm release is deployed, false otherwise
func TestIsInstalled(t *testing.T) {
	a := assert.New(t)

	comp := HelmComponent{}
	defer helm.SetDefaultChartStatusFunction()
	client := fake.NewClientBuilder().WithScheme(k8scheme.Scheme).Build()

	helm.SetCmdRunner(genericHelmTestRunner{
		stdOut: []byte(""),
		stdErr: []byte(""),
		err:    nil,
	})
	defer helm.SetDefaultRunner()
	config.SetDefaultBomFilePath(testBomFilePath)
	defer config.SetDefaultBomFilePath("")
	a.True(comp.IsInstalled(spi.NewFakeContext(nil, &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false)))
	helm.SetCmdRunner(genericHelmTestRunner{
		stdOut: []byte(""),
		stdErr: []byte(""),
		err:    fmt.Errorf("Not installed"),
	})
	a.False(comp.IsInstalled(spi.NewFakeContext(client, &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false)))
}

// TestReady tests IsReady
// GIVEN a component
//  WHEN I call IsReady
//  THEN true is returned based on chart status and the status check function if defined for the component
func TestReady(t *testing.T) {
	defer helm.SetDefaultChartStatusFunction()
	defer helm.SetDefaultChartInfoFunction()
	defer helm.SetDefaultReleaseAppVersionFunction()

	tests := []struct {
		name                string
		chartStatusFn       helm.ChartStatusFnType
		chartInfoFn         helm.ChartInfoFnType
		releaseAppVersionFn helm.ReleaseAppVersionFnType
		expectSuccess       bool
	}{
		{
			name: "IsReady when all conditions are met",
			chartStatusFn: func(releaseName string, namespace string) (string, error) {
				return helm.ChartStatusDeployed, nil
			},
			chartInfoFn: func(chartDir string) (helm.ChartInfo, error) {
				return helm.ChartInfo{
					AppVersion: "1.0",
				}, nil
			},
			releaseAppVersionFn: func(releaseName string, namespace string) (string, error) {
				return "1.0", nil
			},
			expectSuccess: true,
		},
		{
			name: "IsReady fail because chart not found",
			chartStatusFn: func(releaseName string, namespace string) (string, error) {
				return helm.ChartNotFound, nil
			},
			chartInfoFn: func(chartDir string) (helm.ChartInfo, error) {
				return helm.ChartInfo{
					AppVersion: "1.0",
				}, nil
			},
			releaseAppVersionFn: func(releaseName string, namespace string) (string, error) {
				return "1.0", nil
			},
			expectSuccess: false,
		},
		{
			name: "IsReady fail because chart status is failure",
			chartStatusFn: func(releaseName string, namespace string) (string, error) {
				return helm.ChartStatusFailed, nil
			},
			chartInfoFn: func(chartDir string) (helm.ChartInfo, error) {
				return helm.ChartInfo{
					AppVersion: "1.0",
				}, nil
			},
			releaseAppVersionFn: func(releaseName string, namespace string) (string, error) {
				return "1.0", nil
			},
			expectSuccess: false,
		},
		{
			name: "IsReady fail because error from getting chart status",
			chartStatusFn: func(releaseName string, namespace string) (string, error) {
				return "", fmt.Errorf("Unexpected error")
			},
			chartInfoFn: func(chartDir string) (helm.ChartInfo, error) {
				return helm.ChartInfo{
					AppVersion: "1.0",
				}, nil
			},
			releaseAppVersionFn: func(releaseName string, namespace string) (string, error) {
				return "1.0", nil
			},
			expectSuccess: false,
		},
		{
			name: "IsReady fail because app version not matched between release and chart",
			chartStatusFn: func(releaseName string, namespace string) (string, error) {
				return helm.ChartStatusDeployed, nil
			},
			chartInfoFn: func(chartDir string) (helm.ChartInfo, error) {
				return helm.ChartInfo{
					AppVersion: "1.1",
				}, nil
			},
			releaseAppVersionFn: func(releaseName string, namespace string) (string, error) {
				return "1.0", nil
			},
			expectSuccess: false,
		},
	}

	a := assert.New(t)
	client := fake.NewClientBuilder().WithScheme(k8scheme.Scheme).Build()
	ctx := spi.NewFakeContext(client, &v1alpha1.Verrazzano{ObjectMeta: v1.ObjectMeta{Namespace: "foo"}}, false)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp := HelmComponent{}
			helm.SetChartStatusFunction(tt.chartStatusFn)
			helm.SetChartInfoFunction(tt.chartInfoFn)
			helm.SetReleaseAppVersionFunction(tt.releaseAppVersionFn)
			if tt.expectSuccess {
				a.True(comp.IsReady(ctx))
			} else {
				a.False(comp.IsReady(ctx))
			}
		})
	}
}

// fakeUpgrade verifies that the correct parameter values are passed to upgrade
func fakeUpgrade(_ vzlog.VerrazzanoLogger, releaseName string, namespace string, chartDir string, _ bool, _ bool, overrides helm.HelmOverrides) (stdout []byte, stderr []byte, err error) {
	if releaseName != "rancher" {
		return []byte("error"), []byte(""), errors.New("Invalid release name")
	}
	if chartDir != "ChartDir" {
		return []byte("error"), []byte(""), errors.New("Invalid chart directory name")
	}
	if namespace != "chartNS" {
		return []byte("error"), []byte(""), errors.New("Invalid chart namespace")
	}

	foundChartOverridesFile := false
	for _, file := range overrides.FileOverrides {
		if file == "ValuesFile" {
			foundChartOverridesFile = true
			break
		}
	}
	if !foundChartOverridesFile {
		return []byte("error"), []byte(""), errors.New("Invalid values file")
	}

	// This string is built from the Key:Value arrary returned by the bom.buildImageOverrides() function
	if !strings.Contains(overrides.SetOverrides, fakeOverrides) {
		return []byte("error"), []byte(""), errors.New("Invalid overrides")
	}
	return []byte("success"), []byte(""), nil
}

// helmFakeRunner overrides the helm run command
func (r helmFakeRunner) Run(_ *exec.Cmd) (stdout []byte, stderr []byte, err error) {
	return []byte("success"), []byte(""), nil
}
