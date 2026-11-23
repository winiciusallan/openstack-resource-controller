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
	"errors"
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

	t.Run("unexpected error", func(t *testing.T) {
		errUnexpected := errors.New("unexpected swift container error")
		swiftClient.EXPECT().GetContainer(gomock.Any(), "test-container").Return(nil, errUnexpected)

		res, rs := actuator.GetOSResourceByID(context.Background(), "test-container")
		if res != nil {
			t.Errorf("expected nil resource, got %v", res)
		}
		if rs == nil {
			t.Fatal("expected non-nil reschedule status")
		}
		_, err := rs.NeedsReschedule()
		if !errors.Is(err, errUnexpected) {
			t.Errorf("expected error %v, got %v", errUnexpected, err)
		}
	})
}

func TestCreateResource(t *testing.T) {
	t.Run("create success", func(t *testing.T) {
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
	})

	t.Run("create terminal error", func(t *testing.T) {
		mockctrl := gomock.NewController(t)
		defer mockctrl.Finish()

		swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
		actuator := swiftcontainerActuator{osClient: swiftClient}

		orcObject := &orcv1alpha1.SwiftContainer{
			Spec: orcv1alpha1.SwiftContainerSpec{
				Resource: &orcv1alpha1.SwiftContainerResourceSpec{
					Name: ptr.To(orcv1alpha1.OpenStackName("app-assets-bucket")),
				},
			},
		}

		errTerminal := errors.New("client-side validation error") // non-retryable non-HTTP error
		swiftClient.EXPECT().CreateContainer(gomock.Any(), "app-assets-bucket", gomock.Any()).Return(nil, errTerminal)

		_, rs := actuator.CreateResource(context.Background(), orcObject)
		if rs == nil {
			t.Fatal("expected non-nil reschedule status")
		}
		_, err := rs.NeedsReschedule()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var terminalError *orcerrors.TerminalError
		if !errors.As(err, &terminalError) {
			t.Fatalf("expected terminal error, got %v", err)
		}
		if terminalError.Reason != string(orcv1alpha1.ConditionReasonInvalidConfiguration) {
			t.Errorf("expected Reason %s, got %s", orcv1alpha1.ConditionReasonInvalidConfiguration, terminalError.Reason)
		}
	})

	t.Run("create retryable error", func(t *testing.T) {
		mockctrl := gomock.NewController(t)
		defer mockctrl.Finish()

		swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
		actuator := swiftcontainerActuator{osClient: swiftClient}

		orcObject := &orcv1alpha1.SwiftContainer{
			Spec: orcv1alpha1.SwiftContainerSpec{
				Resource: &orcv1alpha1.SwiftContainerResourceSpec{
					Name: ptr.To(orcv1alpha1.OpenStackName("app-assets-bucket")),
				},
			},
		}

		errRetryable := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusInternalServerError} // 5xx
		swiftClient.EXPECT().CreateContainer(gomock.Any(), "app-assets-bucket", gomock.Any()).Return(nil, errRetryable)

		_, rs := actuator.CreateResource(context.Background(), orcObject)
		if rs == nil {
			t.Fatal("expected non-nil reschedule status")
		}
		_, err := rs.NeedsReschedule()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var terminalError *orcerrors.TerminalError
		if errors.As(err, &terminalError) {
			t.Fatal("expected non-terminal error, got terminal error")
		}
	})
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
		var terminalError *orcerrors.TerminalError
		if !errors.As(err, &terminalError) {
			t.Fatalf("expected terminal error, got %v", err)
		}
		if terminalError.Reason != string(orcv1alpha1.ConditionReasonUnrecoverableError) {
			t.Errorf("expected Reason %s, got %s", orcv1alpha1.ConditionReasonUnrecoverableError, terminalError.Reason)
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

	t.Run("delete empty container fails with retryable error", func(t *testing.T) {
		res := &osResourceT{
			Container: containers.Container{
				Name:  "empty-container",
				Count: 0,
			},
		}

		errRetryable := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusInternalServerError}
		swiftClient.EXPECT().DeleteContainer(gomock.Any(), "empty-container").Return(errRetryable)

		rs := actuator.DeleteResource(context.Background(), nil, res)
		if rs == nil {
			t.Fatal("expected non-nil reschedule status")
		}
		_, err := rs.NeedsReschedule()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var terminalError *orcerrors.TerminalError
		if errors.As(err, &terminalError) {
			t.Fatal("expected non-terminal error, got terminal error")
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

func TestListOSResourcesForImport(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	t.Run("import by name success", func(t *testing.T) {
		filter := orcv1alpha1.SwiftContainerFilter{
			Name: ptr.To(orcv1alpha1.OpenStackName("legacy-archive")),
		}

		headers := map[string]string{
			"X-Container-Object-Count": "10",
			"X-Container-Bytes-Used":   "500",
		}

		swiftClient.EXPECT().GetContainer(gomock.Any(), "legacy-archive").Return(headers, nil)

		resourceIter, status := actuator.ListOSResourcesForImport(context.Background(), nil, filter)
		if status != nil {
			t.Fatalf("unexpected status: %v", status.GetError())
		}

		next, stop := iter.Pull2(resourceIter)
		defer stop()

		res, err, ok := next()
		if !ok {
			t.Fatal("expected to yield a resource")
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Name != "legacy-archive" {
			t.Errorf("expected Name legacy-archive, got %s", res.Name)
		}
		if res.Count != 10 {
			t.Errorf("expected Count 10, got %d", res.Count)
		}

		_, _, ok = next()
		if ok {
			t.Error("expected only one resource")
		}
	})

	t.Run("import by name not found", func(t *testing.T) {
		filter := orcv1alpha1.SwiftContainerFilter{
			Name: ptr.To(orcv1alpha1.OpenStackName("legacy-archive")),
		}

		errNotFound := gophercloud.ErrUnexpectedResponseCode{Actual: http.StatusNotFound}
		swiftClient.EXPECT().GetContainer(gomock.Any(), "legacy-archive").Return(nil, errNotFound)

		resourceIter, status := actuator.ListOSResourcesForImport(context.Background(), nil, filter)
		if status != nil {
			t.Fatalf("unexpected status: %v", status.GetError())
		}

		next, stop := iter.Pull2(resourceIter)
		defer stop()

		_, _, ok := next()
		if ok {
			t.Error("expected no resources to be yielded")
		}
	})

	t.Run("import by name mismatch filter", func(t *testing.T) {
		filter := orcv1alpha1.SwiftContainerFilter{
			Name:          ptr.To(orcv1alpha1.OpenStackName("legacy-archive")),
			StoragePolicy: ptr.To("GoldPolicy"),
		}

		headers := map[string]string{
			"X-Storage-Policy": "SilverPolicy", // mismatch
		}

		swiftClient.EXPECT().GetContainer(gomock.Any(), "legacy-archive").Return(headers, nil)

		resourceIter, status := actuator.ListOSResourcesForImport(context.Background(), nil, filter)
		if status != nil {
			t.Fatalf("unexpected status: %v", status.GetError())
		}

		next, stop := iter.Pull2(resourceIter)
		defer stop()

		_, _, ok := next()
		if ok {
			t.Error("expected no resources to be yielded on filter mismatch")
		}
	})

	t.Run("import by name unexpected error", func(t *testing.T) {
		filter := orcv1alpha1.SwiftContainerFilter{
			Name: ptr.To(orcv1alpha1.OpenStackName("legacy-archive")),
		}

		errUnexpected := errors.New("some unexpected API error")
		swiftClient.EXPECT().GetContainer(gomock.Any(), "legacy-archive").Return(nil, errUnexpected)

		resourceIter, status := actuator.ListOSResourcesForImport(context.Background(), nil, filter)
		if status != nil {
			t.Fatalf("unexpected status: %v", status.GetError())
		}

		next, stop := iter.Pull2(resourceIter)
		defer stop()

		_, err, ok := next()
		if !ok {
			t.Fatal("expected yield to return ok with error")
		}
		if !errors.Is(err, errUnexpected) {
			t.Errorf("expected error %v, got %v", errUnexpected, err)
		}
	})

	t.Run("import list containers success", func(t *testing.T) {
		filter := orcv1alpha1.SwiftContainerFilter{
			StoragePolicy: ptr.To("GoldPolicy"),
		}

		containerList := []*containers.Container{
			{Name: "c1"},
			{Name: "c2"},
		}

		// ListContainers returns Seq2[*containers.Container, error]
		swiftClient.EXPECT().ListContainers(gomock.Any(), nil).Return(func(yield func(*containers.Container, error) bool) {
			for _, c := range containerList {
				if !yield(c, nil) {
					return
				}
			}
		})

		// c1 matches
		swiftClient.EXPECT().GetContainer(gomock.Any(), "c1").Return(map[string]string{
			"X-Storage-Policy": "GoldPolicy",
		}, nil)

		// c2 mismatch
		swiftClient.EXPECT().GetContainer(gomock.Any(), "c2").Return(map[string]string{
			"X-Storage-Policy": "SilverPolicy",
		}, nil)

		resourceIter, status := actuator.ListOSResourcesForImport(context.Background(), nil, filter)
		if status != nil {
			t.Fatalf("unexpected status: %v", status.GetError())
		}

		next, stop := iter.Pull2(resourceIter)
		defer stop()

		res, err, ok := next()
		if !ok {
			t.Fatal("expected c1 to be yielded")
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Name != "c1" {
			t.Errorf("expected Name c1, got %s", res.Name)
		}

		_, _, ok = next()
		if ok {
			t.Error("expected only one resource to be yielded")
		}
	})

	t.Run("import list containers error", func(t *testing.T) {
		filter := orcv1alpha1.SwiftContainerFilter{
			StoragePolicy: ptr.To("GoldPolicy"),
		}

		errUnexpected := errors.New("list error")
		swiftClient.EXPECT().ListContainers(gomock.Any(), nil).Return(func(yield func(*containers.Container, error) bool) {
			yield(nil, errUnexpected)
		})

		resourceIter, status := actuator.ListOSResourcesForImport(context.Background(), nil, filter)
		if status != nil {
			t.Fatalf("unexpected status: %v", status.GetError())
		}

		next, stop := iter.Pull2(resourceIter)
		defer stop()

		_, err, ok := next()
		if !ok {
			t.Fatal("expected yield to return ok with error")
		}
		if !errors.Is(err, errUnexpected) {
			t.Errorf("expected error %v, got %v", errUnexpected, err)
		}
	})
}

func TestGetResourceReconcilers_NoDrift(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				ReadACL:    ptr.To(".r:*"),
				WriteACL:   ptr.To("project_admin"),
				QuotaBytes: ptr.To(int64(1000)),
				QuotaCount: ptr.To(int64(50)),
				Metadata: map[string]string{
					"foo": "bar",
				},
			},
		},
		Status: orcv1alpha1.SwiftContainerStatus{
			Resource: &orcv1alpha1.SwiftContainerResourceStatus{
				ReadACL:    ".r:*",
				WriteACL:   "project_admin",
				QuotaBytes: ptr.To(int64(1000)),
				QuotaCount: ptr.To(int64(50)),
				Metadata: map[string]string{
					"foo": "bar",
				},
			},
		},
	}

	osResource := &osResourceT{
		Container: containers.Container{
			Name: "test-container",
		},
		Headers: map[string]string{
			"X-Container-Read":             ".r:*",
			"X-Container-Write":            "project_admin",
			"X-Container-Meta-Quota-Bytes": "1000",
			"X-Container-Meta-Quota-Count": "50",
			"X-Container-Meta-Foo":         "bar",
		},
	}

	reconcilers, rs := actuator.GetResourceReconcilers(context.Background(), orcObject, osResource, nil)
	if rs != nil {
		t.Fatalf("unexpected error: %v", rs.GetError())
	}
	if len(reconcilers) != 1 {
		t.Fatalf("expected 1 reconciler, got %d", len(reconcilers))
	}

	// Initial status resource pointer to check if it has changed
	initialStatus := orcObject.Status.Resource

	reconRS := reconcilers[0](context.Background(), orcObject, osResource)
	if reconRS != nil {
		t.Fatalf("unexpected reconcile status: %v", reconRS.GetError())
	}

	if orcObject.Status.Resource != initialStatus {
		t.Error("expected Status.Resource to not be modified on no drift")
	}
}

func TestGetResourceReconcilers_DriftDetected(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				ReadACL:    ptr.To(".r:*"),
				WriteACL:   ptr.To("project_admin"),
				QuotaBytes: ptr.To(int64(1000)),
				QuotaCount: ptr.To(int64(50)),
				Metadata: map[string]string{
					"foo": "bar",
				},
			},
		},
	}

	osResource := &osResourceT{
		Container: containers.Container{
			Name: "test-container",
		},
		Headers: map[string]string{
			"X-Container-Read":             ".r:-internal.com", // drifted
			"X-Container-Write":            "admin_only",       // drifted
			"X-Container-Meta-Quota-Bytes": "2000",             // drifted
			"X-Container-Meta-Quota-Count": "100",              // drifted
			"X-Container-Meta-Foo":         "baz",              // drifted
		},
	}

	reconcilers, rs := actuator.GetResourceReconcilers(context.Background(), orcObject, osResource, nil)
	if rs != nil {
		t.Fatalf("unexpected error: %v", rs.GetError())
	}
	if len(reconcilers) != 1 {
		t.Fatalf("expected 1 reconciler, got %d", len(reconcilers))
	}

	reconRS := reconcilers[0](context.Background(), orcObject, osResource)
	if reconRS != nil {
		t.Fatalf("unexpected reconcile status: %v", reconRS.GetError())
	}

	status := orcObject.Status.Resource
	if status == nil {
		t.Fatal("expected Status.Resource to be populated")
	}
	if status.ReadACL != ".r:-internal.com" {
		t.Errorf("expected ReadACL .r:-internal.com, got %s", status.ReadACL)
	}
	if status.WriteACL != "admin_only" {
		t.Errorf("expected WriteACL admin_only, got %s", status.WriteACL)
	}
	if status.QuotaBytes == nil || *status.QuotaBytes != 2000 {
		t.Errorf("expected QuotaBytes 2000, got %v", status.QuotaBytes)
	}
	if status.QuotaCount == nil || *status.QuotaCount != 100 {
		t.Errorf("expected QuotaCount 100, got %v", status.QuotaCount)
	}
	if status.Metadata["foo"] != "baz" {
		t.Errorf("expected Metadata foo=baz, got %v", status.Metadata)
	}
}

