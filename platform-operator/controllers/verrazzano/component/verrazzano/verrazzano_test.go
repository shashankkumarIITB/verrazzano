// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package verrazzano

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/common"
	"github.com/verrazzano/verrazzano/platform-operator/internal/k8s/status"

	vmov1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/apis/vmcontroller/v1"
	"github.com/verrazzano/verrazzano/pkg/bom"
	globalconst "github.com/verrazzano/verrazzano/pkg/constants"
	"github.com/verrazzano/verrazzano/pkg/helm"
	"github.com/verrazzano/verrazzano/pkg/log/vzlog"
	vzclusters "github.com/verrazzano/verrazzano/platform-operator/apis/clusters/v1alpha1"
	vzapi "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	vpoconst "github.com/verrazzano/verrazzano/platform-operator/constants"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/config"

	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	"github.com/stretchr/testify/assert"
	istioclinet "istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioclisec "istio.io/client-go/pkg/apis/security/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

const (
	profileDir      = "../../../../manifests/profiles"
	testBomFilePath = "../../testdata/test_bom.json"
)

var (
	testScheme  = runtime.NewScheme()
	pvc100Gi, _ = resource.ParseQuantity("100Gi")
)

func init() {
	_ = clientgoscheme.AddToScheme(testScheme)
	_ = vmov1.AddToScheme(testScheme)
	_ = vzapi.AddToScheme(testScheme)
	_ = vzclusters.AddToScheme(testScheme)

	_ = istioclinet.AddToScheme(testScheme)
	_ = istioclisec.AddToScheme(testScheme)
	_ = certv1.AddToScheme(testScheme)
	// +kubebuilder:scaffold:testScheme
}

// TestVzResolveNamespace tests the Verrazzano component name
// GIVEN a Verrazzano component
//  WHEN I call resolveNamespace
//  THEN the Verrazzano namespace name is correctly resolved
func TestVzResolveNamespace(t *testing.T) {
	const defNs = vpoconst.VerrazzanoSystemNamespace
	a := assert.New(t)
	ns := resolveVerrazzanoNamespace("")
	a.Equal(defNs, ns, "Wrong namespace resolved for Verrazzano when using empty namespace")
	ns = resolveVerrazzanoNamespace("default")
	a.Equal(defNs, ns, "Wrong namespace resolved for Verrazzano when using default namespace")
	ns = resolveVerrazzanoNamespace("custom")
	a.Equal("custom", ns, "Wrong namespace resolved for Verrazzano when using custom namesapce")
}

// TestFixupFluentdDaemonset tests calls to fixupFluentdDaemonset
func TestFixupFluentdDaemonset(t *testing.T) {
	const defNs = vpoconst.VerrazzanoSystemNamespace
	a := assert.New(t)
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	log := vzlog.DefaultLogger()

	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: defNs,
		},
	}
	err := c.Create(context.TODO(), &ns)
	a.NoError(err)

	// Should return with no error since the fluentd daemonset does not exist.
	// This is valid case when fluentd is not installed.
	err = fixupFluentdDaemonset(log, c, defNs)
	a.NoError(err)

	// Create a fluentd daemonset for test purposes
	daemonSet := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: defNs,
			Name:      globalconst.FluentdDaemonSetName,
		},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "wrong-name",
							Env: []corev1.EnvVar{
								{
									Name:  vpoconst.ClusterNameEnvVar,
									Value: "managed1",
								},
								{
									Name:  vpoconst.ElasticsearchURLEnvVar,
									Value: "some-url",
								},
							},
						},
					},
				},
			},
		},
	}
	err = c.Create(context.TODO(), &daemonSet)
	a.NoError(err)

	// should return error that fluentd container is missing
	err = fixupFluentdDaemonset(log, c, defNs)
	a.Contains(err.Error(), "fluentd container not found in fluentd daemonset: fluentd")

	daemonSet.Spec.Template.Spec.Containers[0].Name = "fluentd"
	err = c.Update(context.TODO(), &daemonSet)
	a.NoError(err)

	// should return no error since the env variables don't need fixing up
	err = fixupFluentdDaemonset(log, c, defNs)
	a.NoError(err)

	// create a secret with needed keys
	data := make(map[string][]byte)
	data[vpoconst.ClusterNameData] = []byte("managed1")
	data[vpoconst.ElasticsearchURLData] = []byte("some-url")
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: defNs,
			Name:      vpoconst.MCRegistrationSecret,
		},
		Data: data,
	}
	err = c.Create(context.TODO(), &secret)
	a.NoError(err)

	// Update env variables to use ValueFrom instead of Value
	clusterNameRef := corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: vpoconst.MCRegistrationSecret,
			},
			Key: vpoconst.ClusterNameData,
		},
	}
	esURLRef := corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: vpoconst.MCRegistrationSecret,
			},
			Key: vpoconst.ElasticsearchURLData,
		},
	}
	daemonSet.Spec.Template.Spec.Containers[0].Env[0].Value = ""
	daemonSet.Spec.Template.Spec.Containers[0].Env[0].ValueFrom = &clusterNameRef
	daemonSet.Spec.Template.Spec.Containers[0].Env[1].Value = ""
	daemonSet.Spec.Template.Spec.Containers[0].Env[1].ValueFrom = &esURLRef
	err = c.Update(context.TODO(), &daemonSet)
	a.NoError(err)

	// should return no error
	err = fixupFluentdDaemonset(log, c, defNs)
	a.NoError(err)

	// env variables should be fixed up to use Value instead of ValueFrom
	fluentdNamespacedName := types.NamespacedName{Name: globalconst.FluentdDaemonSetName, Namespace: defNs}
	updatedDaemonSet := appsv1.DaemonSet{}
	err = c.Get(context.TODO(), fluentdNamespacedName, &updatedDaemonSet)
	a.NoError(err)
	a.Equal("managed1", updatedDaemonSet.Spec.Template.Spec.Containers[0].Env[0].Value)
	a.Nil(updatedDaemonSet.Spec.Template.Spec.Containers[0].Env[0].ValueFrom)
	a.Equal("some-url", updatedDaemonSet.Spec.Template.Spec.Containers[0].Env[1].Value)
	a.Nil(updatedDaemonSet.Spec.Template.Spec.Containers[0].Env[1].ValueFrom)
}

