// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package podworkload

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/verrazzano/verrazzano/pkg/test/framework"
	"github.com/verrazzano/verrazzano/pkg/test/framework/metrics"
	"github.com/verrazzano/verrazzano/tests/e2e/metricsbinding"
	"github.com/verrazzano/verrazzano/tests/e2e/pkg"
)

const (
	longWaitTimeout      = 15 * time.Minute
	longPollingInterval  = 20 * time.Second
	namespace            = "hello-helidon-namespace"
	applicationPodPrefix = "hello-helidon-pod"
	yamlPath             = "application-operator/internal/app/resources/workloads/hello-helidon-pod.yaml"
	promConfigJobName    = "hello-helidon-namespace_hello-helidon-pod_v1_Pod"
)

var t = framework.NewTestFramework("podworkload")

var _ = t.BeforeSuite(func() {
	start := time.Now()
	metricsbinding.DeployApplication(namespace, yamlPath)
	metrics.Emit(t.Metrics.With("deployment_elapsed_time", time.Since(start).Milliseconds()))
})

var clusterDump = pkg.NewClusterDumpWrapper()
var _ = clusterDump.AfterEach(func() {}) // Dump cluster if spec fails
var _ = clusterDump.AfterSuite(func() {  // Dump cluster if aftersuite fails
	metricsbinding.UndeployApplication(namespace, yamlPath, promConfigJobName)
})

var _ = t.AfterEach(func() {})

var _ = t.Describe("Verify application.", Label("f:app-lcm.poko"), func() {
	t.Context("Pod.", func() {
		// GIVEN the app is deployed
		// WHEN the running pods are checked
		// THEN the Helidon pod should exist
		t.It("Verify 'hello-helidon-pod' pod is running", func() {
			Eventually(func() bool {
				return pkg.PodsRunning(namespace, []string{applicationPodPrefix})
			}, longWaitTimeout, longPollingInterval).Should(BeTrue())
		})
	})

	// GIVEN the Helidon app is deployed and the pods are running
	// WHEN the Prometheus metrics in the app namespace are scraped
	// THEN the Helidon application metrics should exist using the default metrics template for pods
	t.Context("Verify Prometheus scraped metrics.", Label("f:observability.monitoring.prom"),
		func() {
			t.It("Retrieve Prometheus scraped metrics for 'hello-helidon-pod' Pod", func() {
				Eventually(func() bool {
					return pkg.MetricsExist("base_jvm_uptime_seconds", "app_verrazzano_io_workload", "hello-helidon-pod-v1-pod")
				}, longWaitTimeout, longPollingInterval).Should(BeTrue(), "Expected to find Prometheus scraped metrics for Helidon application.")
				Eventually(func() bool {
					return pkg.MetricsExist("base_jvm_uptime_seconds", "job", promConfigJobName)
				}, longWaitTimeout, longPollingInterval).Should(BeTrue(), "Expected to find Prometheus scraped metrics for Helidon application.")
			})
		})
})