func TestGetResourceReconcilers_DriftOnReadACL(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				ReadACL: ptr.To(".r:*"),
			},
		},
	}

	osResource := &osResourceT{
		Container: containers.Container{Name: "test-container"},
		Headers: map[string]string{
			"X-Container-Read": ".r:-internal.com",
		},
	}

	reconcilers, rs := actuator.GetResourceReconcilers(context.Background(), orcObject, osResource, nil)
	if rs != nil {
		t.Fatalf("unexpected error: %v", rs.GetError())
	}

	reconRS := reconcilers[0](context.Background(), orcObject, osResource)
	if reconRS != nil {
		t.Fatalf("unexpected reconcile status: %v", reconRS.GetError())
	}

	if orcObject.Status.Resource == nil || orcObject.Status.Resource.ReadACL != ".r:-internal.com" {
		t.Errorf("expected status ReadACL to be .r:-internal.com, got %v", orcObject.Status.Resource)
	}
}

func TestGetResourceReconcilers_DriftOnWriteACL(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				WriteACL: ptr.To("project_admin"),
			},
		},
	}

	osResource := &osResourceT{
		Container: containers.Container{Name: "test-container"},
		Headers: map[string]string{
			"X-Container-Write": "admin_only",
		},
	}

	reconcilers, rs := actuator.GetResourceReconcilers(context.Background(), orcObject, osResource, nil)
	if rs != nil {
		t.Fatalf("unexpected error: %v", rs.GetError())
	}

	reconRS := reconcilers[0](context.Background(), orcObject, osResource)
	if reconRS != nil {
		t.Fatalf("unexpected reconcile status: %v", reconRS.GetError())
	}

	if orcObject.Status.Resource == nil || orcObject.Status.Resource.WriteACL != "admin_only" {
		t.Errorf("expected status WriteACL to be admin_only, got %v", orcObject.Status.Resource)
	}
}

