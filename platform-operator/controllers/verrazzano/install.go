// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package verrazzano

import (
	"context"
	vzconst "github.com/verrazzano/verrazzano/platform-operator/constants"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	context2 "github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/context"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/vzinstance"
	ctrl "sigs.k8s.io/controller-runtime"
)

// reconcileComponents reconciles each component using the following rules:
// 1. Always requeue until all enabled components have completed installation
// 2. Don't update the component state until all the work in that state is done, since
//    that update will cause a state transition
// 3. Loop through all components before returning, except for the case
//    where update status fails, in which case we exit the function and requeue
//    immediately.
func (r *Reconciler) reconcileComponents(vzctx context2.VerrazzanoContext) (ctrl.Result, error) {
	cr := vzctx.ActualCR
	vzctx.Log.Progress("Reconciling components for Verrazzano installation")

	spiCtx, err := spi.NewContext(vzctx.Log, r, vzctx.ActualCR, vzctx.Registry, r.DryRun)
	if err != nil {
		return newRequeueWithDelay(), err
	}

	var requeue bool
	// Loop through all of the Verrazzano components and upgrade each one sequentially for now; will parallelize later
	for _, comp := range spiCtx.GetComponentRegistry().GetComponents() {
		compName := comp.Name()
		compContext := spiCtx.Init(compName).Operation(vzconst.InstallOperation)
		compLog := compContext.Log()
		compLog.Oncef("Component %s is being reconciled", compName)
		err := comp.Reconcile(compContext)
		if err != nil {
			requeue = true
		}
	}
	// Update the status with the instance URLs
	// TODO: See if we can conditionally get the instance info URLs
	cr.Status.VerrazzanoInstance = vzinstance.GetInstanceInfo(spiCtx)
	// Update the VZ status before returning
	if err := r.Status().Update(context.TODO(), spiCtx.ActualCR()); err != nil {
		spiCtx.Log().Errorf("Failed to update Verrazzano resource status: %v", err)
		return newRequeueWithDelay(), err
	}
	if requeue {
		return newRequeueWithDelay(), nil
	}
	return ctrl.Result{}, nil
}

