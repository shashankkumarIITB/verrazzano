// Copyright (c) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

// Package files handles searching
package files

import (
	"bufio"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GetMatchingFiles returns the filenames for files that match a regular expression.
func GetMatchingFiles(log *zap.SugaredLogger, rootDirectory string, fileMatchRe *regexp.Regexp) (fileMatches []string, err error) {
	log.Debugf("GetMatchingFiles called with rootDirectory: %s", rootDirectory)
	if len(rootDirectory) == 0 {
		log.Debugf("GetMatchingFiles requires a rootDirectory")
		return nil, errors.New("GetMatchingFiles requires a rootDirectory")
	}

	if fileMatchRe == nil {
		return nil, fmt.Errorf("GetMatchingFiles requires a regular expression")
	}

	walkFunc := func(fileName string, fileInfo os.FileInfo, err error) error {
		if !fileMatchRe.MatchString(fileName) {
			return nil
		}
		if !fileInfo.IsDir() {
			log.Debugf("GetMatchingFiles %s matched", fileName)
			fileMatches = append(fileMatches, fileName)
		}
		return nil
	}

	err = filepath.Walk(rootDirectory, walkFunc)
	if err != nil {
		log.Debugf("GetMatchingFiles failed to walk the filepath", err)
		return nil, err
	}
	return fileMatches, err
}

// GetMatchingDirectories returns the filenames for directories that match a regular expression.
func GetMatchingDirectories(log *zap.SugaredLogger, rootDirectory string, fileMatchRe *regexp.Regexp) (fileMatches []string, err error) {
	log.Debugf("GetMatchingFiles called with rootDirectory: %s", rootDirectory)
	if len(rootDirectory) == 0 {
		log.Debugf("GetMatchingDirectories requires a root directory")
		return nil, errors.New("GetMatchingDirectories requires a rootDirectory")
	}

	if fileMatchRe == nil {
		return nil, fmt.Errorf("GetMatchingDirectories requires a regular expression")
	}

	walkFunc := func(fileName string, fileInfo os.FileInfo, err error) error {
		if !fileMatchRe.MatchString(fileName) {
			return nil
		}
		if fileInfo.IsDir() {
			log.Debugf("GetMatchingDirectories %s matched", fileName)
			fileMatches = append(fileMatches, fileName)
		}
		return nil
	}

	err = filepath.Walk(rootDirectory, walkFunc)
	if err != nil {
		log.Debugf("GetMatchingFiles failed to walk the filepath", err)
		return nil, err
	}
	return fileMatches, nil
}

// FindNamespaces relies on the directory structure of the cluster-dump/namespaces to
// determine the namespaces that were found in the dump. It will return the
// namespace only here and not the entire path.
func FindNamespaces(log *zap.SugaredLogger, clusterRoot string) (namespaces []string, err error) {
	fileInfos, err := ioutil.ReadDir(clusterRoot)
	if err != nil {
		log.Debugf("FindNamespaces failed to read directory %s", clusterRoot, err)
		return nil, err
	}

	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			namespaces = append(namespaces, filepath.Base(fileInfo.Name()))
		}
	}
	return namespaces, nil
}

// FindFileInClusterRoot will find filename in the cluster root
func FindFileInClusterRoot(clusterRoot string, filename string) string {
	return fmt.Sprintf("%s/%s", clusterRoot, filename)
}

// FindFileNameInNamespace will find filename in the namespace
func FindFileInNamespace(clusterRoot string, namespace string, filename string) string {
	return fmt.Sprintf("%s/%s/%s", clusterRoot, namespace, filename)
}

// FindPodLogFileName will find the name of the log file given a pod
func FindPodLogFileName(clusterRoot string, pod corev1.Pod) string {
	return fmt.Sprintf("%s/%s/%s/logs.txt", clusterRoot, pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
}

// The logs.txt for the platform-operator contains each message a json object. But the file logs.txt is not a well-formed
// json, it contains lines starting with "==== START", "==== END", etc. In one of the runs, found a line which is not a json element -
// I0525 13:17:45.742214       7 request.go:665] Waited for 1.04634193s due to client-side throttling, not priority and fairness, request: GET:https://10.96.0.1:443/apis/admissionregistration.k8s.io/v1?timeout=32s
// Also each of the json elements are not part of an array and so there is no comma. This function creates a well-formed json from the logs.txt, using the warning and error messages
func ConvertVPOLogToJson(logDir, logFile, jsonFile string) error {
	vpoLog := fmt.Sprintf("%s/%s", logDir, logFile)
	fileInfo, err := os.Stat(vpoLog)
	if err != nil || fileInfo.Size() == 0 {
		fmt.Sprintf("file %s is either empty or there is an issue in getting the file info about it", vpoLog)
		return err
	}

	file, err := os.Open(vpoLog)
	if err != nil {
		fmt.Sprintf("file %s not found", vpoLog)
		return err
	}

	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	outputJson := logDir + "/" + jsonFile
	outFile, err := os.Create(outputJson)
	if err != nil {
		fmt.Sprintf("could not create the file %s", outFile)
		return err
	}
	defer outFile.Close()

	// Create the json containing an array of json elements, representing individual log messages
	fmt.Fprintln(outFile, "[")
	for scanner.Scan() {
		// How about just getting the error message
		if strings.HasPrefix(scanner.Text(), "{\"level\":\"error\"") ||  strings.HasPrefix(scanner.Text(), "{\"level\":\"warn\"") {
			fmt.Fprintln(outFile, scanner.Text()+",")
		}
	}
	fmt.Fprintln(outFile, "]")
	return nil
}