// Test_appendVerrazzanoValues tests the appendVerrazzanoValues function
// GIVEN a call to appendVerrazzanoValues
//  WHEN I call with a ComponentContext with different profiles and overrides
//  THEN the correct KeyValue objects and overrides file snippets are generated
func Test_appendVerrazzanoValues(t *testing.T) {
	falseValue := false
	trueValue := true
	tests := []struct {
		name         string
		description  string
		expectedYAML string
		actualCR     vzapi.Verrazzano
		expectedErr  error
	}{
		{
			name:         "BasicProdVerrazzanoNoOverrides",
			description:  "Test basic prod no user overrides",
			actualCR:     vzapi.Verrazzano{},
			expectedYAML: "testdata/vzValuesProdNoOverrides.yaml",
			expectedErr:  nil,
		},
		{
			name:         "BasicDevVerrazzanoNoOverrides",
			description:  "Test basic dev no user overrides",
			actualCR:     vzapi.Verrazzano{Spec: vzapi.VerrazzanoSpec{Profile: "dev"}},
			expectedYAML: "testdata/vzValuesDevNoOverrides.yaml",
			expectedErr:  nil,
		},
		{
			name:         "BasicManagedClusterVerrazzanoNoOverrides",
			description:  "Test basic managed-cluster no user overrides",
			actualCR:     vzapi.Verrazzano{Spec: vzapi.VerrazzanoSpec{Profile: "managed-cluster"}},
			expectedYAML: "testdata/vzValuesMgdClusterNoOverrides.yaml",
			expectedErr:  nil,
		},
		{
			name:        "DevVerrazzanoWithOverrides",
			description: "Test dev profile with overrides no user overrides",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile:         "dev",
					EnvironmentName: "myenv",
					Components: vzapi.ComponentSpec{
						Console:                &vzapi.ConsoleComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Prometheus:             &vzapi.PrometheusComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Kibana:                 &vzapi.KibanaComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Elasticsearch:          &vzapi.ElasticsearchComponent{Enabled: &falseValue},
						Grafana:                &vzapi.GrafanaComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Keycloak:               &vzapi.KeycloakComponent{Enabled: &falseValue},
						Rancher:                &vzapi.RancherComponent{Enabled: &falseValue},
						DNS:                    &vzapi.DNSComponent{Wildcard: &vzapi.Wildcard{Domain: "xip.io"}},
						PrometheusOperator:     &vzapi.PrometheusOperatorComponent{Enabled: &trueValue},
						PrometheusAdapter:      &vzapi.PrometheusAdapterComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &trueValue}},
						KubeStateMetrics:       &vzapi.KubeStateMetricsComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &trueValue}},
						PrometheusPushgateway:  &vzapi.PrometheusPushgatewayComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &trueValue}},
						PrometheusNodeExporter: &vzapi.PrometheusNodeExporterComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &trueValue}},
						JaegerOperator:         &vzapi.JaegerOperatorComponent{Enabled: &trueValue},
					},
				},
			},
			expectedYAML: "testdata/vzValuesDevWithOverrides.yaml",
			expectedErr:  nil,
		},
		{
			name:        "ProdWithExternaDNSEnabled",
			description: "Test prod with OCI DNS enabled, should enable external-dns component",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Components: vzapi.ComponentSpec{
						DNS: &vzapi.DNSComponent{
							OCI: &vzapi.OCI{
								OCIConfigSecret:        "myOCISecret",
								DNSZoneCompartmentOCID: "myCompartmentOCID",
								DNSZoneOCID:            "myZoneOCID",
								DNSZoneName:            "myzone.com",
							},
						},
					},
				},
			},
			expectedYAML: "testdata/vzValuesProdWithExternalDNS.yaml",
			expectedErr:  nil,
		},
	}
	defer resetWriteFileFunc()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a := assert.New(t)

			t.Log(test.description)

			fakeClient := createFakeClientWithIngress()
			fakeContext := spi.NewFakeContext(fakeClient, &test.actualCR, false, profileDir)
			values := verrazzanoValues{}

			writeFileFunc = func(filename string, data []byte, perm fs.FileMode) error {
				if test.expectedErr != nil {
					return test.expectedErr
				}
				a.Equal([]byte(test.expectedYAML), data)
				return nil
			}

			err := appendVerrazzanoValues(fakeContext, &values)
			if test.expectedErr != nil {
				a.Error(err)
			} else {
				a.NoError(err)
			}

			// outdata, err := yaml.Marshal(&values)
			// assert.NoError(err)
			// ioutil.WriteFile(fmt.Sprintf("%s/%s.yaml", os.TempDir(), test.name), outdata, fs.FileMode(0664))

			data, err := ioutil.ReadFile(test.expectedYAML)
			a.NoError(err, "Error reading expected values yaml file %s", test.expectedYAML)
			expectedValues := verrazzanoValues{}
			err = yaml.Unmarshal(data, &expectedValues)
			a.NoError(err)
			a.Equal(expectedValues, values)
		})
	}
}

