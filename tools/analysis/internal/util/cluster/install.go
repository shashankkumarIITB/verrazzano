// Copyright (c) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

// Package cluster handles cluster analysis
package cluster

import (
	encjson "encoding/json"
	"fmt"
	installv1alpha1 "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"
	"github.com/verrazzano/verrazzano/tools/analysis/internal/util/files"
	"github.com/verrazzano/verrazzano/tools/analysis/internal/util/report"
	"go.uber.org/zap"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"os"
	"regexp"
	"strings"
)

// Compiled Regular expressions
var installNGINXIngressControllerFailedRe = regexp.MustCompile(`Failed getting DNS suffix: No IP found for service ingress-controller-ingress-nginx-controller with type LoadBalancer`)
var installFailedRe = regexp.MustCompile(`Install.*\[FAILED\]`)

// I'm going with a more general pattern for limit reached as the supporting details should give the precise message
// and the advice can be to refer to the supporting details on the limit that was exceeded. We can change it up
// if we need a more precise match
var ephemeralIPLimitReachedRe = regexp.MustCompile(`.*Limit for non-ephemeral regional public IP per tenant of .* has been already reached`)
var lbServiceLimitReachedRe = regexp.MustCompile(`.*The following service limits were exceeded: lb-.*`)
var failedToEnsureLoadBalancer = regexp.MustCompile(`.*failed to ensure load balancer: awaiting load balancer.*`)

const verrazzanoResource = "verrazzano_resources.json"
const installErrorNotFound = "No component specific error found in the install log"
var componentErrorPattern = `\"level\":\"error\".*\"component\":\"vzcomponent\"`

type LogMessage struct {
	Level             string `json:"level"`
	Timestamp         string `json:"@timestamp,omitempty"`
	Caller            string `json:"caller,omitempty"`
	Message           string `json:"message"`
	ResourceNameSpace string `json:"resource_namespace,omitempty"`
	ResourceName      string `json:"resource_name,omitempty"`
	Controller        string `json:"controller,omitempty"`
	Component         string `json:"component,omitempty"`
	Operation         string `json:"operation,omitempty"`
	Stacktrace        string `json:"stacktrace,omitempty"`
}


const (
	// Service name
	ingressControllerService = "ingress-controller-ingress-nginx-controller"

	// Function names
	nginxIngressControllerFailed = "nginxIngressControllerFailed"
)

var dispatchMatchMap = map[string]*regexp.Regexp{
	nginxIngressControllerFailed: installNGINXIngressControllerFailedRe,
}

var dispatchFunctions = map[string]func(log *zap.SugaredLogger, clusterRoot string, podFile string, pod corev1.Pod, issueReporter *report.IssueReporter) (err error){
	nginxIngressControllerFailed: analyzeNGINXIngressController,
}

func AnalyzeVerrazzanoInstallIssue(log *zap.SugaredLogger, clusterRoot string, issueReporter *report.IssueReporter) (err error) {
	isInstallComplete, err := isInstallComplete(log, clusterRoot)
	if err != nil {
		return err
	}

	if !isInstallComplete {
		reportInstallError(log, clusterRoot, issueReporter)
	}
	return nil
}

// AnalyzeVerrazzanoInstallIssue is called when we have reason to believe that the installation has failed

