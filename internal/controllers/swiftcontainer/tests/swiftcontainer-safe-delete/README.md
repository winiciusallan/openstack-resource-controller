# SwiftContainer safe deletion test

This scenario tests:
1. Deleting an empty container succeeds and removes the finalizer.
2. Deleting a non-empty container blocks finalizer removal and raises an UnrecoverableError.
3. Emptying the container allows the deletion to proceed and succeed.