// Test_appendVMIValues tests the appendVMIValues function
// GIVEN a call to appendVMIValues
//  WHEN I call with a ComponentContext with different profiles and overrides
//  THEN the correct KeyValue objects and overrides file snippets are generated
func Test_appendVMIValues(t *testing.T) {
	falseValue := false
	tests := []struct {
		name                  string
		description           string
		expectedYAML          string
		actualCR              vzapi.Verrazzano
		expectedHelmOverrides []bom.KeyValue
		expectedErr           error
	}{
		{
			name:                  "VMIProdVerrazzanoNoOverrides",
			description:           "Test VMI basic prod no user overrides",
			actualCR:              vzapi.Verrazzano{},
			expectedYAML:          "testdata/vzValuesVMIProdVerrazzanoNoOverrides.yaml",
			expectedHelmOverrides: []bom.KeyValue{},
			expectedErr:           nil,
		},
		{
			name:                  "VMIDevVerrazzanoNoOverrides",
			description:           "Test VMI basic dev no user overrides",
			actualCR:              vzapi.Verrazzano{Spec: vzapi.VerrazzanoSpec{Profile: "dev"}},
			expectedYAML:          "testdata/vzValuesVMIDevVerrazzanoNoOverrides.yaml",
			expectedHelmOverrides: []bom.KeyValue{},
			expectedErr:           nil,
		},
		{
			name:                  "VMIManagedClusterVerrazzanoNoOverrides",
			description:           "Test VMI basic managed-cluster no user overrides",
			actualCR:              vzapi.Verrazzano{Spec: vzapi.VerrazzanoSpec{Profile: "managed-cluster"}},
			expectedYAML:          "testdata/vzValuesVMIManagedClusterVerrazzanoNoOverrides.yaml",
			expectedHelmOverrides: []bom.KeyValue{},
			expectedErr:           nil,
		},
		{
			name:        "VMIDevWithOverrides",
			description: "Test VMI dev profile with overrides no user overrides",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile: "dev",
					Components: vzapi.ComponentSpec{
						Grafana:       &vzapi.GrafanaComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Elasticsearch: &vzapi.ElasticsearchComponent{Enabled: &falseValue},
						Prometheus:    &vzapi.PrometheusComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Kibana:        &vzapi.KibanaComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
					},
				},
			},
			expectedYAML:          "testdata/vzValuesVMIDevWithOverrides.yaml",
			expectedHelmOverrides: []bom.KeyValue{},
			expectedErr:           nil,
		},
		{
			name:        "VMIDevWithStorageOverrides",
			description: "Test VMI dev profile with overrides no user overrides",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile:             "dev",
					DefaultVolumeSource: &corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "vmi"}},
					VolumeClaimSpecTemplates: []vzapi.VolumeClaimSpecTemplate{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "vmi"},
							Spec: corev1.PersistentVolumeClaimSpec{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										"storage": pvc100Gi,
									},
								},
							},
						},
					},
					Components: vzapi.ComponentSpec{},
				},
			},
			expectedYAML:          "testdata/vzValuesVMIDevWithStorageOverrides.yaml",
			expectedHelmOverrides: []bom.KeyValue{},
			expectedErr:           nil,
		},
		{
			name:        "VMIProdWithStorageOverrides",
			description: "Test VMI prod profile with emptyDir defaultVolumeSource override",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile:             "prod",
					DefaultVolumeSource: &corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
			},
			expectedYAML:          "testdata/vzValuesVMIProdWithStorageOverrides.yaml",
			expectedHelmOverrides: []bom.KeyValue{},
			expectedErr:           nil,
		},
		{
			name:        "VMIProdWithESInstallArgs",
			description: "Test VMI prod profile with emptyDir defaultVolumeSource override",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile:             "prod",
					DefaultVolumeSource: &corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
					Components: vzapi.ComponentSpec{
						Elasticsearch: &vzapi.ElasticsearchComponent{
							ESInstallArgs: []vzapi.InstallArgs{
								{Name: "nodes.master.replicas", Value: "6"},
								{Name: "nodes.master.requests.memory", Value: "3G"},
								{Name: "nodes.ingest.replicas", Value: "8"},
								{Name: "nodes.ingest.requests.memory", Value: "32G"},
								{Name: "nodes.data.replicas", Value: "16"},
								{Name: "nodes.data.requests.memory", Value: "32G"},
							},
						},
					},
				},
			},
			expectedHelmOverrides: []bom.KeyValue{},
			expectedYAML:          "testdata/vzValuesVMIProdWithESInstallArgs.yaml",
			expectedErr:           nil,
		},
	}
	defer resetWriteFileFunc()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a := assert.New(t)

			t.Log(test.description)

			fakeClient := createFakeClientWithIngress()
			fakeContext := spi.NewFakeContext(fakeClient, &test.actualCR, false, profileDir)
			values := verrazzanoValues{}

			writeFileFunc = func(filename string, data []byte, perm fs.FileMode) error {
				if test.expectedErr != nil {
					return test.expectedErr
				}
				a.Equal([]byte(test.expectedYAML), data)
				return nil
			}

			storageOverride, err := common.FindStorageOverride(fakeContext.EffectiveCR())
			a.NoError(err)

			keyValues, err := appendVMIOverrides(fakeContext.EffectiveCR(), &values, storageOverride, []bom.KeyValue{})
			a.NoError(err)
			a.Equal(test.expectedHelmOverrides, keyValues, "Install args did not match")

			data, err := ioutil.ReadFile(test.expectedYAML)
			a.NoError(err, "Error reading expected values yaml file %s", test.expectedYAML)
			expectedValues := verrazzanoValues{}
			err = yaml.Unmarshal(data, &expectedValues)
			a.NoError(err)
			a.Equal(expectedValues, values)
		})
	}
}

