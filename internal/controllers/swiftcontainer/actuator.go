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
	"fmt"
	"iter"
	"strconv"
	"strings"

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"

	orcv1alpha1 "github.com/k-orc/openstack-resource-controller/v3/api/v1alpha1"
	"github.com/k-orc/openstack-resource-controller/v3/internal/controllers/generic/interfaces"
	"github.com/k-orc/openstack-resource-controller/v3/internal/controllers/generic/progress"
	"github.com/k-orc/openstack-resource-controller/v3/internal/logging"
	"github.com/k-orc/openstack-resource-controller/v3/internal/osclients"
	orcerrors "github.com/k-orc/openstack-resource-controller/v3/internal/util/errors"
)

type osResource struct {
	containers.Container
	Headers map[string]string
}

// OpenStack resource types
type (
	osResourceT = osResource

	createResourceActuator = interfaces.CreateResourceActuator[orcObjectPT, orcObjectT, filterT, osResourceT]
	deleteResourceActuator = interfaces.DeleteResourceActuator[orcObjectPT, orcObjectT, osResourceT]
	helperFactory          = interfaces.ResourceHelperFactory[orcObjectPT, orcObjectT, resourceSpecT, filterT, osResourceT]
)

type swiftcontainerHelperFactory struct{}

var _ helperFactory = swiftcontainerHelperFactory{}

func (swiftcontainerHelperFactory) NewAPIObjectAdapter(orcObject orcObjectPT) interfaces.APIObjectAdapter[orcObjectPT, resourceSpecT, filterT] {
	return swiftcontainerAdapter{orcObject}
}

func (swiftcontainerHelperFactory) NewCreateActuator(ctx context.Context, orcObject orcObjectPT, controller interfaces.ResourceController) (createResourceActuator, progress.ReconcileStatus) {
	return newActuator(ctx, orcObject, controller)
}

func (swiftcontainerHelperFactory) NewDeleteActuator(ctx context.Context, orcObject orcObjectPT, controller interfaces.ResourceController) (deleteResourceActuator, progress.ReconcileStatus) {
	return newActuator(ctx, orcObject, controller)
}

type swiftcontainerActuator struct {
	osClient osclients.SwiftContainerClient
}

var _ createResourceActuator = swiftcontainerActuator{}
var _ deleteResourceActuator = swiftcontainerActuator{}
var _ interfaces.ReconcileResourceActuator[orcObjectPT, osResourceT] = swiftcontainerActuator{}

func newActuator(ctx context.Context, orcObject *orcv1alpha1.SwiftContainer, controller interfaces.ResourceController) (swiftcontainerActuator, progress.ReconcileStatus) {
	log := ctrl.LoggerFrom(ctx)

	// Ensure credential secrets exist and have our finalizer
	_, reconcileStatus := credentialsDependency.RequireDependencies(ctx, controller.GetK8sClient(), orcObject, func(*corev1.Secret) bool { return true })
	if needsReschedule, _ := reconcileStatus.NeedsReschedule(); needsReschedule {
		return swiftcontainerActuator{}, reconcileStatus
	}

	clientScope, err := controller.GetScopeFactory().NewClientScopeFromObject(ctx, controller.GetK8sClient(), log, orcObject)
	if err != nil {
		return swiftcontainerActuator{}, progress.WrapError(err)
	}
	osClient, err := clientScope.NewSwiftContainerClient()
	if err != nil {
		return swiftcontainerActuator{}, progress.WrapError(err)
	}

	return swiftcontainerActuator{
		osClient: osClient,
	}, nil
}

func (swiftcontainerActuator) GetResourceID(osResource *osResourceT) string {
	return osResource.Name
}

