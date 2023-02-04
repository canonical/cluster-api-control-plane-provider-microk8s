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

package v1beta1

import (
	"github.com/canonical/cluster-api-bootstrap-provider-microk8s/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// UpgradeStrategyType is a string representing the upgrade strategy.
type UpgradeStrategyType string

const (
	// SmartUpgradeStrategyType is an upgrade strategy that
	// performs an in-place upgrade of the control plane on non-HA clusters and
	// a rolling upgrade on HA clusters.
	SmartUpgradeStrategyType UpgradeStrategyType = "SmartUpgrade"

	// InPlaceUpgradeStrategyType is an upgrade strategy that
	// performs an in-place upgrade of the control plane.
	InPlaceUpgradeStrategyType UpgradeStrategyType = "InPlaceUpgrade"

	// RollingUpdateStrategyType is an upgrade strategy that
	// deletes the current control plane machine before creating
	// a new one.
	RollingUpgradeStrategyType UpgradeStrategyType = "RollingUpgrade"
)

const (
	MicroK8sControlPlaneFinalizer = "microk8s.controlplane.cluster.x-k8s.io"
)

// MachineTemplate defines the metadata and infrastructure information
// for control plane machines.
type MachineTemplate struct {
	// InfrastructureTemplate is a required reference to a custom resource
	// offered by an infrastructure provider.
	InfrastructureTemplate corev1.ObjectReference `json:"infrastructureTemplate"`
}

// MicroK8sControlPlaneSpec defines the desired state of MicroK8sControlPlane
type MicroK8sControlPlaneSpec struct {
	// Replicas is the desired number of control-plane machine replicas.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Version defines the desired Kubernetes version.
	// +kubebuilder:validation:MinLength:=2
	// +kubebuilder:validation:Pattern:=^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)([-0-9a-zA-Z_\.+]*)?$
	Version string `json:"version"`

	// MachineTemplate is the machine template to be used for
	// creating control plane machines.
	MachineTemplate `json:"machineTemplate"`

	// UpgradeStrategy describes how to replace existing machines
	// with new ones.
	// Values can be: InPlaceUpgrade, RollingUpgrade or SmartUpgrade.
	// +optional
	// +kubebuilder:validation:Enum=InPlaceUpgrade;RollingUpgrade;SmartUpgrade
	UpgradeStrategy UpgradeStrategyType `json:"upgradeStrategy"`

	// ControlPlaneConfig is the reference configs to be used for initializing and joining
	// machines to the control plane.
	// +optional
	ControlPlaneConfig v1beta1.MicroK8sConfigSpec `json:"controlPlaneConfig,omitempty"`
}

// MicroK8sControlPlaneStatus defines the observed state of MicroK8sControlPlane
type MicroK8sControlPlaneStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// Selector is the label selector in string format to avoid introspection
	// by clients, and is used to provide the CRD-based integration for the
	// scale subresource and additional integrations for things like kubectl
	// describe.. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	Selector string `json:"selector,omitempty"`

	// Total number of non-terminated machines targeted by this control plane
	// (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Total number of fully running and ready control plane machines.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total number of unavailable machines targeted by this control plane.
	// This is the total number of machines that are still required for
	// the deployment to have 100% available capacity. They may either
	// be machines that are running but not yet ready or machines
	// that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// Initialized denotes whether or not the control plane has the
	// uploaded microk8s-config configmap.
	// +optional
	Initialized bool `json:"initialized"`

	// Ready denotes that the MicroK8sControlPlane API Server is ready to
	// receive requests.
	// +optional
	Ready bool `json:"ready"`

	// Bootstrapped denotes whether any nodes received bootstrap request
	// which is required to start etcd and Kubernetes components in MicroK8s.
	// +optional
	Bootstrapped bool `json:"bootstrapped,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions defines current service state of the MicroK8sControlPlane.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=microk8scontrolplanes,shortName=mcp,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=".status.ready",description="MicroK8sControlPlane API Server is ready to receive requests"
// +kubebuilder:printcolumn:name="Initialized",type=boolean,JSONPath=".status.initialized",description="This denotes whether or not the control plane has the uploaded microk8s-config configmap"
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=".status.replicas",description="Total number of non-terminated machines targeted by this control plane"
// +kubebuilder:printcolumn:name="Ready Replicas",type=integer,JSONPath=".status.readyReplicas",description="Total number of fully running and ready control plane machines"
// +kubebuilder:printcolumn:name="Unavailable Replicas",type=integer,JSONPath=".status.unavailableReplicas",description="Total number of unavailable machines targeted by this control plane"

// MicroK8sControlPlane is the Schema for the microk8scontrolplanes API
type MicroK8sControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MicroK8sControlPlaneSpec   `json:"spec,omitempty"`
	Status MicroK8sControlPlaneStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MicroK8sControlPlaneList contains a list of MicroK8sControlPlane
type MicroK8sControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MicroK8sControlPlane `json:"items"`
}

// GetConditions returns the set of conditions for this object.
func (in *MicroK8sControlPlane) GetConditions() clusterv1.Conditions {
	return in.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (in *MicroK8sControlPlane) SetConditions(conditions clusterv1.Conditions) {
	in.Status.Conditions = conditions
}

// +kubebuilder:obj

func init() {
	SchemeBuilder.Register(&MicroK8sControlPlane{}, &MicroK8sControlPlaneList{})
}
