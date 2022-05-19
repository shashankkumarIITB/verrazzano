package dns

import (
	k8net "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// watchIngress watch all ingresses
func (r *Reconciler) watchIngress(namespace string, name string) error {
	p := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			ingress := e.Object.(*k8net.Ingress)
			return r.CheckIngress(ingress)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectOld == e.ObjectNew {
				return false
			}
			ingress := e.ObjectNew.(*k8net.Ingress)
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
	r.ingresses = append(r.ingresses, ingress)
	return true
}