func (actuator swiftcontainerActuator) GetOSResourceByID(ctx context.Context, id string) (*osResourceT, progress.ReconcileStatus) {
	headers, err := actuator.osClient.GetContainer(ctx, id)
	if err != nil {
		return nil, progress.WrapError(err)
	}

	var count, bytes int64
	for k, v := range headers {
		lowerK := strings.ToLower(k)
		if lowerK == "x-container-object-count" {
			if c, err := strconv.ParseInt(v, 10, 64); err == nil {
				count = c
			}
		} else if lowerK == "x-container-bytes-used" {
			if b, err := strconv.ParseInt(v, 10, 64); err == nil {
				bytes = b
			}
		}
	}

	return &osResource{
		Container: containers.Container{
			Name:  id,
			Count: count,
			Bytes: bytes,
		},
		Headers: headers,
	}, nil
}

func (actuator swiftcontainerActuator) ListOSResourcesForAdoption(ctx context.Context, orcObject orcObjectPT) (iter.Seq2[*osResourceT, error], bool) {
	resourceSpec := orcObject.Spec.Resource
	if resourceSpec == nil {
		return nil, false
	}

	name := getResourceName(orcObject)

	return func(yield func(*osResourceT, error) bool) {
		res, status := actuator.GetOSResourceByID(ctx, name)
		if status != nil {
			if _, err := status.NeedsReschedule(); err != nil {
				if orcerrors.IsNotFound(err) {
					// Container doesn't exist, which is fine, so adoption yields nothing.
					return
				}
				yield(nil, err)
			}
			return
		}

		if !MatchExistingContainer(resourceSpec, res.Headers) {
			yield(nil, orcerrors.Terminal(
				orcv1alpha1.ConditionReasonUnrecoverableError,
				"existing container configuration mismatches spec",
				fmt.Errorf("configuration mismatch for container %q", name),
			))
			return
		}

		yield(res, nil)
	}, true
}

func (actuator swiftcontainerActuator) ListOSResourcesForImport(ctx context.Context, obj orcObjectPT, filter filterT) (iter.Seq2[*osResourceT, error], progress.ReconcileStatus) {
	// If name is explicitly provided in the import filter, we can optimize and fetch only that container.
	if filter.Name != nil {
		return func(yield func(*osResourceT, error) bool) {
			res, status := actuator.GetOSResourceByID(ctx, string(*filter.Name))
			if status != nil {
				if _, err := status.NeedsReschedule(); err != nil && !orcerrors.IsNotFound(err) {
					yield(nil, err)
				}
				return
			}
			if res != nil && MatchFilter(&filter, res.Headers) {
				yield(res, nil)
			}
		}, nil
	}

	// Otherwise, we list all containers and fetch details for each to apply filters
	return func(yield func(*osResourceT, error) bool) {
		for c, err := range actuator.osClient.ListContainers(ctx, nil) {
			if err != nil {
				yield(nil, err)
				return
			}
			res, status := actuator.GetOSResourceByID(ctx, c.Name)
			if status != nil {
				if _, err := status.NeedsReschedule(); err != nil {
					if orcerrors.IsNotFound(err) {
						continue
					}
					yield(nil, err)
					return
				}
			}
			if res != nil && MatchFilter(&filter, res.Headers) {
				if !yield(res, nil) {
					return
				}
			}
		}
	}, nil
}

func (actuator swiftcontainerActuator) CreateResource(ctx context.Context, orcObject orcObjectPT) (*osResourceT, progress.ReconcileStatus) {
	resource := orcObject.Spec.Resource
	name := getResourceName(orcObject)

	metadata := make(map[string]string)
	if resource.Metadata != nil {
		for k, v := range resource.Metadata {
			metadata[k] = v
		}
	}
	if resource.QuotaBytes != nil {
		metadata["Quota-Bytes"] = strconv.FormatInt(*resource.QuotaBytes, 10)
	}
	if resource.QuotaCount != nil {
		metadata["Quota-Count"] = strconv.FormatInt(*resource.QuotaCount, 10)
	}

	createOpts := containers.CreateOpts{
		ContainerRead:  ptr.Deref(resource.ReadACL, ""),
		ContainerWrite: ptr.Deref(resource.WriteACL, ""),
		StoragePolicy:  ptr.Deref(resource.StoragePolicy, ""),
		Metadata:       metadata,
	}

	headers, err := actuator.osClient.CreateContainer(ctx, name, createOpts)
	if err != nil {
		if !orcerrors.IsRetryable(err) {
			err = orcerrors.Terminal(orcv1alpha1.ConditionReasonInvalidConfiguration, "invalid configuration creating resource: "+err.Error(), err)
		}
		return nil, progress.WrapError(err)
	}

	var count, bytes int64
	for k, v := range headers {
		lowerK := strings.ToLower(k)
		if lowerK == "x-container-object-count" {
			if c, err := strconv.ParseInt(v, 10, 64); err == nil {
				count = c
			}
		} else if lowerK == "x-container-bytes-used" {
			if b, err := strconv.ParseInt(v, 10, 64); err == nil {
				bytes = b
			}
		}
	}

	return &osResource{
		Container: containers.Container{
			Name:  name,
			Count: count,
			Bytes: bytes,
		},
		Headers: headers,
	}, nil
}