// Test_appendVerrazzanoOverrides tests the appendVerrazzanoOverrides function
// GIVEN a call to appendVerrazzanoOverrides
//  WHEN I call with a ComponentContext with different profiles and overrides
//  THEN the correct KeyValue objects and overrides file snippets are generated
func Test_appendVerrazzanoOverrides(t *testing.T) {
	config.SetDefaultBomFilePath(testBomFilePath)
	defer func() {
		config.SetDefaultBomFilePath("")
	}()
	falseValue := false
	trueValue := true
	tests := []struct {
		name         string
		description  string
		expectedYAML string
		actualCR     vzapi.Verrazzano
		expectedErr  error
		numKeyValues int
	}{
		{
			name:         "ProdDefault",
			description:  "Test basic prod profile with no user overrides",
			actualCR:     vzapi.Verrazzano{},
			expectedYAML: "testdata/vzOverridesProdDefault.yaml",
		},
		{
			name:        "ProdDefaultIOError",
			description: "Test basic prod profile with no user overrides",
			actualCR:    vzapi.Verrazzano{},
			expectedErr: fmt.Errorf("Error writing file"),
		},
		{
			name:         "DevDefault",
			description:  "Test basic dev profile with no user overrides",
			actualCR:     vzapi.Verrazzano{Spec: vzapi.VerrazzanoSpec{Profile: "dev"}},
			expectedYAML: "testdata/vzOverridesDevDefault.yaml",
			numKeyValues: 1,
		},
		{
			name:         "ManagedClusterDefault",
			description:  "Test basic managed-cluster no user overrides",
			actualCR:     vzapi.Verrazzano{Spec: vzapi.VerrazzanoSpec{Profile: "managed-cluster"}},
			expectedYAML: "testdata/vzOverridesManagedClusterDefault.yaml",
			numKeyValues: 1,
		},
		{
			name:        "DevWithOverrides",
			description: "Test dev profile with user overrides",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile:         "dev",
					EnvironmentName: "myenv",
					Components: vzapi.ComponentSpec{
						Console:                &vzapi.ConsoleComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Prometheus:             &vzapi.PrometheusComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Kibana:                 &vzapi.KibanaComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Elasticsearch:          &vzapi.ElasticsearchComponent{Enabled: &falseValue},
						Grafana:                &vzapi.GrafanaComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
						Keycloak:               &vzapi.KeycloakComponent{Enabled: &falseValue},
						Rancher:                &vzapi.RancherComponent{Enabled: &falseValue},
						DNS:                    &vzapi.DNSComponent{Wildcard: &vzapi.Wildcard{Domain: "xip.io"}},
						PrometheusOperator:     &vzapi.PrometheusOperatorComponent{Enabled: &trueValue},
						PrometheusAdapter:      &vzapi.PrometheusAdapterComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &trueValue}},
						KubeStateMetrics:       &vzapi.KubeStateMetricsComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &trueValue}},
						PrometheusPushgateway:  &vzapi.PrometheusPushgatewayComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &trueValue}},
						PrometheusNodeExporter: &vzapi.PrometheusNodeExporterComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &trueValue}},
						JaegerOperator:         &vzapi.JaegerOperatorComponent{Enabled: &trueValue},
					},
				},
			},
			expectedYAML: "testdata/vzOverridesDevWithOverrides.yaml",
			numKeyValues: 1,
		},
		{
			name:        "ProdWithExternaDNSEnabled",
			description: "Test prod with OCI DNS enabled, should enable external-dns component",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Components: vzapi.ComponentSpec{
						DNS: &vzapi.DNSComponent{
							OCI: &vzapi.OCI{
								OCIConfigSecret:        "myOCISecret",
								DNSZoneCompartmentOCID: "myCompartmentOCID",
								DNSZoneOCID:            "myZoneOCID",
								DNSZoneName:            "myzone.com",
							},
						},
					},
				},
			},
			expectedYAML: "testdata/vzOverridesProdWithExternaDNSEnabled.yaml",
		},
		{
			name:        "ProdWithAdminRoleOverrides",
			description: "Test prod with Security admin role overrides only",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Security: vzapi.SecuritySpec{
						AdminSubjects: []rbacv1.Subject{
							{
								Kind: "User",
								Name: "kilgore-trout",
							},
							{
								Kind: "User",
								Name: "fred-flintstone",
							},
						},
					},
				},
			},
			expectedYAML: "testdata/vzOverridesProdWithAdminRoleOverrides.yaml",
		},
		{
			name:        "ProdWithMonitorRoleOverrides",
			description: "Test prod with Monitor admin role overrides only",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Security: vzapi.SecuritySpec{
						MonitorSubjects: []rbacv1.Subject{
							{
								Kind: "Group",
								Name: "group-of-monitors",
							},
							{
								Kind: "User",
								Name: "joe-monitor",
							},
						},
					},
				},
			},
			expectedYAML: "testdata/vzOverridesProdWithMonitorRoleOverrides.yaml",
		},
		{
			name:        "ProdWithAdminAndMonitorRoleOverrides",
			description: "Test prod with Security admin and monitor role overrides",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Security: vzapi.SecuritySpec{
						AdminSubjects: []rbacv1.Subject{
							{
								Kind: "User",
								Name: "kilgore-trout",
							},
						},
						MonitorSubjects: []rbacv1.Subject{
							{
								Kind: "Group",
								Name: "group-of-monitors",
							},
						},
					},
				},
			},
			expectedYAML: "testdata/vzOverridesProdWithAdminAndMonitorRoleOverrides.yaml",
		},
		{
			name:        "ProdWithFluentdEmptyExtraVolumeMountsOverrides",
			description: "Test prod with a fluentd override with an empty extra volume mounts field",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile: vzapi.Prod,
					Components: vzapi.ComponentSpec{
						Fluentd: &vzapi.FluentdComponent{
							ExtraVolumeMounts: []vzapi.VolumeMount{},
						},
					},
				},
			},
			expectedYAML: "testdata/vzOverridesProdWithFluentdEmptyExtraVolumeMountsOverrides.yaml",
		},
		{
			name:        "ProdWithFluentdOverrides",
			description: "Test prod with fluentd overrides",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile: vzapi.Prod,
					Components: vzapi.ComponentSpec{
						Fluentd: &vzapi.FluentdComponent{
							ExtraVolumeMounts: []vzapi.VolumeMount{
								{Source: "mysourceDefaults"},
								{Source: "mysourceRO", ReadOnly: &trueValue},
								{Source: "mysourceCustomDestRW", Destination: "mydest", ReadOnly: &falseValue},
							},
							ElasticsearchURL:    "http://myes.mydomain.com:9200",
							ElasticsearchSecret: "custom-elasticsearch-secret",
						},
					},
				},
			},
			expectedYAML: "testdata/vzOverridesProdWithFluentdOverrides.yaml",
		},
		{
			name:        "ProdWithFluentdOCILoggingOverrides",
			description: "Test prod with fluentd OCI Logging overrides",
			actualCR: vzapi.Verrazzano{
				Spec: vzapi.VerrazzanoSpec{
					Profile: vzapi.Prod,
					Components: vzapi.ComponentSpec{
						Fluentd: &vzapi.FluentdComponent{
							OCI: &vzapi.OciLoggingConfiguration{
								SystemLogID:     "ocid1.log.oc1.iad.system-log-ocid",
								DefaultAppLogID: "ocid1.log.oc1.iad.default-app-log-ocid",
							},
						},
					},
				},
			},
			expectedYAML: "testdata/vzOverridesProdWithFluentdOCILoggingOverrides.yaml",
		},
	}
	defer resetWriteFileFunc()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a := assert.New(t)
			t.Log(test.description)

			fakeClient := createFakeClientWithIngress()
			fakeContext := spi.NewFakeContext(fakeClient, &test.actualCR, false, profileDir)

			writeFileFunc = func(filename string, data []byte, perm fs.FileMode) error {
				if test.expectedErr != nil {
					return test.expectedErr
				}
				if err := ioutil.WriteFile(filename, data, perm); err != nil {
					a.Failf("Failure writing file %s: %s", filename, err)
					return err
				}
				a.FileExists(filename)

				// Unmarshal the VZ expected and actual data into verrazzanoValues structs
				// and do a deep-equals comparison using the asserts package

				// Unmarshal the actual generated helm values from code under test
				actualValues := verrazzanoValues{}
				err := yaml.Unmarshal(data, &actualValues)
				a.NoError(err)

				// read in the expected results data from a file and unmarshal it into a values object
				expectedData, err := ioutil.ReadFile(test.expectedYAML)
				a.NoError(err, "Error reading expected values yaml file %s", test.expectedYAML)
				expectedValues := verrazzanoValues{}
				err = yaml.Unmarshal(expectedData, &expectedValues)

				a.NoError(err)
				a.Equal(expectedValues.Logging.ConfigHash, HashSum(fakeContext.EffectiveCR().Spec.Components.Fluentd))
				// Compare the actual and expected values objects
				a.Equal(expectedValues, actualValues)
				a.Equal(HashSum(expectedValues), HashSum(actualValues))
				return nil
			}

			kvs := []bom.KeyValue{}
			kvs, err := appendVerrazzanoOverrides(fakeContext, "", "", "", kvs)
			if test.expectedErr != nil {
				a.Error(err)
				a.Equal([]bom.KeyValue{}, kvs)
				return
			}
			a.NoError(err)

			actualNumKvs := len(kvs)
			expectedNumKvs := test.numKeyValues
			if expectedNumKvs == 0 {
				// default is 1 custom image overrides
				expectedNumKvs = 1
			}
			a.Equal(expectedNumKvs, actualNumKvs)
			// Check Temp file
			a.True(kvs[0].IsFile, "Expected generated verrazzano overrides first in list of helm args")
			tempFilePath := kvs[0].Value
			_, err = os.Stat(tempFilePath)
			a.NoError(err, "Unexpected error checking for temp file %s: %s", tempFilePath, err)
			cleanTempFiles(fakeContext)
		})
	}
	// Verify temp files are deleted
	files, err := ioutil.ReadDir(os.TempDir())
	assert.NoError(t, err, "Error reading temp dir to verify file cleanup")
	for _, file := range files {
		assert.False(t,
			strings.HasPrefix(file.Name(), tmpFilePrefix) && strings.HasSuffix(file.Name(), ".yaml"),
			"Found unexpected temp file remaining: %s", file.Name())
	}

}

