package dns

import (
	"context"
	"errors"
	"fmt"

	ctrlerrors "github.com/verrazzano/verrazzano/pkg/controller/errors"

	"github.com/verrazzano/verrazzano/pkg/log/vzlog"

	dnsapi "github.com/verrazzano/verrazzano/platform-operator/apis/components/dns/v1alpha1"

	k8err "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

func (r *Reconciler) reconcileDNS(ctx context.Context, cr *dnsapi.DNS, ingressNSNs []*types.NamespacedName) error {
	// Build the domain name and update the status
	domain, err := buildDomainName(r, cr, r.log)
	if err != nil {
		return err
	}
	if domain != cr.Status.DomainName {
		cr.Status.DomainName = domain
		err = r.Status().Update(ctx, cr)
		if err != nil {
			return r.log.ErrorfNewErr("Failed to update the Verrazzano DNS resource: %v", err)
		}
	}

	if len(ingressNSNs) == 0 {
		// This is unusual that an ingress doesn't exist since a DNS resource was created.
		// Just to be sure, double check
		ingressNSNs, err = r.searchForAnnotatedIngresses()
		if err != nil {
			return err
		}
		if len(ingressNSNs) == 0 {
			r.log.Progressf("No ingresses exist with the Verrazzano DNS annotation")
			return nil
		}
	}

	// Update any ingresses
	for _, ingressNSN := range ingressNSNs {
		if err := r.reconcileIngress(ctx, ingressNSN, domain); err != nil {
			return err
		}
	}
	return nil
}

// buildDomainName generates a domain name
func buildDomainName(cli client.Reader, cr *dnsapi.DNS, log vzlog.VerrazzanoLogger) (string, error) {
	if len(cr.Spec.Subdomain) == 0 {
		return "", log.ErrorfNewErr("Failed: empty Subdomain field in Verrazzano DNS resource")
	}

	if cr.Spec.OCI != nil {
		return "", errors.New("Failed, OCI DNS not yet supported")
	}

	if cr.Spec.External != nil {
		return "", errors.New("Failed, External DNS not yet supported")
	}

	return buildDomainNameForWildcard(cli, cr, log)
}

// buildDomainNameForWildcard generates a domain name in the format of "<IP>.<wildcard-domain>"
// Get the IP from Istio resources
func buildDomainNameForWildcard(cli client.Reader, cr *dnsapi.DNS, log vzlog.VerrazzanoLogger) (string, error) {
	var IP string
	var serviceName string
	var serviceNamespace string

	wildcard := "nip.io"
	if cr.Spec.Wildcard != nil {
		wildcard = cr.Spec.Wildcard.Domain
		serviceNamespace = cr.Spec.Wildcard.Service.Namespace
		serviceName = cr.Spec.Wildcard.Service.Name
	}

	// Get the IP from the specified service
	if len(serviceName) > 0 && len(serviceNamespace) > 0 {
		service := corev1.Service{}
		err := cli.Get(context.TODO(), types.NamespacedName{Name: serviceName, Namespace: serviceNamespace}, &service)
		if err != nil {
			return "", log.ErrorfNewErr("Failed getting service %v: %v", err)
		}
		IP = getIPFromService(&service)
		if len(IP) == 0 {
			log.Progressf("Waiting for service %s/%s to have an external or ingress IP", service.Namespace, service.Name)
		}
	} else {
		// Need to discover a service with an IP that can be used
		var err error
		IP, err = discoverIngressIP(cli, serviceNamespace, log)
		if err != nil {
			return "", err
		}
	}

	domain := fmt.Sprintf("%s.%s.%s", cr.Spec.Subdomain, IP, wildcard)
	return domain, nil
}

// Find a service with an IP that provides ingress into the cluster
func discoverIngressIP(cli client.Reader, namespace string, log vzlog.VerrazzanoLogger) (string, error) {
	serviceLlist := corev1.ServiceList{}
	var err error
	if len(namespace) > 0 {
		// Use the provided namespace
		namespaceMatcher := client.InNamespace(namespace)
		err = cli.List(context.TODO(), &serviceLlist, namespaceMatcher)
		log.Progressf("DNS IP discovery looking for services in namespace %s", namespace)
	} else {
		log.Progress("DNS IP discovery looking for services in any namespace")
		err = cli.List(context.TODO(), &serviceLlist)
	}
	if k8err.IsNotFound(err) {
		log.Progress("DNS IP discovery cannot find any matching services")
		return "", ctrlerrors.RetryableError{}
	}
	if err != nil {
		log.ErrorfNewErr("Failed in DNS IP discovery: %v", err)
		return "", err

	}
	for _, service := range serviceLlist.Items {
		IP := getIPFromService(&service)
		if len(IP) > 0 {
			log.Oncef("Discovered IP %s in service %s/%s", IP, service.Namespace, service.Name)
			return IP, nil
		}
	}
	log.Progress("Waiting for a service with an Ingress or External IP")
	return "", ctrlerrors.RetryableError{}
}

// getIPFromService gets the External IP or Ingress IP from a service
func getIPFromService(service *corev1.Service) string {
	if service.Spec.Type == corev1.ServiceTypeLoadBalancer || service.Spec.Type == corev1.ServiceTypeNodePort {
		if len(service.Spec.ExternalIPs) > 0 {
			return service.Spec.ExternalIPs[0]
		}
		if len(service.Status.LoadBalancer.Ingress) > 0 {
			return service.Status.LoadBalancer.Ingress[0].IP
		}
	}
	return ""
}