// TODO: merge with new reconcile code in comoponent
//func (r *Reconciler) reconcileComponents(vzctx vzcontext.VerrazzanoContext) (ctrl.Result, error) {
//	spiCtx, err := spi.NewContext(vzctx.Log, r, vzctx.ActualCR, r.DryRun)
//	if err != nil {
//		spiCtx.Log().Errorf("Failed to create component context: %v", err)
//		return newRequeueWithDelay(), err
//	}
//
//	cr := spiCtx.ActualCR()
//	spiCtx.Log().Progress("Reconciling components for Verrazzano installation")
//
//	var requeue bool
//	// Loop through all of the Verrazzano components and upgrade each one sequentially for now; will parallelize later
//	for _, comp := range spiCtx.GetComponentRegistry().GetComponents() {
//		compName := comp.Name()
//		compContext := spiCtx.Init(compName).Operation(vzconst.InstallOperation)
//		compLog := compContext.Log()
//
//		compLog.Oncef("Component %s is being reconciled", compName)
//
//		if !comp.IsOperatorInstallSupported() {
//			compLog.Debugf("Component based install not supported for %s", compName)
//			continue
//		}
//		componentStatus, ok := cr.Status.Components[comp.Name()]
//		if !ok {
//			compLog.Debugf("Did not find status details in map for component %s", comp.Name())
//			continue
//		}
//		if checkConfigUpdated(spiCtx, componentStatus) {
//			if err := r.updateComponentStatus(compContext, "PreInstall started", vzapi.CondPreInstall); err != nil {
//				return ctrl.Result{Requeue: true}, err
//			}
//			compLog.Oncef("Reset component %s state to %v for generation %v", compName, componentStatus.State, spiCtx.ActualCR().Generation)
//			if spiCtx.ActualCR().Status.State == vzapi.VzStateReady {
//				err = r.setInstallingState(vzctx.Log, spiCtx.ActualCR())
//				compLog.Oncef("Reset Verrazzano state to %v for generation %v", spiCtx.ActualCR().Status.State, spiCtx.ActualCR().Generation)
//				if err != nil {
//					spiCtx.Log().Errorf("Failed to reset state: %v", err)
//					return newRequeueWithDelay(), err
//				}
//			}
//		}
//		switch componentStatus.State {
//		case vzapi.CompStateReady:
//			// For delete, we should look at the VZ resource delete timestamp and shift into Quiescing/Uninstalling state
//			compLog.Oncef("Component %s is ready", compName)
//			if err := comp.Reconcile(compContext); err != nil {
//				return newRequeueWithDelay(), err
//			}
//			// After restore '.status.instance' is empty and not updated. Below change will populate the correct values when comp state is Ready
//			if err := r.updateComponentStatus(compContext, "Component is Ready", vzapi.CondInstallComplete); err != nil {
//				return ctrl.Result{Requeue: true}, err
//			}
//			continue
//		case vzapi.CompStateDisabled:
//			if !comp.IsEnabled(compContext.EffectiveCR()) {
//				compLog.Oncef("Component %s is disabled, skipping install", compName)
//				// User has disabled component in Verrazzano CR, don't install
//				continue
//			}
//			if !isVersionOk(compLog, comp.GetMinVerrazzanoVersion(), cr.Status.Version) {
//				// User needs to do upgrade before this component can be installed
//				compLog.Progressf("Component %s cannot be installed until Verrazzano is upgraded to at least version %s",
//					comp.Name(), comp.GetMinVerrazzanoVersion())
//				continue
//			}
//			if err := r.updateComponentStatus(compContext, "PreInstall started", vzapi.CondPreInstall); err != nil {
//				return ctrl.Result{Requeue: true}, err
//			}
//			requeue = true
//
//		case vzapi.CompStatePreInstalling:
//			if !registry.ComponentDependenciesMet(comp, compContext) {
//				compLog.Progressf("Component %s waiting for dependencies %v to be ready", comp.Name(), comp.GetDependencies())
//				requeue = true
//				continue
//			}
//			compLog.Progressf("Component %s pre-install is running ", compName)
//			if err := comp.PreInstall(compContext); err != nil {
//				requeue = true
//				continue
//			}
//			// If component is not installed,install it
//			compLog.Oncef("Component %s install started ", compName)
//			if err := comp.Install(compContext); err != nil {
//				requeue = true
//				continue
//			}
//			if err := r.updateComponentStatus(compContext, "Install started", vzapi.CondInstallStarted); err != nil {
//				return ctrl.Result{Requeue: true}, err
//			}
//			// Install started requeue to check status
//			requeue = true
//		case vzapi.CompStateInstalling:
//			// For delete, we should look at the VZ resource delete timestamp and shift into Quiescing/Uninstalling state
//			// If component is enabled -- need to replicate scripts' config merging logic here
//			// If component is in deployed state, continue
//			if comp.IsReady(compContext) {
//				compLog.Progressf("Component %s post-install is running ", compName)
//				if err := comp.PostInstall(compContext); err != nil {
//					requeue = true
//					continue
//				}
//				compLog.Oncef("Component %s successfully installed", comp.Name())
//				if err := r.updateComponentStatus(compContext, "Install complete", vzapi.CondInstallComplete); err != nil {
//					return ctrl.Result{Requeue: true}, err
//				}
//				// Don't requeue because of this component, it is done install
//				continue
//			}
//			// Install of this component is not done, requeue to check status
//			compLog.Progressf("Component %s waiting to finish installing", compName)
//			requeue = true
//		}
//	}
//	if requeue {
//		return newRequeueWithDelay(), nil
//	}
//	return ctrl.Result{}, nil
//}

//func checkConfigUpdated(ctx spi.ComponentContext, componentStatus *vzapi.ComponentStatusDetails) bool {
//	return (componentStatus.State == vzapi.CompStateReady) &&
//		(ctx.ActualCR().Generation > componentStatus.LastReconciledGeneration)
//}
//
//// Check if the component can be installed in this Verrazzano installation based on version
//// Components might require a specific a minimum version of Verrazzano > 1.0.0
//func isVersionOk(log vzlog.VerrazzanoLogger, compVersion string, vzVersion string) bool {
//	if len(vzVersion) == 0 {
//		return true
//	}
//	vzSemver, err := semver.NewSemVersion(vzVersion)
//	if err != nil {
//		log.Errorf("Failed getting semver from status: %v", err)
//		return false
//	}
//	compSemver, err := semver.NewSemVersion(compVersion)
//	if err != nil {
//		log.Errorf("Failed creating new semver for component: %v", err)
//		return false
//	}
//
//	// return false if VZ version is too low to install component, else true
//	return !vzSemver.IsLessThan(compSemver)
//}
