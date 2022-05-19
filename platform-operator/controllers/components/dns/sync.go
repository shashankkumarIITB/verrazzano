package dns

import (
	"context"
	"errors"

	"github.com/verrazzano/verrazzano/pkg/log/vzlog"

	dnsapi "github.com/verrazzano/verrazzano/platform-operator/apis/components/dns/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

func (r *Reconciler) reconcileDNS(ctx context.Context, cr *dnsapi.DNS, log vzlog.VerrazzanoLogger) (ctrl.Result, error) {
	// Build the domain name and update the status
	domain, err := buildDomainName(r, cr, log)
	if err != nil {
		return ctrl.Result{}, err
	}
	if domain != cr.Status.DomainName {
		cr.Status.DomainName = domain
		err = r.Status().Update(ctx, cr)
		if err != nil {
			return ctrl.Result{}, log.ErrorfNewErr("Failed to update the Verrazzano DNS resource: %v", err)
		}
	}

	// Update any ingresses

	return ctrl.Result{}, nil
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

	// Get the IP from the specified service
	if len(cr.Spec.Wildcard.Service.Name) > 0 && len(cr.Spec.Wildcard.Service.Namespace) > 0 {
		service := corev1.Service{}
		err := cli.Get(context.TODO(), types.NamespacedName{Name: cr.Spec.Wildcard.Service.Name, Namespace: cr.Spec.Wildcard.Service.Namespace}, &service)
		if err != nil {
			return "", log.ErrorfNewErr("Failed getting service %v: %v", err)
		}
		IP = getIPFromService(&service)
	} else {
		var err error
		IP, err = discoverIngressIP(cli, cr.Spec.Wildcard.Service.Namespace)
		if err != nil {
			return "", log.ErrorfNewErr("Failed discovering a service with an Ingress or External IP: %v", err)
		}
	}

	wildcard := "nip.io"
	if cr.Spec.Wildcard != nil {
		wildcard = cr.Spec.Wildcard.Domain
	}
	domain := IP + "." + wildcard
	return domain, nil
}

// Find a service with an IP that provides ingress into the cluster
func discoverIngressIP(cli client.Reader, namespace string) (string, error) {
	serviceLlist := corev1.ServiceList{}
	var err error
	if len(namespace) > 0 {
		// Use the provided namespace
		namespaceMatcher := client.InNamespace(namespace)
		err = cli.List(context.TODO(), &serviceLlist, namespaceMatcher)
	} else {
		err = cli.List(context.TODO(), &serviceLlist)
	}
	if err != nil {
		return "", err
	}
	for _, service := range serviceLlist.Items {
		IP := getIPFromService(&service)
		if IP != "" {
			return IP, nil
		}
	}
	return "", errors.New("Failed to find service with Ingress or External IP")
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
