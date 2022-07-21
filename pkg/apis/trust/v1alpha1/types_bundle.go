/*
Copyright 2021 The cert-manager Authors.

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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Target",type="string",JSONPath=".status.target.configMap.key",description="Bundle Target Key"
// +kubebuilder:printcolumn:name="Synced",type="string",JSONPath=`.status.conditions[?(@.type == "Synced")].status`,description="Bundle has been synced"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=`.status.conditions[?(@.type == "Synced")].reason`,description="Reason Bundle has Synced status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Timestamp Bundle was created"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

type Bundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Desired state of the Bundle resource.
	Spec BundleSpec `json:"spec"`

	// Status of the Bundle. This is set and managed automatically.
	// +optional
	Status BundleStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Bundle `json:"items"`
}

// BundleSepc defines the desired state of a Bundle.
type BundleSpec struct {
	// Sources is a set of references to data whose data will sync to the target.
	Sources []BundleSource `json:"sources"`

	// Target is the target location in all namespaces to sync source data to.
	Target BundleTarget `json:"target"`
}

// BundleSource is the set of sources whose data will be appended and synced to
// the BundleTarget in all Namespaces.
type BundleSource struct {
	// ConfigMap is a reference to a ConfigMap's `data` key, in the trust
	// Namespace.
	// +optional
	ConfigMap *SourceObjectKeySelector `json:"configMap,omitempty"`

	// Secret is a reference to a Secrets's `data` key, in the trust
	// Namespace.
	// +optional
	Secret *SourceObjectKeySelector `json:"secret,omitempty"`

	// InLine is a simple string to append as the source data.
	// +optional
	InLine *string `json:"inLine,omitempty"`

	// CCADB is an embedded copy of the certificates in Mozilla's root store
	// that have the websites trust bit enabled.
	CCADB *bool `json:"ccadb,omitempty"`
}

// BundleTarget is the target resource that the Bundle will sync all source
// data to.
type BundleTarget struct {
	// ConfigMap is the target ConfigMap in all Namespaces that all Bundle source
	// data will be synced to.
	ConfigMap *KeySelector `json:"configMap,omitempty"`
}

// SourceObjectKeySelector is a reference to a source object and its `data` key
// in the trust Namespace.
type SourceObjectKeySelector struct {
	// Name is the name of the source object in the trust Namespace.
	Name string `json:"name"`

	// KeySelector is the key of the entry in the objects' `data` field to be
	// referenced.
	KeySelector `json:",inline"`
}

// KeySelector is a reference to a key for some map data object.
type KeySelector struct {
	// Key is the key of the entry in the object's `data` field to be used.
	Key string `json:"key"`
}

// BundleStatus defines the observed state of the Bundle.
type BundleStatus struct {
	// Target is the current Target that the Bundle is attempting or has
	// completed syncing the source data to.
	// +optional
	Target *BundleTarget `json:"target"`

	// List of status conditions to indicate the status of the Bundle.
	// Known condition types are `Bundle`.
	// +optional
	Conditions []BundleCondition `json:"conditions,omitempty"`
}

// BundleCondition contains condition information for a Bundle.
type BundleCondition struct {
	// Type of the condition, known values are (`Synced`).
	Type BundleConditionType `json:"type"`

	// Status of the condition, one of ('True', 'False', 'Unknown').
	Status corev1.ConditionStatus `json:"status"`

	// LastTransitionTime is the timestamp corresponding to the last status
	// change of this condition.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a brief machine readable explanation for the condition's last
	// transition.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is a human readable description of the details of the last
	// transition, complementing reason.
	// +optional
	Message string `json:"message,omitempty"`

	// If set, this represents the .metadata.generation that the condition was
	// set based upon.
	// For instance, if .metadata.generation is currently 12, but the
	// .status.condition[x].observedGeneration is 9, the condition is out of date
	// with respect to the current state of the Bundle.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// BundleConditionType represents a Bundle condition value.
type BundleConditionType string

const (
	// BundleConditionSynced indicates that the Bundle has successfully synced
	// all source bundle data to the Bundle target in all Namespaces.
	BundleConditionSynced BundleConditionType = "Synced"
)
