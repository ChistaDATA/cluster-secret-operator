/*
Copyright 2023.

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

package v1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterSecretSpec defines the desired state of ClusterSecret
type ClusterSecretSpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Required
	IncludeNamespaces []string `json:"includeNamespaces,omitempty"`
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`
	// +kubebuilder:default:=Opaque
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum:=Opaque;kubernetes.io/service-account-token;kubernetes.io/dockercfg;kubernetes.io/dockerconfigjson;kubernetes.io/basic-auth;kubernetes.io/ssh-auth;kubernetes.io/tls;bootstrap.kubernetes.io/token
	Type v1.SecretType `json:"type,omitempty"`
	// +kubebuilder:validation:Optional
	Data map[string][]byte `json:"data,omitempty"`
	// +kubebuilder:validation:Optional
	ValueFrom ValueFrom `json:"valueFrom,omitempty"`
}

type ValueFrom struct {
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName,omitempty"`
	// +kubebuilder:validation:Required
	SecretNamespace string `json:"secretNamespace,omitempty"`
}

// ClusterSecretStatus defines the observed state of ClusterSecret
type ClusterSecretStatus struct {
	// Represents the observations of a ClusterSecrets' current state.
	// ClusterSecret.status.conditions.type are: "Available", "Progressing", and "Degraded"
	// ClusterSecret.status.conditions.status are one of True, False, Unknown.
	// ClusterSecret.status.conditions.reason the value should be a CamelCase string and producers of specific
	// condition types may define expected values and meanings for this field, and whether the values
	// are considered a guaranteed API.
	// ClusterSecret.status.conditions.Message is a human-readable message indicating details about the transition.
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ClusterSecret is the Schema for the clustersecrets API
type ClusterSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSecretSpec   `json:"spec,omitempty"`
	Status ClusterSecretStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterSecretList contains a list of ClusterSecret
type ClusterSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterSecret{}, &ClusterSecretList{})
}