// Test_loggingPreInstall tests the Verrazzano loggingPreInstall call
func Test_loggingPreInstall(t *testing.T) {
	// GIVEN a Verrazzano component
	//  WHEN I call loggingPreInstall with fluentd overrides for ES and a custom ES secret
	//  THEN no error is returned and the secret has been copied
	trueValue := true
	secretName := "my-es-secret" //nolint:gosec //#gosec G101
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: vpoconst.VerrazzanoInstallNamespace, Name: secretName},
	}).Build()

	ctx := spi.NewFakeContext(c,
		&vzapi.Verrazzano{
			Spec: vzapi.VerrazzanoSpec{
				Components: vzapi.ComponentSpec{
					Fluentd: &vzapi.FluentdComponent{
						Enabled:             &trueValue,
						ElasticsearchURL:    "https://myes.mydomain.com:9200",
						ElasticsearchSecret: secretName,
					},
				},
			},
		},
		false)
	err := loggingPreInstall(ctx)
	assert.NoError(t, err)

	secret := &corev1.Secret{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: ComponentNamespace}, secret)
	assert.NoError(t, err)

	// GIVEN a Verrazzano component
	//  WHEN I call loggingPreInstall with fluentd overrides for OCI logging, including an OCI API secret name
	//  THEN no error is returned and the secret has been copied
	secretName = "my-oci-api-secret" //nolint:gosec //#gosec G101
	cs := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Namespace: vpoconst.VerrazzanoInstallNamespace, Name: secretName},
		},
	).Build()
	ctx = spi.NewFakeContext(cs,
		&vzapi.Verrazzano{
			Spec: vzapi.VerrazzanoSpec{
				Components: vzapi.ComponentSpec{
					Fluentd: &vzapi.FluentdComponent{
						Enabled: &trueValue,
						OCI: &vzapi.OciLoggingConfiguration{
							APISecret: secretName,
						},
					},
				},
			},
		},
		false)
	err = loggingPreInstall(ctx)
	assert.NoError(t, err)

	err = cs.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: ComponentNamespace}, secret)
	assert.NoError(t, err)
}