func TestGetResourceReconcilers_DriftOnStoragePolicy(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				StoragePolicy: ptr.To("GoldPolicy"),
			},
		},
	}

	osResource := &osResourceT{
		Container: containers.Container{Name: "test-container"},
		Headers: map[string]string{
			"X-Storage-Policy": "SilverPolicy",
		},
	}

	reconcilers, rs := actuator.GetResourceReconcilers(context.Background(), orcObject, osResource, nil)
	if rs != nil {
		t.Fatalf("unexpected error: %v", rs.GetError())
	}

	reconRS := reconcilers[0](context.Background(), orcObject, osResource)
	if reconRS != nil {
		t.Fatalf("unexpected reconcile status: %v", reconRS.GetError())
	}

	if orcObject.Status.Resource == nil || orcObject.Status.Resource.StoragePolicy != "SilverPolicy" {
		t.Errorf("expected status StoragePolicy to be SilverPolicy, got %v", orcObject.Status.Resource)
	}
}

func TestGetResourceReconcilers_DriftOnQuotaBytes(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				QuotaBytes: ptr.To(int64(1000)),
			},
		},
	}

	osResource := &osResourceT{
		Container: containers.Container{Name: "test-container"},
		Headers: map[string]string{
			"X-Container-Meta-Quota-Bytes": "2000",
		},
	}

	reconcilers, rs := actuator.GetResourceReconcilers(context.Background(), orcObject, osResource, nil)
	if rs != nil {
		t.Fatalf("unexpected error: %v", rs.GetError())
	}

	reconRS := reconcilers[0](context.Background(), orcObject, osResource)
	if reconRS != nil {
		t.Fatalf("unexpected reconcile status: %v", reconRS.GetError())
	}

	if orcObject.Status.Resource == nil || orcObject.Status.Resource.QuotaBytes == nil || *orcObject.Status.Resource.QuotaBytes != 2000 {
		t.Errorf("expected status QuotaBytes to be 2000, got %v", orcObject.Status.Resource)
	}
}