/*func AnalyzeVerrazzanoInstallIssues(log *zap.SugaredLogger, clusterRoot string, podFile string, pod corev1.Pod, issueReporter *report.IssueReporter) (err error) {
	log.Infof(">>>> Inside AnalyzeVerrazzanoInstallIssue with podFile %s and pod %s", podFile, pod.Name)
	// Skip if it is not the Verrazzano install job pod
	if !IsVerrazzanoInstallJobPod(pod) {
		return nil
	}

	log.Debugf("verrazzanoInstallIssues analysis called for cluster: %s, ns: %s, pod: %s", clusterRoot, pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
	// TODO: Not correlating time here yet
	if IsContainerNotReady(pod.Status.Conditions) {
		// The install job pod log is currently the only place we can determine where the install process failed at, so we
		// scrape those log messages out.
		logMatches, err := files.SearchFile(log, files.FindPodLogFileName(clusterRoot, pod), installFailedRe, nil)
		if err == nil {
			// We likely will only have a single failure message here (we may only want to look at the last one for install failures)
			for _, matched := range logMatches {
				log.Debugf("Install failure message: %s", matched.MatchedText)
				// Loop through the match expressions to see if we have a handler for the message that matches
				for matchKey, matcher := range dispatchMatchMap {
					log.Debugf("Checking matcher: %s", matchKey)
					// If the matcher expression matches the failure message, call the handler function related to that matcher (same key)
					if matcher.MatchString(matched.MatchedText) {
						log.Debugf("Dispatch to handler: %s", matchKey)
						err = dispatchFunctions[matchKey](log, clusterRoot, podFile, pod, issueReporter)
						if err != nil {
							log.Errorf("AnalyzeVerrazzanoInstallIssue failed in %s function", matchKey, err)
						}
					}
				}
			}
		} else {
			log.Errorf("AnalyzeVerrazzanoInstallIssue failed to get log messages to determine install issue", err)
		}
	}

	// TODO: If we got here without determining a specific cause, put out a General Issue that the install has failed with supporting details
	//  Note that we may not have a lot of details to provide here (which is why we are falling back to this general issue)
	if len(issueReporter.PendingIssues) == 0 {
		// TODO: Add more supporting details here
		messages := make(StringSlice, 1)
		messages[0] = fmt.Sprintf("Namespace %s, Pod %s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
		files := make(StringSlice, 1)
		files[0] = podFile
		issueReporter.AddKnownIssueMessagesFiles(report.InstallFailure, clusterRoot, messages, files)
	}
	return nil
}
*/

