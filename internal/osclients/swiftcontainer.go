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
	"context"
	"fmt"
	"iter"
	"strconv"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/utils/v2/openstack/clientconfig"
	"k8s.io/utils/ptr"

	orcv1alpha1 "github.com/k-orc/openstack-resource-controller/v3/api/v1alpha1"
)

type SwiftContainerClient interface {
	GetContainer(ctx context.Context, name string) (map[string]string, error)
	CreateContainer(ctx context.Context, name string, opts containers.CreateOptsBuilder) (map[string]string, error)
	DeleteContainer(ctx context.Context, name string) error
	ListContainers(ctx context.Context, opts containers.ListOptsBuilder) iter.Seq2[*containers.Container, error]
}

type swiftContainerClient struct {
	client *gophercloud.ServiceClient
}

// NewSwiftContainerClient returns a new Swift container client.
func NewSwiftContainerClient(providerClient *gophercloud.ProviderClient, providerClientOpts *clientconfig.ClientOpts) (SwiftContainerClient, error) {
	client, err := openstack.NewObjectStorageV1(providerClient, gophercloud.EndpointOpts{
		Region:       providerClientOpts.RegionName,
		Availability: clientconfig.GetEndpointType(providerClientOpts.EndpointType),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create object storage service client: %w", err)
	}

	return &swiftContainerClient{client}, nil
}

func (c *swiftContainerClient) GetContainer(ctx context.Context, name string) (map[string]string, error) {
	res := containers.Get(ctx, c.client, name, nil)
	if res.Err != nil {
		return nil, res.Err
	}
	headers := make(map[string]string)
	for k, v := range res.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return headers, nil
}

func (c *swiftContainerClient) CreateContainer(ctx context.Context, name string, opts containers.CreateOptsBuilder) (map[string]string, error) {
	res := containers.Create(ctx, c.client, name, opts)
	if res.Err != nil {
		return nil, res.Err
	}
	headers := make(map[string]string)
	for k, v := range res.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return headers, nil
}

func (c *swiftContainerClient) DeleteContainer(ctx context.Context, name string) error {
	res := containers.Delete(ctx, c.client, name)
	return res.Err
}

func (c *swiftContainerClient) ListContainers(ctx context.Context, opts containers.ListOptsBuilder) iter.Seq2[*containers.Container, error] {
	pager := containers.List(c.client, opts)
	return listResources(ctx, pager, containers.ExtractInfo)
}

// ParseSwiftHeaders parses HTTP response headers from Swift into a SwiftContainerResourceStatus.
func ParseSwiftHeaders(headers map[string]string) orcv1alpha1.SwiftContainerResourceStatus {
	status := orcv1alpha1.SwiftContainerResourceStatus{
		Metadata: make(map[string]string),
	}

	for k, v := range headers {
		lowerK := strings.ToLower(k)
		switch lowerK {
		case "x-container-read":
			status.ReadACL = v
		case "x-container-write":
			status.WriteACL = v
		case "x-storage-policy":
			status.StoragePolicy = v
		case "x-container-object-count":
			if count, err := strconv.ParseInt(v, 10, 64); err == nil {
				status.ObjectCount = ptr.To(count)
			}
		case "x-container-bytes-used":
			if bytes, err := strconv.ParseInt(v, 10, 64); err == nil {
				status.BytesUsed = ptr.To(bytes)
			}
		default:
			if strings.HasPrefix(lowerK, "x-container-meta-") {
				metaKey := strings.TrimPrefix(lowerK, "x-container-meta-")
				if metaKey == "quota-bytes" {
					if qb, err := strconv.ParseInt(v, 10, 64); err == nil {
						status.QuotaBytes = ptr.To(qb)
					}
				} else if metaKey == "quota-count" {
					if qc, err := strconv.ParseInt(v, 10, 64); err == nil {
						status.QuotaCount = ptr.To(qc)
					}
				} else {
					status.Metadata[metaKey] = v
				}
			}
		}
	}

	if len(status.Metadata) == 0 {
		status.Metadata = nil
	}

	return status
}

type swiftContainerErrorClient struct {
	error
}

// NewSwiftContainerErrorClient returns a SwiftContainerClient in which every method returns the given error.
func NewSwiftContainerErrorClient(e error) SwiftContainerClient {
	return swiftContainerErrorClient{e}
}

func (e swiftContainerErrorClient) GetContainer(_ context.Context, _ string) (map[string]string, error) {
	return nil, e.error
}

func (e swiftContainerErrorClient) CreateContainer(_ context.Context, _ string, _ containers.CreateOptsBuilder) (map[string]string, error) {
	return nil, e.error
}

func (e swiftContainerErrorClient) DeleteContainer(_ context.Context, _ string) error {
	return e.error
}

func (e swiftContainerErrorClient) ListContainers(_ context.Context, _ containers.ListOptsBuilder) iter.Seq2[*containers.Container, error] {
	return func(yield func(*containers.Container, error) bool) {
		yield(nil, e.error)
	}
}
