/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	clusterv1beta1 "github.com/canonical/cluster-api-control-plane-provider-microk8s/api/v1beta1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const requeueDuration = 30 * time.Second

// ControlPlane holds business logic around control planes.
// It should never need to connect to a service, that responsibility lies outside of this struct.
type ControlPlane struct {
	MCP      *clusterv1beta1.MicroK8sControlPlane
	Cluster  *clusterv1.Cluster
	Machines []clusterv1.Machine
}

// MicroK8sControlPlaneReconciler reconciles a MicroK8sControlPlane object
type MicroK8sControlPlaneReconciler struct {
	client.Client
	APIReader client.Reader
	Scheme    *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,namespace=kube-system,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac,resources=roles,namespace=kube-system,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=rbac,resources=rolebindings,namespace=kube-system,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MicroK8sControlPlane object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *MicroK8sControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, reterr error) {
	logger := log.FromContext(ctx)

	mcp := &clusterv1beta1.MicroK8sControlPlane{}
	if err := r.APIReader.Get(ctx, req.NamespacedName, mcp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// TODO(user): your logic here
	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, mcp.ObjectMeta)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to retrieve owner Cluster from the API Server")

			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}

	if cluster == nil {
		logger.Info("cluster Controller has not yet set OwnerRef")
		return ctrl.Result{Requeue: true}, nil
	}
	logger = logger.WithValues("cluster", cluster.Name)

	ctx = log.IntoContext(ctx, logger)

	if annotations.IsPaused(cluster, mcp) {
		logger.Info("reconciliation is paused for this object")
		return ctrl.Result{Requeue: true}, nil
	}

	// Wait for the cluster infrastructure to be ready before creating machines
	if !cluster.Status.InfrastructureReady {
		logger.Info("cluster infra not ready")

		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(mcp, r.Client)
	if err != nil {
		logger.Error(err, "failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(mcp, clusterv1beta1.MicroK8sControlPlaneFinalizer) {
		controllerutil.AddFinalizer(mcp, clusterv1beta1.MicroK8sControlPlaneFinalizer)

		// patch and return right away instead of reusing the main defer,
		// because the main defer may take too much time to get cluster status

		if err := patchMicroK8sControlPlane(ctx, patchHelper, mcp, patch.WithStatusObservedGeneration{}); err != nil {
			logger.Error(err, "failed to add finalizer to MicroK8sControlPlane")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	defer func() {
		logger.Info("attempting to set control plane status")

		// Always attempt to update status.
		if err := r.updateStatus(ctx, mcp, cluster); err != nil {
			logger.Error(err, "failed to update MicroK8sControlPlane Status")

			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// Always attempt to Patch the MicroK8sControlPlane object and status after each reconciliation.
		if err := patchMicroK8sControlPlane(ctx, patchHelper, mcp, patch.WithStatusObservedGeneration{}); err != nil {
			logger.Error(err, "failed to patch MicroK8sControlPlane")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// TODO: remove this as soon as we have a proper remote cluster cache in place.
		// Make TCP to requeue in case status is not ready, so we can check for node status without waiting for a full resync (by default 10 minutes).
		// Only requeue if we are not going in exponential backoff due to error, or if we are not already re-queueing, or if the object has a deletion timestamp.
		if reterr == nil && !res.Requeue && res.RequeueAfter <= 0 && mcp.ObjectMeta.DeletionTimestamp.IsZero() {
			if !mcp.Status.Ready || mcp.Status.UnavailableReplicas > 0 {
				res = ctrl.Result{RequeueAfter: 20 * time.Second}
			}
		}

		res = ctrl.Result{RequeueAfter: 30 * time.Second}
		logger.Info("successfully updated control plane status")
	}()

	if !mcp.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		return r.reconcileDelete(ctx, cluster, mcp)
	}

	return r.reconcile(ctx, cluster, mcp)
}

func patchMicroK8sControlPlane(ctx context.Context, patchHelper *patch.Helper, tcp *clusterv1beta1.MicroK8sControlPlane, opts ...patch.Option) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(tcp,
		conditions.WithConditions(
			clusterv1beta1.MachinesCreatedCondition,
			clusterv1beta1.ResizedCondition,
			clusterv1beta1.MachinesReadyCondition,
			clusterv1beta1.AvailableCondition,
			clusterv1beta1.MachinesBootstrapped,
		),
	)

	opts = append(opts,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1beta1.MachinesCreatedCondition,
			clusterv1.ReadyCondition,
			clusterv1beta1.ResizedCondition,
			clusterv1beta1.MachinesReadyCondition,
			clusterv1beta1.AvailableCondition,
			clusterv1beta1.MachinesBootstrapped,
		}},
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		tcp,
		opts...,
	)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MicroK8sControlPlaneReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1beta1.MicroK8sControlPlane{}).
		Owns(&clusterv1.Machine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Cluster{}},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToMicroK8sControlPlane),
		).
		WithOptions(options).
		Complete(r)
}

func (r *MicroK8sControlPlaneReconciler) ClusterToMicroK8sControlPlane(o client.Object) []ctrl.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		fmt.Printf("expected a Cluster but got a %T\n", o)
		return nil
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "MicroK8sControlPlane" {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}

	return nil
}

func (r *MicroK8sControlPlaneReconciler) getControlPlaneMachinesForCluster(ctx context.Context, cluster client.ObjectKey, cpName string) ([]clusterv1.Machine, error) {
	selector := map[string]string{
		clusterv1.ClusterLabelName:             cluster.Name,
		clusterv1.MachineControlPlaneLabelName: "",
	}

	machineList := clusterv1.MachineList{}
	if err := r.Client.List(
		ctx,
		&machineList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(selector),
	); err != nil {
		return nil, err
	}

	return machineList.Items, nil
}

func (r *MicroK8sControlPlaneReconciler) newControlPlane(cluster *clusterv1.Cluster, mcp *clusterv1beta1.MicroK8sControlPlane, machines []clusterv1.Machine) *ControlPlane {
	return &ControlPlane{
		MCP:      mcp,
		Cluster:  cluster,
		Machines: machines,
	}
}
