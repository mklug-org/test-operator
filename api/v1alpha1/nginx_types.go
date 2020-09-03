/*


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

// NginxSpec defines the desired state of Nginx
type NginxSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +kubebuilder:validation:Minimum=0

	// Replicas is the size of the deployment.
	// The pointer is necessary to allow a real 0 value
	Replicas *int32 `json:"replicas"`

	// Image is the image that will be deployed
	Image string `json:"image"`

	// Ingress is the definition needed if a ingress resource should be created
	// +optional
	Ingress IngressSpec `json:"ingress"`
}

// IngressSpec describes the desired state of the ingress resource
type IngressSpec struct {
	// Enabled controls whether an ingress resource is created
	Enabled bool `json:"enabled"`

	// Hostname is the hostname the ingress will be bound to if enabled
	Hostname string `json:"hostname"`
}

// NginxStatus defines the observed state of Nginx
type NginxStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Updating shows whether the webserver is currently updating
	Updating bool `json:"updating"`
}

// Nginx is the Schema for the nginxes API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Hostname",type=string,JSONPath=`.spec.ingress.hostname`
// +kubebuilder:printcolumn:name="Updating",type=boolean,JSONPath=`.status.updating`
type Nginx struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NginxSpec   `json:"spec,omitempty"`
	Status NginxStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NginxList contains a list of Nginx
type NginxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Nginx `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Nginx{}, &NginxList{})
}
