// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package dashboards

import (
	"bufio"
	"fmt"
	"github.com/verrazzano/verrazzano/pkg/k8sutil"
	"github.com/verrazzano/verrazzano/tests/e2e/update"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/verrazzano/verrazzano/pkg/test/framework"
	"github.com/verrazzano/verrazzano/tests/e2e/pkg"
)

const (
	threeMinutes                = 3 * time.Minute
	pollingInterval             = 10 * time.Second
	longTimeout                 = 10 * time.Minute
	oldPatternsTestDataFile     = "testdata/upgrade/opensearch-dashboards/old-index-patterns.txt"
	updatedPatternsTestDataFile = "testdata/upgrade/opensearch-dashboards/updated-index-patterns.txt"
)

var t = framework.NewTestFramework("opensearch-dashboards")

var _ = t.BeforeSuite(func() {
	kubeconfigPath, err := k8sutil.GetKubeConfigLocation()
	if err != nil {
		return
	}
	supported, err := pkg.IsVerrazzanoMinVersion("1.3.0", kubeconfigPath)
	if err != nil {
		return
	}
	if supported {
		m := pkg.ElasticSearchISMPolicyAddModifier{}
		update.UpdateCR(m)
	}
	pkg.WaitForISMPolicyUpdate(pollingInterval, longTimeout)
})

var _ = t.AfterSuite(func() {
	m := pkg.ElasticSearchISMPolicyRemoveModifier{}
	update.UpdateCR(m)
})

var _ = t.Describe("Pre Upgrade OpenSearch Dashboards Setup", Label("f:observability.logging.kibana"), func() {
	// It Wrapper to only run spec if component is supported on the current Verrazzano installation
	MinimumVerrazzanoIt := func(description string, f func()) {
		kubeconfigPath, err := k8sutil.GetKubeConfigLocation()
		if err != nil {
			t.It(description, func() {
				Fail(fmt.Sprintf("Failed to get default kubeconfig path: %s", err.Error()))
			})
		}
		supported, err := pkg.IsVerrazzanoMinVersion("1.3.0", kubeconfigPath)
		if err != nil {
			t.It(description, func() {
				Fail(err.Error())
			})
		}
		// Only run tests if Verrazzano is at least version 1.3.0
		if supported {
			t.It(description, f)
		} else {
			pkg.Log(pkg.Info, fmt.Sprintf("Skipping check '%v', Verrazzano is not at version 1.3.0", description))
		}
	}
	// GIVEN the OpenSearchDashboards pod
	// WHEN the index patterns are created
	// THEN verify that they are created successfully
	MinimumVerrazzanoIt("Create Index Patterns", func() {
		Eventually(func() bool {
			kubeConfigPath, _ := k8sutil.GetKubeConfigLocation()
			if pkg.IsOpenSearchDashboardsEnabled(kubeConfigPath) {
				isVersionAbove1_3_0, err := pkg.IsVerrazzanoMinVersion("1.3.0", kubeConfigPath)
				if err != nil {
					pkg.Log(pkg.Error, fmt.Sprintf("failed to find the verrazzano version: %v", err))
					return false
				}
				patternFile := oldPatternsTestDataFile
				if isVersionAbove1_3_0 {
					patternFile = updatedPatternsTestDataFile
				}
				file, err := pkg.FindTestDataFile(patternFile)
				if err != nil {
					pkg.Log(pkg.Error, fmt.Sprintf("failed to find test data file: %v", err))
					return false
				}
				found, err := os.Open(file)
				if err != nil {
					pkg.Log(pkg.Error, fmt.Sprintf("failed to open test data file: %v", err))
					return false
				}
				reader := bufio.NewScanner(found)
				reader.Split(bufio.ScanLines)
				defer found.Close()

				for reader.Scan() {
					line := strings.TrimSpace(reader.Text())
					// skip empty lines
					if len(line) == 0 {
						continue
					}
					// ignore lines starting with "#"
					if strings.HasPrefix(line, "#") {
						continue
					}
					// Create index pattern
					result := pkg.CreateIndexPattern(line)
					if result != nil {
						pkg.Log(pkg.Error, fmt.Sprintf("failed to create index pattern %s: %v", line, err))
						return false
					}
				}
			}
			return true
		}).WithPolling(pollingInterval).WithTimeout(threeMinutes).Should(BeTrue(), "Expected not to fail creation of index patterns")
	})
})