func (actuator swiftcontainerActuator) DeleteResource(ctx context.Context, orcObject orcObjectPT, osResource *osResourceT) progress.ReconcileStatus {
	if osResource.Count > 0 {
		err := fmt.Errorf("Cannot delete container: Swift container is not empty.")
		return progress.WrapError(orcerrors.Terminal(orcv1alpha1.ConditionReasonUnrecoverableError, err.Error(), err))
	}
	return progress.WrapError(actuator.osClient.DeleteContainer(ctx, osResource.Name))
}

func (actuator swiftcontainerActuator) GetResourceReconcilers(ctx context.Context, orcObject orcObjectPT, osResource *osResourceT, controller interfaces.ResourceController) ([]interfaces.ResourceReconciler[orcObjectPT, osResourceT], progress.ReconcileStatus) {
	return []interfaces.ResourceReconciler[orcObjectPT, osResourceT]{
		actuator.monitorDrift,
	}, nil
}

func (actuator swiftcontainerActuator) monitorDrift(ctx context.Context, orcObject orcObjectPT, osResource *osResourceT) progress.ReconcileStatus {
	log := ctrl.LoggerFrom(ctx)
	spec := orcObject.Spec.Resource
	if spec == nil {
		return nil
	}

	status := osclients.ParseSwiftHeaders(osResource.Headers)
	drifted := false

	if spec.ReadACL != nil && status.ReadACL != *spec.ReadACL {
		log.V(logging.Info).Info("Drift detected on ReadACL", "desired", *spec.ReadACL, "observed", status.ReadACL)
		drifted = true
	}

	if spec.WriteACL != nil && status.WriteACL != *spec.WriteACL {
		log.V(logging.Info).Info("Drift detected on WriteACL", "desired", *spec.WriteACL, "observed", status.WriteACL)
		drifted = true
	}

	if spec.StoragePolicy != nil && status.StoragePolicy != *spec.StoragePolicy {
		log.V(logging.Info).Info("Drift detected on StoragePolicy", "desired", *spec.StoragePolicy, "observed", status.StoragePolicy)
		drifted = true
	}

	if spec.QuotaBytes != nil {
		if status.QuotaBytes == nil || *status.QuotaBytes != *spec.QuotaBytes {
			observed := int64(0)
			if status.QuotaBytes != nil {
				observed = *status.QuotaBytes
			}
			log.V(logging.Info).Info("Drift detected on QuotaBytes", "desired", *spec.QuotaBytes, "observed", observed)
			drifted = true
		}
	}

	if spec.QuotaCount != nil {
		if status.QuotaCount == nil || *status.QuotaCount != *spec.QuotaCount {
			observed := int64(0)
			if status.QuotaCount != nil {
				observed = *status.QuotaCount
			}
			log.V(logging.Info).Info("Drift detected on QuotaCount", "desired", *spec.QuotaCount, "observed", observed)
			drifted = true
		}
	}

	for k, v := range spec.Metadata {
		observedVal, exists := status.Metadata[strings.ToLower(k)]
		if !exists || observedVal != v {
			log.V(logging.Info).Info("Drift detected on Metadata", "key", k, "desired", v, "observed", observedVal)
			drifted = true
		}
	}

	if drifted {
		log.V(logging.Info).Info("Out-of-band drift detected in Swift container, updating status to match remote configuration")
		orcObject.Status.Resource = &status
		orcObject.Status.Resource.Name = osResource.Name
	}

	return nil
}

