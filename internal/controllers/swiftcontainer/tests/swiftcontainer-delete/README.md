# SwiftContainer Deletion Policies Test

This scenario tests the standard delete option vs the detach option when deleting a managed SwiftContainer resource.
- `onDelete: detach` should keep the container in OpenStack, verified by re-importing it.
- `onDelete: delete` should delete the container from OpenStack, verified by trying to import it and failing to find it.
