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

package swiftcontainer

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	orcv1alpha1 "github.com/k-orc/openstack-resource-controller/v3/api/v1alpha1"
	"github.com/k-orc/openstack-resource-controller/v3/internal/controllers/generic/interfaces"
	"github.com/k-orc/openstack-resource-controller/v3/internal/controllers/generic/progress"
	"github.com/k-orc/openstack-resource-controller/v3/internal/osclients"
	orcapplyconfigv1alpha1 "github.com/k-orc/openstack-resource-controller/v3/pkg/clients/applyconfiguration/api/v1alpha1"
)

type swiftcontainerStatusWriter struct{}

type objectApplyT = orcapplyconfigv1alpha1.SwiftContainerApplyConfiguration
type statusApplyT = orcapplyconfigv1alpha1.SwiftContainerStatusApplyConfiguration

var _ interfaces.ResourceStatusWriter[*orcv1alpha1.SwiftContainer, *osResource, *objectApplyT, *statusApplyT] = swiftcontainerStatusWriter{}

func (swiftcontainerStatusWriter) GetApplyConfig(name, namespace string) *objectApplyT {
	return orcapplyconfigv1alpha1.SwiftContainer(name, namespace)
}

func (swiftcontainerStatusWriter) ResourceAvailableStatus(orcObject *orcv1alpha1.SwiftContainer, osResource *osResource) (metav1.ConditionStatus, progress.ReconcileStatus) {
	if osResource == nil {
		if orcObject.Status.ID == nil {
			return metav1.ConditionFalse, nil
		} else {
			return metav1.ConditionUnknown, nil
		}
	}

	return metav1.ConditionTrue, nil
}

func (swiftcontainerStatusWriter) ApplyResourceStatus(_ logr.Logger, osResource *osResource, statusApply *statusApplyT) {
	status := osclients.ParseSwiftHeaders(osResource.Headers)

	resourceStatus := orcapplyconfigv1alpha1.SwiftContainerResourceStatus().
		WithName(osResource.Name)

	if status.ReadACL != "" {
		resourceStatus.WithReadACL(status.ReadACL)
	}
	if status.WriteACL != "" {
		resourceStatus.WithWriteACL(status.WriteACL)
	}
	if status.StoragePolicy != "" {
		resourceStatus.WithStoragePolicy(status.StoragePolicy)
	}
	if status.QuotaBytes != nil {
		resourceStatus.WithQuotaBytes(*status.QuotaBytes)
	}
	if status.QuotaCount != nil {
		resourceStatus.WithQuotaCount(*status.QuotaCount)
	}
	if status.BytesUsed != nil {
		resourceStatus.WithBytesUsed(*status.BytesUsed)
	}
	if status.ObjectCount != nil {
		resourceStatus.WithObjectCount(*status.ObjectCount)
	}
	if len(status.Metadata) > 0 {
		resourceStatus.WithMetadata(status.Metadata)
	}

	statusApply.WithResource(resourceStatus)
}
