// Copyright (c) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

// Package cluster handles cluster analysis
package cluster

import (
	encjson "encoding/json"
	installv1alpha1 "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	"github.com/verrazzano/verrazzano/tools/analysis/internal/util/files"
	"github.com/verrazzano/verrazzano/tools/analysis/internal/util/report"
	"go.uber.org/zap"
	"io/ioutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"os"
	"regexp"
	"strings"
)

// TODO: Helpers to access this info as needed

// allNamespacesFound is a list of the namespaces found
var allNamespacesFound []string

// verrazzanoNamespacesFound is a list of the Verrazzano namespaces found
var verrazzanoNamespacesFound []string

// Pattern matchers
var verrazzanoInstallJobPodMatcher = regexp.MustCompile("verrazzano-install-.*")
var verrazzanoUninstallJobPodMatcher = regexp.MustCompile("verrazzano-uninstall-.*")

// TODO: CRDs related to verrazzano
// TODO: Can we determine the underlying platform that is being used? This may generally help in terms
//       of the analysis (ie: message formatting), but it also is generally useful in terms of how we
//       provide action advice as well. Inspecting the nodes.json seems like the a good place to determine this

// verrazzanoDeployments related to verrazzano
var verrazzanoDeployments = make(map[string]appsv1.Deployment)
var problematicVerrazzanoDeploymentNames = make([]string, 0)
var componentsNotInReadyState = make([]string, 0)

const verrazzanoResource = "verrazzano_resources.json"

var verrazzanoAnalysisFunctions = map[string]func(log *zap.SugaredLogger, clusterRoot string, issueReporter *report.IssueReporter) (err error){
	"Installation status": installationStatus,
}

// AnalyzeVerrazzano handles high level checking for Verrazzano itself. Note that we are not necessarily going to drill deeply here and
// we may actually handle scenarios as part of the other drill-downs separately
func AnalyzeVerrazzano(log *zap.SugaredLogger, clusterRoot string) (err error) {
	log.Debugf("AnalyzeVerrazzano called for %s", clusterRoot)

	var issueReporter = report.IssueReporter{
		PendingIssues: make(map[string]report.Issue),
	}

	// Call the Verrazzano analysis functions
	for functionName, function := range verrazzanoAnalysisFunctions {
		err := function(log, clusterRoot, &issueReporter)
		if err != nil {
			// Log the error and continue on
			log.Errorf("Error processing analysis function %s", functionName, err)
		}
	}
	issueReporter.Contribute(log, clusterRoot)
	return nil
}

// Determine the state of the Verrazzano Installation
func installationStatus(log *zap.SugaredLogger, clusterRoot string, issueReporter *report.IssueReporter) (err error) {
	// TODO: Is verrazzano:
	//      installed, installed-but-not-running, uninstalled-success-no-cruft, failed-install, failed-uninstall,
	//      uninstall-success-but-cruft-remaining, etc...
	// The intention is that we should at least give an Informational on what the state is.

	isInstallComplete, err := isInstallComplete(log, clusterRoot)
	if err != nil {
		return err
	}
	if !isInstallComplete {
		messages := make(StringSlice, 1)
		messages[0] = "Verrazzano installation is not complete for the following components:"
		for _, comp := range componentsNotInReadyState {
			messages = append(messages, "\t " + comp)
		}
		files := make(StringSlice, 1)
		files[0] = clusterRoot + "/" + verrazzanoResource
		issueReporter.AddKnownIssueMessagesFiles(report.InstallFailureCompNotReady, clusterRoot, messages, files)
	}

	// Enumerate the namespaces that we found overall and the Verrazzano specific ones separately
	// Also look at the deployments in the Verrazzano related namespaces
	allNamespacesFound, err = files.FindNamespaces(log, clusterRoot)
	if err != nil {
		return err
	}

	for _, namespace := range allNamespacesFound {
		// These are Verrazzano owned namespaces
		if strings.Contains(namespace, "verrazzano") {
			verrazzanoNamespacesFound = append(verrazzanoNamespacesFound, namespace)
			deploymentList, err := GetDeploymentList(log, files.FindFileInNamespace(clusterRoot, namespace, "deployments.json"))
			if err != nil {
				// Log the error and continue on
				log.Debugf("Error getting deployments in %s", namespace, err)
			}
			if deploymentList != nil && len(deploymentList.Items) > 0 {
				for i, deployment := range deploymentList.Items {
					verrazzanoDeployments[deployment.ObjectMeta.Name] = deployment
					if IsDeploymentProblematic(&deploymentList.Items[i]) {
						problematicVerrazzanoDeploymentNames = append(problematicVerrazzanoDeploymentNames, deployment.ObjectMeta.Name)
					}
				}
			}
		}

		// TBD: For now not enumerating out potentially related namespaces that could be here even
		// without Verrazzano (cattle, keycloak, etc...). Those will still be in the AllNamespacesFound if present
		// so until there is an explicit need to separate those, not doing that here (we could though)
	}

	// TODO: Inspect the verrazzano-install namespace platform operator logs. We should be able to glean state from the
	//       the logs here, and what the name of the install job resource to look for is.
	// TODO: Inspect the default namespace for a Verrazzano install job pod logs. Inspecting the logs should here should
	//       tell us whether an install/uninstall was done and what state it thinks it is in. NOTE, a user can name this
	//       how they want, so use the resource gleaned above on what to look for here.
	// TODO: Inspect the verrazzano-system namespace. The deployments/status here will tell us what we need to fan out
	//       and drill into
	// TODO: Inspect the verrazzano-mc namespace (TBD)

	// TODO: verrazzanoApiResourceMatches := files.SearchFile(log, files.FindFileInCluster(cluserRoot, "api_resources.out"), ".*verrazzano.*")
	// TODO: verrazzanoResources (json file)
	// What are we doing with problematicVerrazzanoDeploymentNames ?

	return nil
}

