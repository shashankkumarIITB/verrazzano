// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package dns

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/controller"

	vzctrl "github.com/verrazzano/verrazzano/pkg/controller"
	"github.com/verrazzano/verrazzano/pkg/log/vzlog"
	vzstring "github.com/verrazzano/verrazzano/pkg/string"
	dnsapi "github.com/verrazzano/verrazzano/platform-operator/apis/components/dns/v1alpha1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	finalizerName   = "dns.verrazzano.io"
	vzDnsAnnotation = "dns.verrazzano.io/host"
)

// SetupWithManager creates a new controller and adds it to the manager
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	var err error
	r.Controller, err = ctrl.NewControllerManagedBy(mgr).
		For(&dnsapi.DNS{}).
		Build(r)

	return err
}

var initialized bool

// Reconciler reconciles a DNS object.
// The reconciler will create a ServiceAcount, RoleBinding, and a Secret which
// contains the kubeconfig to be used by the Multi-Cluster Agent to access the admin cluster.
type Reconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	Controller   controller.Controller
	log          vzlog.VerrazzanoLogger
	WatchMutex   *sync.RWMutex
	ingressNames []*types.NamespacedName
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Get the  resource
	if ctx == nil {
		ctx = context.TODO()
	}
	cr := &dnsapi.DNS{}
	if err := r.Get(context.TODO(), req.NamespacedName, cr); err != nil {
		// If the resource is not found, that means all of the finalizers have been removed,
		// and the Verrazzano resource has been deleted, so there is nothing left to do.
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		zap.S().Errorf("Failed to fetch DNS resource: %v", err)
		return newRequeueWithDelay(), nil
	}

	// Get the resource logger needed to log message using 'progress' and 'once' methods
	log, err := vzlog.EnsureResourceLogger(&vzlog.ResourceConfig{
		Name:           cr.Name,
		Namespace:      cr.Namespace,
		ID:             string(cr.UID),
		Generation:     cr.Generation,
		ControllerName: "multicluster",
	})
	if err != nil {
		zap.S().Errorf("Failed to create controller logger for DNS controller", err)
	}

	r.log = log
	log.Oncef("Reconciling Verrazzano resource %v", req.NamespacedName)
	res, err := r.doReconcile(ctx, log, cr)
	if vzctrl.ShouldRequeue(res) {
		return res, nil
	}

	// Never return an error since it has already been logged and we don't want the
	// controller runtime to log again (with stack trace).  Just re-queue if there is an error.
	if err != nil {
		return newRequeueWithDelay(), nil
	}

	// Never return an error since it has already been logged and we don't want the
	// controller runtime to log again (with stack trace).  Just re-queue if there is an error.
	if err != nil {
		return newRequeueWithDelay(), nil
	}

	// The resource has been reconciled.
	log.Oncef("Successfully reconciled DNS resource %v", req.NamespacedName)
	return ctrl.Result{}, nil
}

// Reconcile reconciles a DNS object
func (r *Reconciler) doReconcile(ctx context.Context, log vzlog.VerrazzanoLogger, cr *dnsapi.DNS) (ctrl.Result, error) {
	if !cr.ObjectMeta.DeletionTimestamp.IsZero() {
		// Finalizer is present, so lets do the cluster deletion
		if vzstring.SliceContainsString(cr.ObjectMeta.Finalizers, finalizerName) {
			if err := r.reconcileDNSDelete(ctx, cr); err != nil {
				return reconcile.Result{}, err
			}

			// Remove the finalizer and update the Verrazzano resource if the deletion has finished.
			log.Infof("Removing finalizer %s", finalizerName)
			cr.ObjectMeta.Finalizers = vzstring.RemoveStringFromSlice(cr.ObjectMeta.Finalizers, finalizerName)
			err := r.Update(ctx, cr)
			if err != nil && !errors.IsConflict(err) {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add our finalizer if not already added
	if !vzstring.SliceContainsString(cr.ObjectMeta.Finalizers, finalizerName) {
		log.Infof("Adding finalizer %s", finalizerName)
		cr.ObjectMeta.Finalizers = append(cr.ObjectMeta.Finalizers, finalizerName)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, r.reconcileDNS(ctx, cr, r.getIngressNames())
}

// Create a new Result that will cause a reconcile requeue after a short delay
func newRequeueWithDelay() ctrl.Result {
	return vzctrl.NewRequeueWithDelay(1, 2, time.Second)
}

// reconcileManagedClusterDelete performs all necessary cleanup during cluster deletion
func (r *Reconciler) reconcileDNSDelete(ctx context.Context, cr *dnsapi.DNS) error {
	return nil
}

func (r Reconciler) init(namespace string, name string, log vzlog.VerrazzanoLogger) error {
	if initialized {
		return nil
	}
	if err := r.watchIngress(namespace, name); err != nil {
		return err
	}
	initialized = true
	return nil
}