func analyzeNGINXIngressController(log *zap.SugaredLogger, clusterRoot string, podFile string, pod corev1.Pod, issueReporter *report.IssueReporter) (err error) {
	// TODO: We need to add in time range handling here. The timestamps from structured K8S JSON should already be there, but we will also need to
	//     be able to correlate timestamps which are coming from Pod logs (not in the initial handling but we will almost certainly need that)
	//
	// 1) Find the events related to ingress-controller-ingress-nginx-controller service in the ingress-nginx namespace
	// If we have a start/end time for the install containerStatus, then we can use that to only look at logs which are in that time range

	// Look at the ingress-controller-ingress-nginx-controller, and look at the events related to it
	services, err := GetServiceList(log, files.FindFileInNamespace(clusterRoot, "ingress-nginx", "services.json"))
	if err != nil {
		return err
	}
	var controllerService corev1.Service
	controllerServiceSet := false
	for _, service := range services.Items {
		log.Debugf("Service found. namespace: %s, name: %s", service.ObjectMeta.Namespace, service.ObjectMeta.Name)
		if service.ObjectMeta.Name == ingressControllerService {
			log.Debugf("NGINX Ingress Controller service. namespace: %s, name: %s", service.ObjectMeta.Namespace, service.ObjectMeta.Name)
			controllerService = service
			controllerServiceSet = true
		}
	}
	if controllerServiceSet {
		issueDetected := false

		// TODO: Need to handle time range correlation (only events within a time range)
		events, err := GetEventsRelatedToService(log, clusterRoot, controllerService, nil)
		if err != nil {
			log.Debugf("Failed to get events related to the NGINX ingress controller service", err)
			return err
		}
		//flags to make sure we're not capturing the same event message repeatedly
		ephemeralIPLimitReachedCheck := false
		lbServiceLimitReachedCheck := false
		var errorSyncingLoadBalancerCheck bool

		// Check if the event matches failure
		log.Infof("Found %d events", len(events))
		for _, event := range events {
			log.Infof("analyzeNGINXIngressController event Reason: %s", event.Reason)
			if !EventReasonFailedRe.MatchString(event.Reason) {
				continue
			}
			log.Infof("analyzeNGINXIngressController event Reason: %s", event.Message)
			if ephemeralIPLimitReachedRe.MatchString(event.Message) && !ephemeralIPLimitReachedCheck {
				messages := make(StringSlice, 1)
				messages[0] = event.Message
				eventFile := files.FindFileInNamespace(clusterRoot, controllerService.ObjectMeta.Namespace, "events.json")
				files := make(StringSlice, 2)
				files[0] = podFile
				files[1] = eventFile
				issueReporter.AddKnownIssueMessagesFiles(report.IngressOciIPLimitExceeded, clusterRoot, messages, files)
				issueDetected = true
				ephemeralIPLimitReachedCheck = true
			} else if lbServiceLimitReachedRe.MatchString(event.Message) && !lbServiceLimitReachedCheck {
				messages := make(StringSlice, 1)
				messages[0] = event.Message
				eventFile := files.FindFileInNamespace(clusterRoot, controllerService.ObjectMeta.Namespace, "events.json")
				files := make(StringSlice, 2)
				files[0] = podFile
				files[1] = eventFile
				issueReporter.AddKnownIssueMessagesFiles(report.IngressLBLimitExceeded, clusterRoot, messages, files)
				issueDetected = true
				lbServiceLimitReachedCheck = true
			} else if failedToEnsureLoadBalancer.MatchString(event.Message) && !errorSyncingLoadBalancerCheck {
				fmt.Println("I AM HERE ", errorSyncingLoadBalancerCheck)
				messages := make(StringSlice, 1)
				messages[0] = event.Message
				eventFile := files.FindFileInNamespace(clusterRoot, controllerService.ObjectMeta.Namespace, "events.json")
				files := make(StringSlice, 2)
				files[0] = podFile
				files[1] = eventFile
				issueReporter.AddKnownIssueMessagesFiles(report.IngressNoIPFound, clusterRoot, messages, files)
				issueDetected = true
				errorSyncingLoadBalancerCheck = true
			}
		}

		// If we detected a more specific issue above, return now. If we didn't we check for cases where
		// we may not be able to narrow it down fully
		if issueDetected {
			return nil
		}

		// We check the LoadBalancer status to see if there is an IP address set. If not, we can at least
		// advise them that the LoadBalancer may not be setup
		if len(controllerService.Status.LoadBalancer.Ingress) == 0 {
			// TODO: Add and report a known issue here (we know the IP is not set, but not more than that)
			messages := make(StringSlice, 1)
			messages[0] = fmt.Sprintf("Namespace %s, Pod %s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
			files := make(StringSlice, 1)
			files[0] = podFile
			issueReporter.AddKnownIssueMessagesFiles(report.IngressNoLoadBalancerIP, clusterRoot, messages, files)
			return nil
		}

		// if we made it this far we know that there is an issue with the ingress controller but
		// we haven't found anything, so give general advise for now.
		messages := make(StringSlice, 1)
		messages[0] = fmt.Sprintf("Namespace %s, Pod %s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
		// TODO: Time correlation on error search here
		nginxPodErrors, err := files.FindFilesAndSearch(log, files.FindFileInClusterRoot(clusterRoot, "ingress-nginx"), LogFilesMatchRe, WideErrorSearchRe, nil)
		if err != nil {
			log.Debugf("Failed searching NGINX Ingress namespace log files for supporting error log data", err)
		}
		files := make(StringSlice, 1)
		files[0] = podFile
		supportingData := make([]report.SupportData, 1)
		supportingData[0] = report.SupportData{
			Messages:     messages,
			TextMatches:  nginxPodErrors,
			RelatedFiles: files,
		}
		issueReporter.AddKnownIssueSupportingData(report.IngressInstallFailure, clusterRoot, supportingData)
		return nil
	}

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

	var vzResourceList installv1alpha1.VerrazzanoList
	err = encjson.Unmarshal(fileBytes, &vzResourceList)
	if err != nil {
		log.Infof("Failed to unmarshal Verrazzano resource at %s", vzResourcesPath)
		return false, err
	}

	if len(vzResourceList.Items) > 0 {
		// There should be only one Verrazzano resource, so the first item from the list should be good enough
		for _, vzRes := range vzResourceList.Items {
			if vzRes.Status.State != installv1alpha1.VzStateReady {
				log.Debugf("Installation is not good, installation state %s", vzRes.Status.State)

				// Verrazzano installation is not complete, find out the list of components which are not ready
				for _, compStatusDetail := range vzRes.Status.Components {
					if compStatusDetail.State != installv1alpha1.CompStateReady {
						if compStatusDetail.State == installv1alpha1.CompStateDisabled {
							continue
						}
						// Create the list of components which did not reach Ready state, so that we can look for the errors for the component in the platform operator log,
						// and other artifacts for the component
						log.Debugf("Component %s is not in ready state, state is %s", compStatusDetail.Name, vzRes.Status.State)
						componentsNotInReadyState = append(componentsNotInReadyState, compStatusDetail.Name)
					}
				}
				return false, nil
			}
		}
	}
	return true, nil
}

func reportInstallError(log *zap.SugaredLogger, clusterRoot string, issueReporter *report.IssueReporter) error {
	vpologRegExp := regexp.MustCompile(`verrazzano-install/verrazzano-platform-operator-.*/logs.txt`)
	allPodFiles, err := files.GetMatchingFiles(log, clusterRoot, vpologRegExp)
	if err != nil {
		return err
	}
	// podList, _ := GetPodList(log, "/Users/pabhat/vz/work/VZ-4324/885/cluster-dump/ingress-nginx/pods.json")

	// We should get only one pod file, use the first element rather than going through the slice
	vpoLog := allPodFiles[0]
	messages := make(StringSlice, 1)
	messages[0] = "Verrazzano installation is not complete for the following components:"
	// Go through all the components which did not reach Ready state
	for _, comp := range componentsNotInReadyState {
		regExpStr := strings.Replace(componentErrorPattern, "vzcomponent", comp, 1)
		componentErrorMatcher := regexp.MustCompile(regExpStr)
		logMatches, err := files.SearchFile(log, vpoLog, componentErrorMatcher, nil)
		if err != nil {
			log.Infof("There is an error searching the file %s for a regular expression: %s", vpoLog, err)
		}
		var allErrors []LogMessage
		var logMessage LogMessage
		for _, matched := range logMatches {
			fileBytes := []byte(matched.MatchedText)
			err = encjson.Unmarshal(fileBytes, &logMessage)
			if err != nil {
				log.Error("Error unmarshalling the json")
				return err
			}
			allErrors = append(allErrors, logMessage)
		}
		errorMessage := installErrorNotFound
		// For now, display only the last error for the component in the platform operator log
		// Need a better way to handle distinct errors for a component, however some of the errors during the initial
		// stage of the install might indicate any real issue, as reconcile takes care of healing those errors.
		if len(allErrors) > 2 {
			errorMessage = allErrors[len(allErrors)-1].Message
		}
		if len(allErrors) == 1 {
			errorMessage = allErrors[0].Message
		}
		messages = append(messages, "\t " + comp + ": " + errorMessage)

		/*for _, pod := range podList.Items {
			if strings.HasPrefix(pod.Name, "ingress-controller-ingress-nginx-controller") {
				podFile := "/Users/pabhat/vz/work/VZ-4324/885/cluster-dump/ingress-nginx/pods.json"
				for matchKey, matcher := range dispatchMatchMap {
					// log.Infof("Checking matcher: %s", matchKey)
					// If the matcher expression matches the failure message, call the handler function related to that matcher (same key)
					if matcher.MatchString("Failed getting DNS suffix: No IP found for service ingress-controller-ingress-nginx-controller with type LoadBalancer") {
						// log.Infof("Dispatch to handler: %s", matchKey)

						err = dispatchFunctions[matchKey](log, clusterRoot, podFile, pod, issueReporter)
						if err != nil {
							log.Errorf("AnalyzeVerrazzanoInstallIssue failed in %s function", matchKey, err)
						}
					}
				}
			}
		}
		fmt.Println("I AM HERE AS WELL")
		*/

	}
	var files []string
	files = append(files, clusterRoot + "/" + verrazzanoResource)
	files = append(files, vpoLog)
	issueReporter.AddKnownIssueMessagesFiles(report.InstallFailureCompNotReady, clusterRoot, messages, files)
	return nil
}
