/*
Copyright The ORC Authors.

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

// SwiftContainerResourceSpec contains the desired state of a Swift container.
type SwiftContainerResourceSpec struct {
	// name will be the name of the created resource. If not specified, the
	// name of the ORC object will be used.
	// +optional
	Name *OpenStackName `json:"name,omitempty"`

	// metadata is a key-value map representing X-Container-Meta-* properties.
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`

	// readACL translates to X-Container-Read access control rules.
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	ReadACL *string `json:"readACL,omitempty"`

	// writeACL translates to X-Container-Write access control rules.
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	WriteACL *string `json:"writeACL,omitempty"`

	// storagePolicy translates to X-Storage-Policy.
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	StoragePolicy *string `json:"storagePolicy,omitempty"`

	// quotaBytes is the maximum container size limit in bytes.
	// +kubebuilder:validation:Minimum:=0
	// +optional
	QuotaBytes *int64 `json:"quotaBytes,omitempty"`

	// quotaCount is the maximum container object limit count.
	// +kubebuilder:validation:Minimum:=0
	// +optional
	QuotaCount *int64 `json:"quotaCount,omitempty"`
}

// SwiftContainerFilter defines an existing resource by its properties
// +kubebuilder:validation:MinProperties:=1
type SwiftContainerFilter struct {
	// name is the exact name of the Swift container.
	// +optional
	Name *OpenStackName `json:"name,omitempty"`

	// storagePolicy filter by Swift storage policy
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	StoragePolicy *string `json:"storagePolicy,omitempty"`

	// readACL filter by X-Container-Read access control rules
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	ReadACL *string `json:"readACL,omitempty"`

	// writeACL filter by X-Container-Write access control rules
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	WriteACL *string `json:"writeACL,omitempty"`

	// quotaBytes filter by maximum container size in bytes
	// +kubebuilder:validation:Minimum:=0
	// +optional
	QuotaBytes *int64 `json:"quotaBytes,omitempty"`

	// quotaCount filter by maximum container object count
	// +kubebuilder:validation:Minimum:=0
	// +optional
	QuotaCount *int64 `json:"quotaCount,omitempty"`

	// metadata filter by container metadata properties
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SwiftContainerResourceStatus represents the observed state of the resource.
type SwiftContainerResourceStatus struct {
	// name is the actual name of the container in Swift.
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	Name string `json:"name,omitempty"`

	// metadata is a map of container metadata properties (X-Container-Meta-*)
	// +optional
	Metadata map[string]string `json:"metadata,omitempty"`

	// readACL represents X-Container-Read access control rules
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	ReadACL string `json:"readACL,omitempty"`

	// writeACL represents X-Container-Write access control rules
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	WriteACL string `json:"writeACL,omitempty"`

	// storagePolicy represents the actual Swift storage policy
	// +kubebuilder:validation:MaxLength=1024
	// +optional
	StoragePolicy string `json:"storagePolicy,omitempty"`

	// quotaBytes represents the maximum container size limit in bytes
	// +optional
	QuotaBytes *int64 `json:"quotaBytes,omitempty"`

	// quotaCount represents the maximum container object limit count
	// +optional
	QuotaCount *int64 `json:"quotaCount,omitempty"`

	// bytesUsed is the actual bytes used in Swift
	// +optional
	BytesUsed *int64 `json:"bytesUsed,omitempty"`

	// objectCount is the actual object count in Swift
	// +optional
	ObjectCount *int64 `json:"objectCount,omitempty"`
}