func isInstallComplete(log *zap.SugaredLogger, clusterRoot string) (bool, error) {
	vzResourcesPath := files.FindFileInClusterRoot(clusterRoot, verrazzanoResource)
	fileInfo, e := os.Stat(vzResourcesPath)
	if e != nil || fileInfo.Size() == 0 {
		log.Infof("Verrazzano resource file %s is either empty or there is an issue in getting the file info about it", vzResourcesPath)
		return false, e
	}

	file, err := os.Open(vzResourcesPath)
	if err != nil {
		log.Infof("file %s not found", vzResourcesPath)
		return false, err
	}
	defer file.Close()
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Infof("Failed reading Json file %s", vzResourcesPath)
		return false, err
	}

	log.Infof("Length in bytes of the file %s",  len(fileBytes))
	var vzResourceList installv1alpha1.VerrazzanoList
	err = encjson.Unmarshal(fileBytes, &vzResourceList)
	if err != nil {
		log.Infof("Failed to unmarshal Verrazzano resource at %s", vzResourcesPath)
		return false, err
	}

	if len(vzResourceList.Items) > 0 {
		log.Infof("Size %s", len(vzResourceList.Items))
		// There should be only one Verrazzano resource, so the first item from the list should be good enough
		for _, vzRes := range vzResourceList.Items {
			if vzRes.Status.State != installv1alpha1.VzStateReady {
				log.Infof("Installation is not good, installation state %s", vzRes.Status.State)

				// Verrazzano installation is not complete, find out the list of components which are not ready
				for _, compStatusDetail := range vzRes.Status.Components {
					if compStatusDetail.State != installv1alpha1.CompStateReady {
						if compStatusDetail.State == installv1alpha1.CompStateDisabled {
							continue
						}
						// Create the list of components which did not reach Ready state, so that we can look for the errors for the component in the platform operator log,
						// and other artifacts for the component
						componentsNotInReadyState = append(componentsNotInReadyState, compStatusDetail.Name)
					}
				}
				return false, nil
			}
		}
	}
	return true, nil
}

// TODO: Don't need below functions, delete
// IsVerrazzanoInstallJobPod returns true if the pod is an install job related pod for Verrazzano
func IsVerrazzanoInstallJobPod(pod corev1.Pod) bool {
	return verrazzanoInstallJobPodMatcher.MatchString(pod.ObjectMeta.Name) && (pod.ObjectMeta.Namespace == "verrazzano-install" || pod.ObjectMeta.Namespace == "default")
}

// IsVerrazzanoUninstallJobPod returns true if the pod is an uninstall job related pod for Verrazzano
func IsVerrazzanoUninstallJobPod(pod corev1.Pod) bool {
	return verrazzanoUninstallJobPodMatcher.MatchString(pod.ObjectMeta.Name) && (pod.ObjectMeta.Namespace == "verrazzano-install" || pod.ObjectMeta.Namespace == "default")
}

