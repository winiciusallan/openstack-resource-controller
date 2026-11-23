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
	"context"
	"iter"
	"net/http"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"go.uber.org/mock/gomock"
	"k8s.io/utils/ptr"

	orcv1alpha1 "github.com/k-orc/openstack-resource-controller/v3/api/v1alpha1"
	"github.com/k-orc/openstack-resource-controller/v3/internal/osclients/mock"
	orcerrors "github.com/k-orc/openstack-resource-controller/v3/internal/util/errors"
)

func TestGetOSResourceByID(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	t.Run("success", func(t *testing.T) {
		headers := map[string]string{
			"X-Container-Object-Count": "5",
			"X-Container-Bytes-Used":   "100",
			"X-Storage-Policy":         "GoldPolicy",
		}
		swiftClient.EXPECT().GetContainer(gomock.Any(), "test-container").Return(headers, nil)

		res, rs := actuator.GetOSResourceByID(context.Background(), "test-container")
		if rs != nil {
			t.Fatalf("unexpected reschedule: %v", rs.GetError())
		}
		if res.Name != "test-container" {
			t.Errorf("expected Name to be test-container, got %s", res.Name)
		}
		if res.Count != 5 {
			t.Errorf("expected Count to be 5, got %d", res.Count)
		}
		if res.Bytes != 100 {
			t.Errorf("expected Bytes to be 100, got %d", res.Bytes)
		}
	})

	t.Run("not found error", func(t *testing.T) {
		errNotFound := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusNotFound}
		swiftClient.EXPECT().GetContainer(gomock.Any(), "test-container").Return(nil, errNotFound)

		res, rs := actuator.GetOSResourceByID(context.Background(), "test-container")
		if res != nil {
			t.Errorf("expected nil resource, got %v", res)
		}
		if rs == nil {
			t.Fatal("expected non-nil reschedule status")
		}
		_, err := rs.NeedsReschedule()
		if !orcerrors.IsNotFound(err) {
			t.Errorf("expected NotFound error, got %v", err)
		}
	})
}

func TestCreateResource(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				Name:          ptr.To(orcv1alpha1.OpenStackName("app-assets-bucket")),
				StoragePolicy: ptr.To("GoldPolicy"),
				ReadACL:       ptr.To(".r:*"),
				WriteACL:      ptr.To("project_admin"),
				QuotaBytes:    ptr.To(int64(10737418240)),
				QuotaCount:    ptr.To(int64(50000)),
				Metadata: map[string]string{
					"owner": "devops",
				},
			},
		},
	}

	expectedCreateOpts := containers.CreateOpts{
		ContainerRead:  ".r:*",
		ContainerWrite: "project_admin",
		StoragePolicy:  "GoldPolicy",
		Metadata: map[string]string{
			"owner":       "devops",
			"Quota-Bytes": "10737418240",
			"Quota-Count": "50000",
		},
	}

	headers := map[string]string{
		"X-Container-Object-Count": "0",
		"X-Container-Bytes-Used":   "0",
	}

	swiftClient.EXPECT().CreateContainer(gomock.Any(), "app-assets-bucket", expectedCreateOpts).Return(headers, nil)

	res, rs := actuator.CreateResource(context.Background(), orcObject)
	if rs != nil {
		t.Fatalf("unexpected reschedule: %v", rs.GetError())
	}
	if res.Name != "app-assets-bucket" {
		t.Errorf("expected Name app-assets-bucket, got %s", res.Name)
	}
	if res.Count != 0 {
		t.Errorf("expected Count 0, got %d", res.Count)
	}
}

func TestDeleteResource(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	t.Run("delete non-empty container fails", func(t *testing.T) {
		res := &osResourceT{
			Container: containers.Container{
				Name:  "non-empty-container",
				Count: 5,
			},
		}

		rs := actuator.DeleteResource(context.Background(), nil, res)
		if rs == nil {
			t.Fatal("expected reschedule status")
		}
		_, err := rs.NeedsReschedule()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !orcv1alpha1.IsConditionReasonTerminal(string(orcv1alpha1.ConditionReasonUnrecoverableError)) {
			t.Fatal("expected terminal unrecoverable error condition reason")
		}
	})

	t.Run("delete empty container succeeds", func(t *testing.T) {
		res := &osResourceT{
			Container: containers.Container{
				Name:  "empty-container",
				Count: 0,
			},
		}

		swiftClient.EXPECT().DeleteContainer(gomock.Any(), "empty-container").Return(nil)

		rs := actuator.DeleteResource(context.Background(), nil, res)
		if rs != nil && rs.GetError() != nil {
			t.Fatalf("unexpected error: %v", rs.GetError())
		}
	})
}

func TestListOSResourcesForAdoption(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	t.Run("adoption matches", func(t *testing.T) {
		orcObject := &orcv1alpha1.SwiftContainer{
			Spec: orcv1alpha1.SwiftContainerSpec{
				Resource: &orcv1alpha1.SwiftContainerResourceSpec{
					Name:          ptr.To(orcv1alpha1.OpenStackName("adopt-me")),
					StoragePolicy: ptr.To("GoldPolicy"),
				},
			},
		}

		headers := map[string]string{
			"X-Storage-Policy":         "GoldPolicy",
			"X-Container-Object-Count": "10",
			"X-Container-Bytes-Used":   "500",
		}

		swiftClient.EXPECT().GetContainer(gomock.Any(), "adopt-me").Return(headers, nil)

		resourceIter, canAdopt := actuator.ListOSResourcesForAdoption(context.Background(), orcObject)
		if !canAdopt {
			t.Fatal("expected canAdopt to be true")
		}

		next, stop := iter.Pull2(resourceIter)
		defer stop()

		res, err, ok := next()
		if !ok {
			t.Fatal("expected to yield at least one resource")
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Name != "adopt-me" {
			t.Errorf("expected Name adopt-me, got %s", res.Name)
		}

		_, _, ok = next()
		if ok {
			t.Error("expected at most one resource")
		}
	})

	t.Run("adoption mismatch fails with terminal error", func(t *testing.T) {
		orcObject := &orcv1alpha1.SwiftContainer{
			Spec: orcv1alpha1.SwiftContainerSpec{
				Resource: &orcv1alpha1.SwiftContainerResourceSpec{
					Name:          ptr.To(orcv1alpha1.OpenStackName("adopt-me")),
					StoragePolicy: ptr.To("GoldPolicy"),
				},
			},
		}

		headers := map[string]string{
			"X-Storage-Policy": "SilverPolicy", // mismatch!
		}

		swiftClient.EXPECT().GetContainer(gomock.Any(), "adopt-me").Return(headers, nil)

		resourceIter, canAdopt := actuator.ListOSResourcesForAdoption(context.Background(), orcObject)
		if !canAdopt {
			t.Fatal("expected canAdopt to be true")
		}

		next, stop := iter.Pull2(resourceIter)
		defer stop()

		_, err, ok := next()
		if !ok {
			t.Fatal("expected to yield error")
		}
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
