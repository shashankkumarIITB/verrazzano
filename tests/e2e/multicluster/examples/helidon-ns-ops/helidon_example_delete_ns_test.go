// Copyright (c) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package mcnshelidon

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/verrazzano/verrazzano/tests/e2e/multicluster/examples"
	"github.com/verrazzano/verrazzano/tests/e2e/pkg"
)

const (
	pollingInterval      = 5 * time.Second
	waitTimeout          = 5 * time.Minute
	consistentlyDuration = 1 * time.Minute
	sourceDir            = "hello-helidon-ns"
	testNamespace        = "hello-helidon-ns"
	testProjectName      = "hello-helidon-ns"
)

var adminKubeconfig = os.Getenv("ADMIN_KUBECONFIG")
var managedKubeconfig = os.Getenv("MANAGED_KUBECONFIG")

// failed indicates whether any of the tests has failed
var failed = false

var _ = AfterEach(func() {
	// set failed to true if any of the tests has failed
	failed = failed || CurrentSpecReport().Failed()
})

// set the kubeconfig to use the admin cluster kubeconfig and deploy the example resources
var _ = BeforeSuite(func() {
	// deploy the VerrazzanoProject
	Eventually(func() error {
		return examples.DeployHelloHelidonProject(adminKubeconfig, sourceDir)
	}, waitTimeout, pollingInterval).ShouldNot(HaveOccurred())

	// wait for the namespace to be created on the cluster before deploying app
	Eventually(func() bool {
		return examples.HelidonNamespaceExists(adminKubeconfig, sourceDir)
	}, waitTimeout, pollingInterval).Should(BeTrue())

	Eventually(func() error {
		return examples.DeployHelloHelidonApp(adminKubeconfig, sourceDir)
	}, waitTimeout, pollingInterval).ShouldNot(HaveOccurred())
})

var _ = Describe("Multi-cluster verify delete ns of hello-helidon-ns", func() {
	Context("Admin Cluster", func() {
		// GIVEN an admin cluster and at least one managed cluster
		// WHEN the example application has been deployed to the admin cluster
		// THEN expect that the multi-cluster resources have been created on the admin cluster
		It("Has multi cluster resources", func() {
			Eventually(func() bool {
				return examples.VerifyMCResources(adminKubeconfig, true, false, testNamespace)
			}, waitTimeout, pollingInterval).Should(BeTrue())
		})
		// GIVEN an admin cluster
		// WHEN the multi-cluster example application has been created on admin cluster but not placed there
		// THEN expect that the app is not deployed to the admin cluster consistently for some length of time
		It("Does not have application placed", func() {
			Consistently(func() bool {
				return examples.VerifyHelloHelidonInCluster(adminKubeconfig, true, false, testProjectName, testNamespace)
			}, consistentlyDuration, pollingInterval).Should(BeTrue())
		})
	})

	Context("Managed Cluster", func() {
		// GIVEN an admin cluster and at least one managed cluster
		// WHEN the example application has been deployed to the admin cluster
		// THEN expect that the multi-cluster resources have been created on the managed cluster
		It("Has multi cluster resources", func() {
			Eventually(func() bool {
				return examples.VerifyMCResources(managedKubeconfig, false, true, testNamespace)
			}, waitTimeout, pollingInterval).Should(BeTrue())
		})
		// GIVEN an admin cluster and at least one managed cluster
		// WHEN the multi-cluster example application has been created on admin cluster and placed in managed cluster
		// THEN expect that the app is deployed to the managed cluster
		It("Has application placed", func() {
			Eventually(func() bool {
				return examples.VerifyHelloHelidonInCluster(managedKubeconfig, false, true, testProjectName, testNamespace)
			}, waitTimeout, pollingInterval).Should(BeTrue())
		})
	})

	Context("Delete resources", func() {
		It("Delete project on admin cluster", func() {
			Eventually(func() error {
				return deleteProject(adminKubeconfig)
			}, waitTimeout, pollingInterval).ShouldNot(HaveOccurred())
		})

		It("Delete test namespace on managed cluster", func() {
			Eventually(func() error {
				return pkg.DeleteNamespaceInCluster(testNamespace, managedKubeconfig)
			}, waitTimeout, pollingInterval).ShouldNot(HaveOccurred())
		})

		It("Verify deletion on managed cluster", func() {
			Eventually(func() bool {
				return examples.VerifyHelloHelidonDeletedInManagedCluster(managedKubeconfig, testNamespace, testProjectName)
			}, waitTimeout, pollingInterval).Should(BeTrue())
		})

		It("Delete test namespace on admin cluster", func() {
			Eventually(func() error {
				return pkg.DeleteNamespaceInCluster(testNamespace, adminKubeconfig)
			}, waitTimeout, pollingInterval).ShouldNot(HaveOccurred())
		})

		It("Verify deletion on admin cluster", func() {
			Eventually(func() bool {
				return examples.VerifyHelloHelidonDeletedAdminCluster(adminKubeconfig, false, testNamespace, testProjectName)
			}, waitTimeout, pollingInterval).Should(BeTrue())
		})

	})
})

var _ = AfterSuite(func() {
	if failed {
		pkg.ExecuteClusterDumpWithEnvVarConfig()
	}
})

func deleteProject(kubeconfigPath string) error {
	if err := pkg.DeleteResourceFromFileInCluster("examples/multicluster/hello-helidon-ns/verrazzano-project.yaml", kubeconfigPath); err != nil {
		return fmt.Errorf("failed to delete hello-helidon project resource: %v", err)
	}
	return nil
}
