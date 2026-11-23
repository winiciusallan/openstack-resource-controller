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

	"github.com/gophercloud/gophercloud/v2/openstack/objectstorage/v1/containers"

	"github.com/k-orc/openstack-resource-controller/v3/internal/controllers/generic/interfaces"
	"github.com/k-orc/openstack-resource-controller/v3/internal/controllers/generic/progress"
)

// OpenStack resource types
type (
	osResourceT = containers.Container

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
	return swiftcontainerActuator{}, nil
}

func (swiftcontainerHelperFactory) NewDeleteActuator(ctx context.Context, orcObject orcObjectPT, controller interfaces.ResourceController) (deleteResourceActuator, progress.ReconcileStatus) {
	return swiftcontainerActuator{}, nil
}

type swiftcontainerActuator struct{}

var _ createResourceActuator = swiftcontainerActuator{}
var _ deleteResourceActuator = swiftcontainerActuator{}

func (swiftcontainerActuator) GetResourceID(osResource *containers.Container) string {
	return osResource.Name
}

func (swiftcontainerActuator) GetOSResourceByID(ctx context.Context, id string) (*containers.Container, progress.ReconcileStatus) {
	return nil, nil
}

func (swiftcontainerActuator) ListOSResourcesForAdoption(ctx context.Context, orcObject orcObjectPT) (iter.Seq2[*containers.Container, error], bool) {
	return nil, false
}

func (swiftcontainerActuator) ListOSResourcesForImport(ctx context.Context, orcObject orcObjectPT, filter filterT) (iter.Seq2[*containers.Container, error], progress.ReconcileStatus) {
	return nil, nil
}

func (swiftcontainerActuator) CreateResource(ctx context.Context, orcObject orcObjectPT) (*containers.Container, progress.ReconcileStatus) {
	return nil, nil
}

func (swiftcontainerActuator) DeleteResource(ctx context.Context, orcObject orcObjectPT, osResource *containers.Container) progress.ReconcileStatus {
	return nil
}
