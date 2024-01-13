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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DomainNameSpec defines the desired state of DomainName
type DomainNameSpec struct {
	//+kubebuilder:validation:required
	// Domain is the domain name to register
	Domain string `json:"domain"`
	//+kubebuilder:validation:required
	// Address is the ipv4 address of the host to assign to the domain
	Address string `json:"address"`
}

// DomainNameStatus defines the observed state of DomainName
type DomainNameStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DomainName is the Schema for the domainnames API
type DomainName struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DomainNameSpec   `json:"spec,omitempty"`
	Status DomainNameStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DomainNameList contains a list of DomainName
type DomainNameList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DomainName `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DomainName{}, &DomainNameList{})
}
