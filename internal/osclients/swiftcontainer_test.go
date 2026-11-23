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

package osclients

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"k8s.io/utils/ptr"

	orcv1alpha1 "github.com/k-orc/openstack-resource-controller/v3/api/v1alpha1"
)

func TestParseSwiftHeaders(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected orcv1alpha1.SwiftContainerResourceStatus
	}{
		{
			name: "all headers",
			headers: map[string]string{
				"X-Container-Read":             ".r:*",
				"X-Container-Write":            "project_admin",
				"X-Storage-Policy":             "GoldPolicy",
				"X-Container-Object-Count":     "142",
				"X-Container-Bytes-Used":       "34910283",
				"X-Container-Meta-Quota-Bytes": "10737418240",
				"X-Container-Meta-Quota-Count": "50000",
				"X-Container-Meta-Owner":       "devops",
				"X-Container-Meta-Tier":        "prod",
			},
			expected: orcv1alpha1.SwiftContainerResourceStatus{
				ReadACL:       ".r:*",
				WriteACL:      "project_admin",
				StoragePolicy: "GoldPolicy",
				ObjectCount:   ptr.To(int64(142)),
				BytesUsed:     ptr.To(int64(34910283)),
				QuotaBytes:    ptr.To(int64(10737418240)),
				QuotaCount:    ptr.To(int64(50000)),
				Metadata: map[string]string{
					"owner": "devops",
					"tier":  "prod",
				},
			},
		},
		{
			name: "case insensitivity and missing fields",
			headers: map[string]string{
				"x-container-read":             ".r:web",
				"x-storage-policy":             "Standard",
				"X-CONTAINER-META-QUOTA-BYTES": "12345",
				"x-container-meta-custom":      "value",
			},
			expected: orcv1alpha1.SwiftContainerResourceStatus{
				ReadACL:       ".r:web",
				StoragePolicy: "Standard",
				QuotaBytes:    ptr.To(int64(12345)),
				Metadata: map[string]string{
					"custom": "value",
				},
			},
		},
		{
			name:    "empty headers",
			headers: map[string]string{},
			expected: orcv1alpha1.SwiftContainerResourceStatus{
				Metadata: nil,
			},
		},
		{
			name: "invalid integer parsing",
			headers: map[string]string{
				"X-Container-Object-Count":     "invalid",
				"X-Container-Bytes-Used":       "invalid",
				"X-Container-Meta-Quota-Bytes": "invalid",
				"X-Container-Meta-Quota-Count": "invalid",
			},
			expected: orcv1alpha1.SwiftContainerResourceStatus{
				Metadata: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ParseSwiftHeaders(tt.headers)
			if !reflect.DeepEqual(actual, tt.expected) {
				t.Errorf("ParseSwiftHeaders() = %+v, want %+v", actual, tt.expected)
			}
		})
	}
}

func TestSwiftContainerErrorClient(t *testing.T) {
	wantErr := errors.New("mock swift container error")
	client := NewSwiftContainerErrorClient(wantErr)

	if _, err := client.GetContainer(t.Context(), "test"); !errors.Is(err, wantErr) {
		t.Errorf("GetContainer() error = %v, want %v", err, wantErr)
	}

	if _, err := client.CreateContainer(t.Context(), "test", nil); !errors.Is(err, wantErr) {
		t.Errorf("CreateContainer() error = %v, want %v", err, wantErr)
	}

	if err := client.DeleteContainer(t.Context(), "test"); !errors.Is(err, wantErr) {
		t.Errorf("DeleteContainer() error = %v, want %v", err, wantErr)
	}

	it := client.ListContainers(t.Context(), nil)
	it(func(c *containers.Container, err error) bool {
		if !errors.Is(err, wantErr) {
			t.Errorf("ListContainers() error = %v, want %v", err, wantErr)
		}
		return false
	})
}
