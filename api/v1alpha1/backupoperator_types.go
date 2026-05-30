/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BackupOperatorSpec defines the desired state of BackupOperator
type BackupOperatorSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// targetPVC is the PersistentVolumeClaim to be backed up
	// +optional
	RestoreGeneration int64 `json:"restoreGeneration,omitempty"`

	TargetPVC string `json:"targetPVC,omitempty"`

	Schedule string `json:"schedule,omitempty"`

	Retention int `json:"retention,omitempty"`

	BackupImage string `json:"backupImage,omitempty"`

	BackupPath string `json:"backupPath,omitempty"`
}

// BackupOperatorStatus defines the observed state of BackupOperator.
type BackupOperatorStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the BackupOperator resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +optional
	LastBackupTime metav1.Time `json:"lastBackupTime,omitempty"`

	LastBackupStatus string `json:"lastBackupStatus,omitempty"`

	ActiveBackupJobs []string `json:"activeBackupJobs,omitempty"`

	RestoreStatus BackupOperatorRestoreStatus `json:"restoreStatus,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type BackupOperatorRestoreStatus struct {
	ObservedRestoreGeneration int64 `json:"observedRestoreGeneration,omitempty"`

	LastRestoreTime metav1.Time `json:"lastRestoreTime,omitempty"`

	RestoreStatus string `json:"restoreStatus,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// BackupOperator is the Schema for the backupoperators API
type BackupOperator struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of BackupOperator
	// +required
	Spec BackupOperatorSpec `json:"spec"`

	// status defines the observed state of BackupOperator
	// +optional
	Status BackupOperatorStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BackupOperatorList contains a list of BackupOperator
type BackupOperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []BackupOperator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupOperator{}, &BackupOperatorList{})
}
