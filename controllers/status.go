package controllers

import (
	"context"

	clusterv1beta1 "github.com/canonical/cluster-api-control-plane-provider-microk8s/api/v1beta1"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *MicroK8sControlPlaneReconciler) updateStatus(ctx context.Context,
	mcp *clusterv1beta1.MicroK8sControlPlane, cluster *clusterv1.Cluster) error {
	clusterSelector := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			clusterv1.ClusterLabelName:             cluster.Name,
			clusterv1.MachineControlPlaneLabelName: "",
		},
	}

	selector, err := metav1.LabelSelectorAsSelector(clusterSelector)
	if err != nil {
		// Since we are building up the LabelSelector above, this should not fail
		return errors.Wrap(err, "failed to parse label selector")
	}
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	mcp.Status.Selector = selector.String()

	ownedMachines, err := r.getControlPlaneMachinesForCluster(ctx, util.ObjectKey(cluster), mcp.Name)
	if err != nil {
		return err
	}

	replicas := int32(len(ownedMachines))

	// set basic data that does not require interacting with the workload cluster
	mcp.Status.Ready = false
	mcp.Status.Replicas = replicas
	mcp.Status.ReadyReplicas = 0
	mcp.Status.UnavailableReplicas = replicas

	// Return early if the deletion timestamp is set, we don't want to try to connect to the workload cluster.
	if !mcp.DeletionTimestamp.IsZero() {
		return nil
	}

	logger := log.FromContext(ctx)

	kubeclient, err := r.kubeconfigForCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		logger.Error(err, "failed to get kubeconfig for the cluster")
		return nil
	}

	defer kubeclient.Close() //nolint:errcheck

	err = r.updateProviderID(ctx, util.ObjectKey(cluster), kubeclient)
	if err != nil {
		logger.Error(err, "failed to update provider ID of nodes")
		return err
	}

	nodeSelector := labels.NewSelector()
	req, err := labels.NewRequirement("node.kubernetes.io/microk8s-controlplane", selection.Exists, []string{})
	if err != nil {
		return err
	}

	nodes, err := kubeclient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: nodeSelector.Add(*req).String(),
	})

	if err != nil {
		logger.Error(err, "failed to list controlplane nodes")
		return err
	}

	for _, node := range nodes.Items {
		if util.IsNodeReady(&node) {
			mcp.Status.ReadyReplicas++
		}
	}

	mcp.Status.UnavailableReplicas = replicas - mcp.Status.ReadyReplicas

	if len(nodes.Items) > 0 {
		mcp.Status.Initialized = true
		conditions.MarkTrue(mcp, clusterv1beta1.AvailableCondition)
	}

	if mcp.Status.ReadyReplicas > 0 {
		mcp.Status.Ready = true
	}

	logger.WithValues("count", mcp.Status.ReadyReplicas).Info("ready replicas")

	return nil
}

func (r *MicroK8sControlPlaneReconciler) updateProviderID(ctx context.Context, cluster client.ObjectKey, kubeclient *kubernetesClient) error {
	nodes, err := kubeclient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	logger := log.FromContext(ctx)

	if err != nil {
		logger.Error(err, "failed to list nodes")
		return err
	}

	selector := map[string]string{
		clusterv1.ClusterLabelName: cluster.Name,
	}

	machineList := clusterv1.MachineList{}
	if err := r.Client.List(
		ctx,
		&machineList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(selector),
	); err != nil {
		return err
	}

	for _, node := range nodes.Items {
		if !util.IsNodeReady(&node) || node.Spec.ProviderID != "" {
			continue
		}
		for _, address := range node.Status.Addresses {
			machine := r.findMachineWithAddress(machineList.Items, &address)
			if machine != nil {
				node.Spec.ProviderID = *machine.Spec.ProviderID
				_, err := kubeclient.CoreV1().Nodes().Update(ctx, &node, metav1.UpdateOptions{})
				if err != nil {
					logger.Error(err, "failed to update node")
					return err
				}
				break
			}
		}
	}
	return nil
}

func (r *MicroK8sControlPlaneReconciler) findMachineWithAddress(machineList []clusterv1.Machine, address *v1.NodeAddress) *clusterv1.Machine {
	for _, machine := range machineList {
		for _, maddress := range machine.Status.Addresses {
			if maddress.Address == address.Address {
				return &machine
			}
		}
	}
	return nil
}
