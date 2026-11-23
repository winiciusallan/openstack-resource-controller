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
	"testing"

	"github.com/go-logr/logr"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	orcv1alpha1 "github.com/k-orc/openstack-resource-controller/v3/api/v1alpha1"
	orcapplyconfigv1alpha1 "github.com/k-orc/openstack-resource-controller/v3/pkg/clients/applyconfiguration/api/v1alpha1"
)

func TestResourceAvailableStatus(t *testing.T) {
	writer := swiftcontainerStatusWriter{}

	t.Run("nil osResource, nil status ID", func(t *testing.T) {
		orcObject := &orcv1alpha1.SwiftContainer{}
		status, rs := writer.ResourceAvailableStatus(orcObject, nil)
		if rs != nil {
			t.Errorf("expected nil ReconcileStatus, got %v", rs)
		}
		if status != metav1.ConditionFalse {
			t.Errorf("expected ConditionFalse, got %v", status)
		}
	})

	t.Run("nil osResource, non-nil status ID", func(t *testing.T) {
		orcObject := &orcv1alpha1.SwiftContainer{
			Status: orcv1alpha1.SwiftContainerStatus{
				ID: ptr.To("test-container"),
			},
		}
		status, rs := writer.ResourceAvailableStatus(orcObject, nil)
		if rs != nil {
			t.Errorf("expected nil ReconcileStatus, got %v", rs)
		}
		if status != metav1.ConditionUnknown {
			t.Errorf("expected ConditionUnknown, got %v", status)
		}
	})

	t.Run("non-nil osResource", func(t *testing.T) {
		orcObject := &orcv1alpha1.SwiftContainer{}
		osResource := &osResourceT{
			Container: containers.Container{Name: "test-container"},
		}
		status, rs := writer.ResourceAvailableStatus(orcObject, osResource)
		if rs != nil {
			t.Errorf("expected nil ReconcileStatus, got %v", rs)
		}
		if status != metav1.ConditionTrue {
			t.Errorf("expected ConditionTrue, got %v", status)
		}
	})
}

func TestApplyResourceStatus(t *testing.T) {
	writer := swiftcontainerStatusWriter{}

	t.Run("mappings work", func(t *testing.T) {
		osResource := &osResourceT{
			Container: containers.Container{
				Name: "my-container",
			},
			Headers: map[string]string{
				"X-Container-Read":             ".r:*",
				"X-Container-Write":            "project_admin",
				"X-Storage-Policy":             "GoldPolicy",
				"X-Container-Meta-Quota-Bytes": "10737418240",
				"X-Container-Meta-Quota-Count": "50000",
				"X-Container-Object-Count":     "5",
				"X-Container-Bytes-Used":       "100",
				"X-Container-Meta-Owner":       "devops",
			},
		}

		statusApply := orcapplyconfigv1alpha1.SwiftContainerStatus()
		writer.ApplyResourceStatus(logr.Discard(), osResource, statusApply)

		if statusApply.Resource == nil {
			t.Fatal("expected Resource to be populated")
		}

		res := statusApply.Resource
		if *res.Name != "my-container" {
			t.Errorf("expected Name my-container, got %s", *res.Name)
		}
		if *res.ReadACL != ".r:*" {
			t.Errorf("expected ReadACL .r:*, got %s", *res.ReadACL)
		}
		if *res.WriteACL != "project_admin" {
			t.Errorf("expected WriteACL project_admin, got %s", *res.WriteACL)
		}
		if *res.StoragePolicy != "GoldPolicy" {
			t.Errorf("expected StoragePolicy GoldPolicy, got %s", *res.StoragePolicy)
		}
		if *res.QuotaBytes != 10737418240 {
			t.Errorf("expected QuotaBytes 10737418240, got %d", *res.QuotaBytes)
		}
		if *res.QuotaCount != 50000 {
			t.Errorf("expected QuotaCount 50000, got %d", *res.QuotaCount)
		}
		if *res.BytesUsed != 100 {
			t.Errorf("expected BytesUsed 100, got %d", *res.BytesUsed)
		}
		if *res.ObjectCount != 5 {
			t.Errorf("expected ObjectCount 5, got %d", *res.ObjectCount)
		}
		if res.Metadata["owner"] != "devops" {
			t.Errorf("expected Metadata owner=devops, got %v", res.Metadata)
		}
	})
}
