package dns

import (
	"context"

	k8net "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// syncIngress updates the ingress with the domain name
func (r *Reconciler) reconcileIngress(ctx context.Context, ingressNSN *types.NamespacedName, domain string) error {
	ingress := k8net.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressNSN.Name,
			Namespace: ingressNSN.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, &ingress, func() error {
		rules := ingress.Spec.Rules
		if len(rules) == 0 {
			return r.log.ErrorfNewErr("Failed to update host field with DNS name because Ingress is missing rules")
		}
		rules[0].Host = domain
		return nil
	})
	if err != nil {
		return r.log.ErrorfNewErr("Failed to udpate Ingress %v: %v", ingressNSN, err)
	}
	r.log.Progressf("Ingress %v host field is set to %s", ingressNSN, domain)
	return nil
}

// watchIngress watch all ingresses
func (r *Reconciler) watchIngress(namespace string, name string) error {
	p := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			ingress := e.Object.(*k8net.Ingress)
			r.log.Infof("Create event for Ingress %s/%s", ingress.Namespace, ingress.Name)
			return r.CheckIngress(ingress)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == e.ObjectNew {
				return false
			}
			ingress := e.ObjectNew.(*k8net.Ingress)
			r.log.Infof("Update event for Ingress %s/%s", ingress.Namespace, ingress.Name)
			return r.CheckIngress(ingress)
		},
	}
	return r.Controller.Watch(
		&source.Kind{Type: &k8net.Ingress{}},
		createReconcileEventHandler(namespace, name),
		p)
}

func createReconcileEventHandler(namespace, name string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(
		func(a client.Object) []reconcile.Request {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Namespace: namespace,
					Name:      name,
				}},
			}
		})
}

func (r *Reconciler) CheckIngress(ingress *k8net.Ingress) bool {
	// todo - this should have verrazzano namespace/name as value so that we can have
	// multiple verrazzanos
	if ingress.Annotations == nil || ingress.Annotations[vzDnsAnnotation] != "true" {
		return false
	}
	r.WatchMutex.Lock()
	defer r.WatchMutex.Unlock()
	r.ingressNames = append(r.ingressNames,
		&types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name})
	return true
}
func (r *Reconciler) getIngressNames() []*types.NamespacedName {
	ingressNames := []*types.NamespacedName{}
	if len(r.ingressNames) > 0 {
		r.WatchMutex.Lock()
		defer r.WatchMutex.Unlock()
		for i, _ := range r.ingressNames {
			ingressNames = append(ingressNames, r.ingressNames[i])
		}
	}
	return ingressNames
}

func (r Reconciler) searchForAnnotatedIngresses() ([]*types.NamespacedName, error) {
	r.log.Progress("Searching for Ingresses that DNS annotation")

	NSNs := []*types.NamespacedName{}
	ingressList := k8net.IngressList{}
	if err := r.List(context.TODO(), &ingressList); err != nil {
		return nil, r.log.ErrorfNewErr("Error listing ingresses: %v", err)
	}
	for _, ingress := range ingressList.Items {
		if ingress.Annotations == nil || ingress.Annotations[vzDnsAnnotation] != "true" {
			NSNs = append(NSNs, &types.NamespacedName{Namespace: ingress.Namespace, Name: ingress.Name})
		}
	}
	return NSNs, nil
}