// Test_loggingPreInstallSecretNotFound tests the Verrazzano loggingPreInstall call
// GIVEN a Verrazzano component
//  WHEN I call loggingPreInstall with fluentd overrides for ES and a custom ES secret and the secret does not exist
//  THEN an error is returned
func Test_loggingPreInstallSecretNotFound(t *testing.T) {
	trueValue := true
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	ctx := spi.NewFakeContext(c,
		&vzapi.Verrazzano{
			Spec: vzapi.VerrazzanoSpec{
				Components: vzapi.ComponentSpec{
					Fluentd: &vzapi.FluentdComponent{
						Enabled:             &trueValue,
						ElasticsearchURL:    "https://myes.mydomain.com:9200",
						ElasticsearchSecret: "my-es-secret",
					},
				},
			},
		},
		false)
	err := loggingPreInstall(ctx)
	assert.Error(t, err)
}

// Test_loggingPreInstallFluentdNotEnabled tests the Verrazzano loggingPreInstall call
// GIVEN a Verrazzano component
//  WHEN I call loggingPreInstall and fluentd is disabled
//  THEN no error is returned
func Test_loggingPreInstallFluentdNotEnabled(t *testing.T) {
	falseValue := false
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	ctx := spi.NewFakeContext(c,
		&vzapi.Verrazzano{
			Spec: vzapi.VerrazzanoSpec{
				Components: vzapi.ComponentSpec{
					Fluentd: &vzapi.FluentdComponent{
						Enabled: &falseValue,
					},
				},
			},
		},
		false)
	err := loggingPreInstall(ctx)
	assert.NoError(t, err)
}

func createFakeClientWithIngress() client.Client {

	fakeClient := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: vpoconst.NGINXControllerServiceName, Namespace: globalconst.IngressNamespace},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{IP: "11.22.33.44"},
					},
				},
			},
		},
	).Build()
	return fakeClient
}