func getHeader(headers map[string]string, key string) string {
	lowerKey := strings.ToLower(key)
	for k, v := range headers {
		if strings.ToLower(k) == lowerKey {
			return v
		}
	}
	return ""
}

func MatchExistingContainer(spec *orcv1alpha1.SwiftContainerResourceSpec, observedHeaders map[string]string) bool {
	if spec == nil {
		return true
	}

	if spec.StoragePolicy != nil {
		observedStoragePolicy := getHeader(observedHeaders, "X-Storage-Policy")
		if *spec.StoragePolicy != observedStoragePolicy {
			return false
		}
	}

	if spec.ReadACL != nil {
		observedReadACL := getHeader(observedHeaders, "X-Container-Read")
		if *spec.ReadACL != observedReadACL {
			return false
		}
	}

	if spec.WriteACL != nil {
		observedWriteACL := getHeader(observedHeaders, "X-Container-Write")
		if *spec.WriteACL != observedWriteACL {
			return false
		}
	}

	if spec.QuotaBytes != nil {
		observedQuotaBytes := getHeader(observedHeaders, "X-Container-Meta-Quota-Bytes")
		var qb int64
		if observedQuotaBytes != "" {
			var err error
			qb, err = strconv.ParseInt(observedQuotaBytes, 10, 64)
			if err != nil {
				return false
			}
		}
		if *spec.QuotaBytes != qb {
			return false
		}
	}

	if spec.QuotaCount != nil {
		observedQuotaCount := getHeader(observedHeaders, "X-Container-Meta-Quota-Count")
		var qc int64
		if observedQuotaCount != "" {
			var err error
			qc, err = strconv.ParseInt(observedQuotaCount, 10, 64)
			if err != nil {
				return false
			}
		}
		if *spec.QuotaCount != qc {
			return false
		}
	}

	for k, v := range spec.Metadata {
		observedMetaVal := getHeader(observedHeaders, "X-Container-Meta-"+k)
		if v != observedMetaVal {
			return false
		}
	}

	return true
}

func MatchFilter(filter *orcv1alpha1.SwiftContainerFilter, observedHeaders map[string]string) bool {
	if filter == nil {
		return true
	}

	if filter.StoragePolicy != nil {
		observedStoragePolicy := getHeader(observedHeaders, "X-Storage-Policy")
		if *filter.StoragePolicy != observedStoragePolicy {
			return false
		}
	}

	if filter.ReadACL != nil {
		observedReadACL := getHeader(observedHeaders, "X-Container-Read")
		if *filter.ReadACL != observedReadACL {
			return false
		}
	}

	if filter.WriteACL != nil {
		observedWriteACL := getHeader(observedHeaders, "X-Container-Write")
		if *filter.WriteACL != observedWriteACL {
			return false
		}
	}

	if filter.QuotaBytes != nil {
		observedQuotaBytes := getHeader(observedHeaders, "X-Container-Meta-Quota-Bytes")
		var qb int64
		if observedQuotaBytes != "" {
			var err error
			qb, err = strconv.ParseInt(observedQuotaBytes, 10, 64)
			if err != nil {
				return false
			}
		}
		if *filter.QuotaBytes != qb {
			return false
		}
	}

	if filter.QuotaCount != nil {
		observedQuotaCount := getHeader(observedHeaders, "X-Container-Meta-Quota-Count")
		var qc int64
		if observedQuotaCount != "" {
			var err error
			qc, err = strconv.ParseInt(observedQuotaCount, 10, 64)
			if err != nil {
				return false
			}
		}
		if *filter.QuotaCount != qc {
			return false
		}
	}

	for k, v := range filter.Metadata {
		observedMetaVal := getHeader(observedHeaders, "X-Container-Meta-"+k)
		if v != observedMetaVal {
			return false
		}
	}

	return true
}
