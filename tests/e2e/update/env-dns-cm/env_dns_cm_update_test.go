// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package envdnscm

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	vzapi "github.com/verrazzano/verrazzano/platform-operator/apis/verrazzano/v1alpha1"

	. "github.com/onsi/gomega"
	"github.com/verrazzano/verrazzano/pkg/test/framework"
	"github.com/verrazzano/verrazzano/tests/e2e/pkg"
	"github.com/verrazzano/verrazzano/tests/e2e/update"
)

const (
	waitTimeout     = 5 * time.Minute
	pollingInterval = 10 * time.Second
)

type EnvironmentNameModifier struct {
	EnvironmentName string
}

type WildcardDNSModifier struct {
	Domain string
}

type CustomCACertificateModifier struct {
	ClusterResourceNamespace string
	SecretName               string
}

func (u EnvironmentNameModifier) ModifyCR(cr *vzapi.Verrazzano) {
	cr.Spec.EnvironmentName = u.EnvironmentName
}

func (u WildcardDNSModifier) ModifyCR(cr *vzapi.Verrazzano) {
	cr.Spec.Components.DNS = &vzapi.DNSComponent{}
	cr.Spec.Components.DNS.Wildcard = &vzapi.Wildcard{}
	cr.Spec.Components.DNS.Wildcard.Domain = u.Domain
}

func (u CustomCACertificateModifier) ModifyCR(cr *vzapi.Verrazzano) {
	var b bool = true
	cr.Spec.Components.CertManager = &vzapi.CertManagerComponent{}
	cr.Spec.Components.CertManager.Enabled = &b
	cr.Spec.Components.CertManager.Certificate.CA.ClusterResourceNamespace = u.ClusterResourceNamespace
	cr.Spec.Components.CertManager.Certificate.CA.SecretName = u.SecretName
}

var (
	t                              = framework.NewTestFramework("Update env-dns-cm")
	testEnvironmentName     string = "test-env"
	testDNSDomain           string = "sslip.io"
	testCertName            string = "test-ca"
	testCertSecretName      string = "test-secret-ca"
	testCertSecretNamespace string = "test-namespace"

	clusterIssuerName string = "verrazzano-cluster-issuer"

	currentEnvironmentName     string
	currentDNSDomain           string
	currentCertNamespace       string = "cert-manager"
	currentCertName            string = "verrazzano-ca-certificate"
	currentIssuerNamespace     string = "cert-manager"
	currentIssuerName          string = "verrazzano-selfsigned-issuer"
	currentCertSecretNamespace string = "cert-manager"
	/* #nosec G101 -- This is a false positive */
	currentCertSecretName string = "verrazzano-ca-certificate-secret"
)

var _ = t.AfterSuite(func() {
	files := []string{testCertName + ".crt", testCertName + ".key"}
	cleanupTemporaryFiles(files)
})

var _ = t.Describe("Test updates to environment name, dns domain and cert-manager CA certificates", func() {
	t.It("Verify the current environment name", func() {
		cr := update.GetCR()
		currentEnvironmentName = cr.Spec.EnvironmentName
		currentDNSDomain = cr.Spec.Components.DNS.Wildcard.Domain
		validateIngressList(currentEnvironmentName, currentDNSDomain)
		validateVirtualServiceList(currentDNSDomain)
	})

	t.It("Update and verify environment name", func() {
		m := EnvironmentNameModifier{testEnvironmentName}
		err := update.UpdateCR(m)
		if err != nil {
			log.Fatalf("Error in updating environment name\n%s", err)
		}
		validateIngressList(testEnvironmentName, currentDNSDomain)
		validateVirtualServiceList(currentDNSDomain)
	})

	t.It("Update and verify dns domain", func() {
		m := WildcardDNSModifier{testDNSDomain}
		err := update.UpdateCR(m)
		if err != nil {
			log.Fatalf("Error in updating DNS domain\n%s", err)
		}
		validateIngressList(testEnvironmentName, testDNSDomain)
		validateVirtualServiceList(testDNSDomain)
	})

	t.It("Update and verify CA certificate", func() {
		createCustomCACertificate(testCertName, testCertSecretNamespace, testCertSecretName)
		m := CustomCACertificateModifier{testCertSecretNamespace, testCertSecretName}
		err := update.UpdateCR(m)
		if err != nil {
			log.Fatalf("Error in updating CA certificate\n%s", err)
		}
		validateCertManagerResourcesCleanup()
		validateClusterIssuerUpdate()
	})
})

func validateIngressList(environmentName string, domain string) {
	log.Printf("Validating the ingresses")
	Eventually(func() bool {
		// Fetch the ingresses for the Verrazzano components
		ingressList, err := pkg.GetIngressList("")
		if err != nil {
			log.Fatalf("Error while fetching IngressList\n%s", err)
		}
		// Verify that the ingresses contain the expected environment name and domain name
		for _, ingress := range ingressList.Items {
			hostname := ingress.Spec.Rules[0].Host
			if !strings.Contains(hostname, environmentName) {
				log.Printf("Ingress %s in namespace %s with hostname %s must contain %s", ingress.Name, ingress.Namespace, hostname, environmentName)
				return false
			}
			if !strings.Contains(hostname, domain) {
				log.Printf("Ingress %s in namespace %s with hostname %s must contain %s", ingress.Name, ingress.Namespace, hostname, domain)
				return false
			}
		}
		return true
	}, waitTimeout, pollingInterval).Should(BeTrue(), "Expected that the ingress hosts contain the expected environment and domain names")
}