// TestFakeExecHandler is a test intended to be use to handle fake command execution
// See the fakeExecCommand function.
// When this test is invoked normally no TEST_FAKE_EXEC_SCENARIO is present
// so no assertions are made and therefore passes.
func TestFakeExecHandler(t *testing.T) {
	a := assert.New(t)
	scenario, found := os.LookupEnv("TEST_FAKE_EXEC_SCENARIO")
	if found {
		switch scenario {
		case "fixupElasticSearchReplicaCount/get":
			a.Equal(`curl -v -XGET -s -k --fail http://localhost:42/_cluster/health`,
				os.Args[13], "Expected curl command to be correct.")
			fmt.Print(`"number_of_data_nodes":1,`)
		case "fixupElasticSearchReplicaCount/put":
			fmt.Println(scenario)
			fmt.Println(strings.Join(os.Args, " "))
			a.Equal(`curl -v -XPUT -d '{"index":{"auto_expand_replicas":"0-1"}}' --header 'Content-Type: application/json' -s -k --fail http://localhost:42/verrazzano-*/_settings`,
				os.Args[13], "Expected curl command to be correct.")
		default:
			a.Fail("Unknown test scenario provided in environment variable TEST_FAKE_EXEC_SCENARIO: %s", scenario)
		}
	}
}

// TestAssociateHelmObjectToThisRelease tests labelling/annotating objects that will be imported to a helm chart
// GIVEN an unmanaged object
//  WHEN I call associateHelmObjectToThisRelease
//  THEN the object is managed by helm
func TestAssociateHelmObjectToThisRelease(t *testing.T) {
	namespacedName := types.NamespacedName{
		Name:      ComponentName,
		Namespace: ComponentNamespace,
	}
	obj := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ComponentName,
			Namespace: ComponentNamespace,
		},
	}

	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(obj).Build()
	_, err := associateHelmObjectToThisRelease(c, obj, namespacedName)
	assert.NoError(t, err)
	assert.Equal(t, obj.Annotations["meta.helm.sh/release-name"], ComponentName)
	assert.Equal(t, obj.Annotations["meta.helm.sh/release-namespace"], globalconst.VerrazzanoSystemNamespace)
	assert.Equal(t, obj.Labels["app.kubernetes.io/managed-by"], "Helm")
}

// TestAssociateHelmObjectAndKeep tests labelling/annotating objects that will be associated to a helm chart
// GIVEN an unmanaged object
//  WHEN I call associateHelmObject with keep set to true
//  THEN the object is managed by helm and is labeled with a resource policy of "keep"
func TestAssociateHelmObjectAndKeep(t *testing.T) {
	namespacedName := types.NamespacedName{
		Name:      ComponentName,
		Namespace: ComponentNamespace,
	}
	obj := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ComponentName,
			Namespace: ComponentNamespace,
		},
	}

	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(obj).Build()
	_, err := common.AssociateHelmObject(c, obj, namespacedName, namespacedName, true)
	assert.NoError(t, err)
	assert.Equal(t, ComponentName, obj.Annotations["meta.helm.sh/release-name"])
	assert.Equal(t, globalconst.VerrazzanoSystemNamespace, obj.Annotations["meta.helm.sh/release-namespace"])
	assert.Equal(t, "keep", obj.Annotations["helm.sh/resource-policy"])
	assert.Equal(t, "Helm", obj.Labels["app.kubernetes.io/managed-by"])
}

// TestIsReadySecretNotReady tests the Verrazzano isVerrazzanoReady call
// GIVEN a Verrazzano component
//  WHEN I call isVerrazzanoReady when it is installed and the deployment availability criteria are met, but the secret is not found
//  THEN false is returned
func TestIsReadySecretNotReady(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ComponentNamespace,
				Name:      verrazzanoConsoleDeployment,
				Labels:    map[string]string{"app": verrazzanoConsoleDeployment},
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 1,
				Replicas:          1,
				UpdatedReplicas:   1,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ComponentNamespace,
				Name:      prometheusDeployment,
				Labels:    map[string]string{"app": "system-prometheus"},
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 1,
				Replicas:          1,
				UpdatedReplicas:   1,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: globalconst.VerrazzanoSystemNamespace,
				Name:      fluentDaemonset,
			},
			Status: appsv1.DaemonSetStatus{
				UpdatedNumberScheduled: 1,
				NumberAvailable:        1,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: globalconst.VerrazzanoMonitoringNamespace,
				Name:      nodeExporterDaemonset,
			},
			Status: appsv1.DaemonSetStatus{
				UpdatedNumberScheduled: 1,
				NumberAvailable:        1,
			},
		},
	).Build()
	ctx := spi.NewFakeContext(c, &vzapi.Verrazzano{}, false)
	assert.False(t, checkVerrazzanoComponentStatus(ctx, status.DeploymentsAreReady, status.DaemonSetsAreReady))
}

// TestIsReadyChartNotInstalled tests the Verrazzano isVerrazzanoReady call
// GIVEN a Verrazzano component
//  WHEN I call isVerrazzanoReady when it is not installed
//  THEN false is returned
func TestIsReadyChartNotInstalled(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).Build()
	ctx := spi.NewFakeContext(c, &vzapi.Verrazzano{}, false)
	assert.False(t, checkVerrazzanoComponentStatus(ctx, status.DeploymentsAreReady, status.DaemonSetsAreReady))
}

