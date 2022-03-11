// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package verrazzano

import (
	"context"
	"github.com/stretchr/testify/assert"
	vmov1 "github.com/verrazzano/verrazzano-monitoring-operator/pkg/apis/vmcontroller/v1"
	globalconst "github.com/verrazzano/verrazzano/pkg/constants"
	vzapi "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

var enabled = true

var monitoringComponent = vzapi.MonitoringComponent{
	Enabled: &enabled,
}

var vmiEnabledCR = vzapi.Verrazzano{
	Spec: vzapi.VerrazzanoSpec{
		Profile: vzapi.Prod,
		Components: vzapi.ComponentSpec{
			DNS: dnsComponents.DNS,
			Kibana: &vzapi.KibanaComponent{
				MonitoringComponent: monitoringComponent,
			},
			Prometheus: &vzapi.PrometheusComponent{
				MonitoringComponent: monitoringComponent,
			},
			Grafana: &vzapi.GrafanaComponent{
				MonitoringComponent: monitoringComponent,
			},
			Elasticsearch: &vzapi.ElasticsearchComponent{

				ESInstallArgs: []vzapi.InstallArgs{
					{
						Name:  "nodes.master.replicas",
						Value: "1",
					},
					{
						Name:  "nodes.master.requests.memory",
						Value: "1G",
					},
					{
						Name:  "nodes.ingest.replicas",
						Value: "2",
					},
					{
						Name:  "nodes.ingest.requests.memory",
						Value: "2G",
					},
					{
						Name:  "nodes.data.replicas",
						Value: "3",
					},
					{
						Name:  "nodes.data.requests.memory",
						Value: "3G",
					},
					{
						Name:  "nodes.data.requests.storage",
						Value: "100Gi",
					},
				},
			},
		},
	},
}

// TestNewVMIResources tests that new VMI resources can be created from a CR
// GIVEN a Verrazzano CR
//  WHEN I create new VMI resources
//  THEN the configuration in the CR is respected
func TestNewVMIResources(t *testing.T) {
	r := &resourceRequestValues{
		Memory:  "",
		Storage: "50Gi",
	}
	grafana := newGrafana(&vmiEnabledCR, r, nil)
	assert.Equal(t, "48Mi", grafana.Resources.RequestMemory)
	assert.Equal(t, "50Gi", grafana.Storage.Size)

	prometheus := newPrometheus(&vmiEnabledCR, r, nil)
	assert.Equal(t, "128Mi", prometheus.Resources.RequestMemory)
	assert.Equal(t, "50Gi", prometheus.Storage.Size)

	opensearch, err := newOpenSearch(&vmiEnabledCR, r, nil)
	assert.NoError(t, err)
	assert.EqualValues(t, 1, opensearch.MasterNode.Replicas)
	assert.EqualValues(t, 2, opensearch.IngestNode.Replicas)
	assert.EqualValues(t, 3, opensearch.DataNode.Replicas)
	assert.Equal(t, "100Gi", opensearch.Storage.Size)

	opensearchDashboards := newOpenSearchDashboards(&vmiEnabledCR)
	assert.Equal(t, "192Mi", opensearchDashboards.Resources.RequestMemory)
}

// TestOpenSearchInvalidArgs tests trying to create an opensearch resource with invalid args
// GIVEN a Verrazzano CR with invalid install args
//  WHEN I create a new opensearch resource
//  THEN the opensearch resource fails to create
func TestOpenSearchInvalidArgs(t *testing.T) {
	r := &resourceRequestValues{}
	crBadArgs := &vzapi.Verrazzano{
		Spec: vzapi.VerrazzanoSpec{
			Components: vzapi.ComponentSpec{
				Elasticsearch: &vzapi.ElasticsearchComponent{
					ESInstallArgs: []vzapi.InstallArgs{
						{
							Name:  "nodes.master.replicas",
							Value: "foobar!",
						},
					},
				},
			},
		},
	}

	_, err := newOpenSearch(crBadArgs, r, nil)
	assert.Error(t, err)
}

// TestNewGrafanaWithExistingVMI tests that storage values in the VMI are not erased when a new Grafana is created
// GIVEN a Verrazzano CR and an existing VMO
//  WHEN I create a new Grafana resource
//  THEN the storage options from the existing VMO are preserved.
func TestNewGrafanaWithExistingVMI(t *testing.T) {
	existingVmo := vmov1.VerrazzanoMonitoringInstance{
		Spec: vmov1.VerrazzanoMonitoringInstanceSpec{
			Grafana: vmov1.Grafana{
				Enabled: true,
				Storage: vmov1.Storage{
					Size: "100Gi",
					PvcNames: []string{
						"my-pvc",
					},
				},
			},
		},
	}

	grafana := newGrafana(&vmiEnabledCR, nil, &existingVmo)
	assert.NotNil(t, grafana)
	assert.Equal(t, "100Gi", grafana.Storage.Size)
	assert.Equal(t, []string{"my-pvc"}, grafana.Storage.PvcNames)
}

// TestNewPrometheusWithDefaultStorage tests that the default storage of Prometheus is 50Gi
// GIVEN a Verrazzano CR
// WHEN I create a new Prometheus resource
//  THEN the storage is 50Gi
func TestNewPrometheusWithDefaultStorage(t *testing.T) {
	prometheus := newPrometheus(&vmiEnabledCR, nil, nil)
	assert.Equal(t, "50Gi", prometheus.Storage.Size)
}

// TestPrometheusWithStorageOverride tests that storage overrides are applied to Prometheus
// GIVEN a Verrazzano CR and a storage override of 100Gi
// WHEN I create a new Prometheus resource
//  THEN the storage is 100Gi
func TestPrometheusWithStorageOverride(t *testing.T) {
	prometheus := newPrometheus(&vmiEnabledCR, &resourceRequestValues{Storage: "100Gi"}, nil)
	assert.Equal(t, "100Gi", prometheus.Storage.Size)
}

// TestCreateVMI tests a new VMI resources is created in K8s according to the CR
// GIVEN a Verrazzano CR
// WHEN I create a new VMI resource
//  THEN the configuration in the CR is respected
func TestCreateVMI(t *testing.T) {
	ctx := spi.NewFakeContext(fake.NewFakeClientWithScheme(testScheme), &vmiEnabledCR, false)
	err := createVMI(ctx)
	assert.NoError(t, err)
	vmi := &vmov1.VerrazzanoMonitoringInstance{}
	namespacedName := types.NamespacedName{Name: system, Namespace: globalconst.VerrazzanoSystemNamespace}
	err = ctx.Client().Get(context.TODO(), namespacedName, vmi)
	assert.NoError(t, err)
	assert.Equal(t, "100Gi", vmi.Spec.Elasticsearch.Storage.Size)
	assert.EqualValues(t, 2, vmi.Spec.Elasticsearch.IngestNode.Replicas)
	assert.EqualValues(t, 1, vmi.Spec.Elasticsearch.MasterNode.Replicas)
	assert.EqualValues(t, 3, vmi.Spec.Elasticsearch.DataNode.Replicas)
}