func validateVirtualServiceList(domain string) {
	log.Printf("Validating the virtual services")
	Eventually(func() bool {
		// Fetch the virtual services for the deployed applications
		virtualServiceList, err := pkg.GetVirtualServiceList("")
		if err != nil {
			log.Fatalf("Error while fetching VirtualServiceList\n%s", err)
		}
		// Verify that the virtual services contain the expected environment name and domain nameƒ
		for _, virtualService := range virtualServiceList.Items {
			hostname := virtualService.Spec.Hosts[0]
			if !strings.Contains(hostname, domain) {
				log.Printf("Virtual Service %s in namespace %s with hostname %s must contain %s\n", virtualService.Name, virtualService.Namespace, hostname, domain)
				return false
			}
		}
		return true
	}, waitTimeout, pollingInterval).Should(BeTrue(), "Expected that the application virtual service hosts contain the expected domain name")
}

func createCustomCACertificate(certName string, secretNamespace string, secretName string) {
	log.Printf("Creating custom CA certificate")
	output, err := exec.Command("/bin/sh", "create-custom-ca.sh", "-k", "-c", certName, "-s", secretName, "-n", secretNamespace).Output()
	if err != nil {
		log.Println("Error in creating custom CA secret using the script create-custom-ca.sh")
		log.Fatalf("Arguments:\n\t Certificate name: %s\n\t Secret name: %s\n\t Secret namespace: %s\n", certName, secretName, secretNamespace)
	}
	log.Println(string(output))
}

func validateClusterIssuerUpdate() {
	log.Printf("Validating updates to the ClusterIssuer")
	Eventually(func() bool {
		// Fetch the cluster issuers
		clusterIssuer, err := pkg.GetClusterIssuer(clusterIssuerName)
		if err != nil {
			log.Fatalf("Error while fetching ClusterIssuer %s\n%s", clusterIssuerName, err)
		}
		// Verify that the cluster issuer has been updated with the new secret
		if clusterIssuer.Spec.CA == nil {
			log.Printf("ClusterIssuer %s does not contain CA section", clusterIssuerName)
			return false
		}
		if clusterIssuer.Spec.CA.SecretName != testCertSecretName {
			log.Printf("ClusterIssuer %s uses the secret %s, instead of the secret %s\n", clusterIssuerName, clusterIssuer.Spec.CA.SecretName, testCertSecretName)
			return false
		}
		return true
	}, waitTimeout, pollingInterval).Should(BeTrue(), "Expected that the cluster issuer should be updated")
}

func validateCertManagerResourcesCleanup() {
	log.Printf("Validating CA certificate resource cleanup")
	Eventually(func() bool {
		// Fetch the certificates
		certificateList, err := pkg.GetCertificateList("")
		if err != nil {
			log.Fatalf("Error while fetching CertificateList\n%s", err)
		}
		for _, certificate := range certificateList.Items {
			// Currently issued certificate must be removed
			if certificate.Name == currentCertName && certificate.Namespace == currentCertNamespace {
				log.Printf("Certificate %s should NOT exist in the namespace %s\n", currentCertName, currentCertNamespace)
				return false
			}
		}
		// Verify that the certificate issuer has been removed
		issuerList, err := pkg.GetIssuerList(currentIssuerNamespace)
		if err != nil {
			log.Fatalf("Error while fetching IssuerList\n%s", err)
		}
		for _, issuer := range issuerList.Items {
			// Self-signed issuer must not exist
			if issuer.Name == currentIssuerName && issuer.Namespace == currentIssuerNamespace {
				log.Printf("Issuer %s should NOT exist in the namespace %s\n", currentIssuerName, currentIssuerNamespace)
				return false
			}
		}
		// Verify that the secret used for the default certificate has been removed
		_, err = pkg.GetSecret(currentCertSecretNamespace, currentCertSecretName)
		if err != nil {
			log.Printf("Expected that the secret %s should NOT exist in the namespace %s", currentCertSecretName, currentCertSecretNamespace)
		} else {
			log.Printf("Secret %s should NOT exist in the namespace %s\n", currentCertSecretName, currentCertSecretNamespace)
			return false
		}
		return true
	}, waitTimeout, pollingInterval).Should(BeTrue(), "Expected that the default CA resources should be cleaned up")
}

func cleanupTemporaryFiles(files []string) error {
	log.Printf("Cleaning up temporary files")
	var err error
	for _, file := range files {
		_, err = os.Stat(file)
		if os.IsNotExist(err) {
			log.Printf("File %s does not exist", file)
			continue
		}
		err = os.Remove(file)
		if err != nil {
			log.Fatalf("Error while cleaning up temporary file %s\n%s", file, err)
		}
	}
	return err
}