// TestIsReady tests the Verrazzano isVerrazzanoReady call
// GIVEN Verrazzano components that are all enabled by default
//  WHEN I call isVerrazzanoReady when all requirements are met
//  THEN false is returned
func TestIsReady(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ComponentNamespace,
				Name:      verrazzanoConsoleDeployment,
				Labels:    map[string]string{"app": verrazzanoConsoleDeployment},
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 1,
				Replicas:          1,
				UpdatedReplicas:   1,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ComponentNamespace,
				Name:      prometheusDeployment,
				Labels:    map[string]string{"app": "system-prometheus"},
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 1,
				Replicas:          1,
				UpdatedReplicas:   1,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: globalconst.VerrazzanoSystemNamespace,
				Name:      fluentDaemonset,
			},
			Status: appsv1.DaemonSetStatus{
				UpdatedNumberScheduled: 1,
				NumberAvailable:        1,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: globalconst.VerrazzanoMonitoringNamespace,
				Name:      nodeExporterDaemonset,
			},
			Status: appsv1.DaemonSetStatus{
				UpdatedNumberScheduled: 1,
				NumberAvailable:        1,
			},
		},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "verrazzano",
			Namespace: ComponentNamespace}},
	).Build()

	vz := &vzapi.Verrazzano{}
	vz.Spec.Components = vzapi.ComponentSpec{}
	ctx := spi.NewFakeContext(c, vz, false)
	assert.True(t, checkVerrazzanoComponentStatus(ctx, status.DeploymentsAreReady, status.DaemonSetsAreReady))
}

// TestIsReadyDeploymentNotAvailable tests the Verrazzano isVerrazzanoReady call
// GIVEN a Verrazzano component
//  WHEN I call isVerrazzanoReady when the Verrazzano console deployment is not available
//  THEN false is returned
func TestIsReadyDeploymentNotAvailable(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ComponentNamespace,
				Name:      verrazzanoConsoleDeployment,
				Labels:    map[string]string{"app": verrazzanoConsoleDeployment},
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 1,
				Replicas:          1,
				UpdatedReplicas:   0,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ComponentNamespace,
				Name:      prometheusDeployment,
				Labels:    map[string]string{"app": "system-prometheus"},
			},
			Status: appsv1.DeploymentStatus{
				AvailableReplicas: 1,
				Replicas:          1,
				UpdatedReplicas:   1,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: globalconst.VerrazzanoSystemNamespace,
				Name:      fluentDaemonset,
			},
			Status: appsv1.DaemonSetStatus{
				UpdatedNumberScheduled: 1,
				NumberAvailable:        1,
			},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: globalconst.VerrazzanoMonitoringNamespace,
				Name:      nodeExporterDaemonset,
			},
			Status: appsv1.DaemonSetStatus{
				UpdatedNumberScheduled: 1,
				NumberAvailable:        1,
			},
		},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "verrazzano",
			Namespace: ComponentNamespace}},
	).Build()
	ctx := spi.NewFakeContext(c, &vzapi.Verrazzano{}, false)
	assert.False(t, checkVerrazzanoComponentStatus(ctx, status.DeploymentsAreReady, status.DaemonSetsAreReady))
}

// TestIsReadyDeploymentVMIDisabled tests the Verrazzano isVerrazzanoReady call
// GIVEN a Verrazzano component with all VMI components disabled
//  WHEN I call isVerrazzanoReady
//  THEN true is returned
func TestIsReadyDeploymentVMIDisabled(t *testing.T) {
	helm.SetChartStatusFunction(func(releaseName string, namespace string) (string, error) {
		return helm.ChartStatusDeployed, nil
	})
	defer helm.SetDefaultChartStatusFunction()
	c := fake.NewClientBuilder().WithScheme(testScheme).WithObjects(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "verrazzano",
		Namespace: ComponentNamespace}},
	).Build()
	vz := &vzapi.Verrazzano{}
	falseValue := false
	vz.Spec.Components = vzapi.ComponentSpec{
		Console:       &vzapi.ConsoleComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
		Fluentd:       &vzapi.FluentdComponent{Enabled: &falseValue},
		Kibana:        &vzapi.KibanaComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
		Elasticsearch: &vzapi.ElasticsearchComponent{Enabled: &falseValue},
		Prometheus:    &vzapi.PrometheusComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
		Grafana:       &vzapi.GrafanaComponent{MonitoringComponent: vzapi.MonitoringComponent{Enabled: &falseValue}},
	}
	ctx := spi.NewFakeContext(c, vz, false)
	assert.True(t, checkVerrazzanoComponentStatus(ctx, status.DeploymentsAreReady, status.DaemonSetsAreReady))
}

func TestConfigHashSum(t *testing.T) {
	defaultAppLogID := "test-defaultAppLogId"
	systemLogID := "test-systemLogId"
	apiSec := "test-my-apiSec"
	b := true
	f1 := vzapi.FluentdComponent{
		OCI: &vzapi.OciLoggingConfiguration{DefaultAppLogID: defaultAppLogID,
			SystemLogID: systemLogID, APISecret: apiSec,
		}}
	f2 := vzapi.FluentdComponent{OCI: &vzapi.OciLoggingConfiguration{
		APISecret:       apiSec,
		DefaultAppLogID: defaultAppLogID,
		SystemLogID:     systemLogID,
	}}
	assert.Equal(t, HashSum(f1), HashSum(f2))
	f1.Enabled = &b
	assert.NotEqual(t, HashSum(f1), HashSum(f2))
	f2.Enabled = &b
	assert.Equal(t, HashSum(f1), HashSum(f2))
}