func TestGetResourceReconcilers_DriftOnQuotaCount(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				QuotaCount: ptr.To(int64(50)),
			},
		},
	}

	osResource := &osResourceT{
		Container: containers.Container{Name: "test-container"},
		Headers: map[string]string{
			"X-Container-Meta-Quota-Count": "100",
		},
	}

	reconcilers, rs := actuator.GetResourceReconcilers(context.Background(), orcObject, osResource, nil)
	if rs != nil {
		t.Fatalf("unexpected error: %v", rs.GetError())
	}

	reconRS := reconcilers[0](context.Background(), orcObject, osResource)
	if reconRS != nil {
		t.Fatalf("unexpected reconcile status: %v", reconRS.GetError())
	}

	if orcObject.Status.Resource == nil || orcObject.Status.Resource.QuotaCount == nil || *orcObject.Status.Resource.QuotaCount != 100 {
		t.Errorf("expected status QuotaCount to be 100, got %v", orcObject.Status.Resource)
	}
}

func TestGetResourceReconcilers_DriftOnMetadata(t *testing.T) {
	mockctrl := gomock.NewController(t)
	defer mockctrl.Finish()

	swiftClient := mock.NewMockSwiftContainerClient(mockctrl)
	actuator := swiftcontainerActuator{osClient: swiftClient}

	orcObject := &orcv1alpha1.SwiftContainer{
		Spec: orcv1alpha1.SwiftContainerSpec{
			Resource: &orcv1alpha1.SwiftContainerResourceSpec{
				Metadata: map[string]string{
					"owner": "devops",
				},
			},
		},
	}

	osResource := &osResourceT{
		Container: containers.Container{Name: "test-container"},
		Headers: map[string]string{
			"X-Container-Meta-Owner": "platform",
		},
	}

	reconcilers, rs := actuator.GetResourceReconcilers(context.Background(), orcObject, osResource, nil)
	if rs != nil {
		t.Fatalf("unexpected error: %v", rs.GetError())
	}

	reconRS := reconcilers[0](context.Background(), orcObject, osResource)
	if reconRS != nil {
		t.Fatalf("unexpected reconcile status: %v", reconRS.GetError())
	}

	if orcObject.Status.Resource == nil || orcObject.Status.Resource.Metadata["owner"] != "platform" {
		t.Errorf("expected status Metadata owner=platform, got %v", orcObject.Status.Resource)
	}
}
