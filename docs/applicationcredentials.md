
# ApplicationCredential Controller



This document provides a brief overview of the Keystone ApplicationCredential (AC) controller.



## General Information
AC controller watches `KeystoneApplicationCredential` custom resources (CR) and performs these actions:

1. **Create** or **rotate** [ACs in Keystone](https://docs.openstack.org/keystone/2023.1/user/application_credentials.html)
2. **Store** the generated AC ID and AC secret in a k8s `Secret`
3. **Reconcile** the CR’s status (AC ID, creation/expiry time, conditions)

Note: AC controller is not responsible for generating AC CR for a service, that responsibility falls under the openstack-operator. AC controller merely watches/updates AC CR.

## API Specification

### KeystoneApplicationCredentialSpec
```yaml
spec:
  # Secret containing service user password (default: osp-secret)
  secret: osp-secret
  # PasswordSelector for extracting the service password
  passwordSelector: ServicePassword
  # UserName - the Keystone user under which this ApplicationCredential is created
  userName: barbican
  # ExpirationDays sets the lifetime in days (default: 365, minimum: 2)
  expirationDays: 365
  # GracePeriodDays sets rotation window (default: 182, minimum: 1)
  # Must be smaller than expirationDays
  gracePeriodDays: 182
  # Roles to assign to the ApplicationCredential (minimum: 1 role)
  roles:
    - service
  # Unrestricted indicates whether AC may create/destroy other credentials
  unrestricted: false
  # AccessRules defines which services the AC is permitted to access
  accessRules:
    - service: compute
      path: /servers
      method: GET
    - service: image
      path: /images
      method: GET
```

### KeystoneApplicationCredentialStatus
```yaml
status:
  # ACID - the ID in Keystone for this ApplicationCredential
  ACID: "7b23dbac20bc4f048f937415c84bb329"
  # SecretName - name of the k8s Secret storing the ApplicationCredential secret
  secretName: "ac-barbican-secret"
  # CreatedAt - timestamp of creation
  createdAt: "2025-05-29T09:02:28Z"
  # ExpiresAt - time of validity expiration
  expiresAt: "2026-05-29T09:02:28Z"
  # RotationEligibleAt - when rotation becomes eligible (ExpiresAt - GracePeriodDays)
  rotationEligibleAt: "2025-11-29T09:02:28Z"
  # LastRotated - timestamp when credentials were last rotated (only set after first rotation)
  lastRotated: "2025-05-29T09:02:28Z"
  # Conditions
  conditions:
    - type: Ready
      status: "True"
    - type: KeystoneAPIReady
      status: "True"
    - type: KeystoneApplicationCredentialReady
      status: "True"
```

## Controller's Reconciliation Logic

When the openstack-operator generates a new AC CR for Barbican `oc get appcred ac-barbican -n openstack`:

```yaml
apiVersion: keystone.openstack.org/v1beta1
kind: KeystoneApplicationCredential
metadata:
  name: ac-barbican
  namespace: openstack
spec:
  expirationDays: 365
  gracePeriodDays: 182
  passwordSelector: BarbicanPassword
  roles:
    - service
  secret: osp-secret
  unrestricted: false
  userName: barbican
```

the AC controller:
1. Reconcile() invoked
   - Controller-runtime calls `Reconcile()` with `ac-barbican`

2. Finalizer & Conditions
   - Adds `openstack.org/applicationcredential` finalizer
   - Initializes status conditions (`KeystoneAPIReady`, `KeystoneApplicationCredentialReady`, `Ready`)

3. Wait for KeystoneAPI
   - If missing or not ready, marks condition and requeues

4. Determine Rotation Need
   - `needsRotation()` returns `true` because no AC ID exists yet

5. Service‐Scoped Client
   - Retrieves `BarbicanPassword` from `osp-secret` (or any other specified in the AC CR)
   - Authenticates as `barbican` user

6. Extract User ID from Token
   - Extracts the user ID from the service user's own authentication token
   - Uses the authenticated service user's token to get its own Keystone user ID

7. Create AC in Keystone
   - Calls gophercloud's `applicationcredentials.Create(...)`
   - Includes access rules if specified in the CR

8. Store Secret
   - Creates a k8s `Secret` named `ac-barbican-secret`
   - Adds `openstack.org/ac-secret-protection` finalizer to the Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ac-barbican-secret
  namespace: openstack
  finalizers:
    - openstack.org/ac-secret-protection
data:
  AC_ID:     <base64-of-AC-ID>
  AC_SECRET: <base64-of-AC-secret>
```

9. Update CR status
   - Sets `.status.ACID`, `.status.secretName`, `.status.createdAt`, `.status.expiresAt`, `.status.rotationEligibleAt`
   - Sets `.status.lastRotated` (only during rotation, not initial creation)
   - Marks AC CR ready
   - Emits an event for rotation to notify EDPM nodes

10. Requeue for Next Check
    - Calculates next reconcile at `expiresAt - gracePeriod`
    - If already in grace window, requeues immediately, otherwise requeues after 24 h

AC in Keystone side:
```
openstack application credential show 7b23dbac20bc4f048f937415c84bb329
+--------------+------------------------------------------------------------------------+
| Field        | Value                                                                  |
+--------------+------------------------------------------------------------------------+
| description  | Created by keystone-operator for AC CR openstack/ac-barbican (user...  |
| expires_at   | 2026-05-29T09:02:28.705012                                             |
| id           | 7b23dbac20bc4f048f937415c84bb329                                       |
| name         | ac-barbican-29090                                                      |
| project_id   | 24f05745bc2145c6a625b528ce21d7a3                                       |
| roles        | service                                                                |
| system       | None                                                                   |
| unrestricted | False                                                                  |
| user_id      | 2ecd25b38f0d432388ad8b838e46f36d                                       |
+--------------+------------------------------------------------------------------------+
```
Note: The actual AC `name` in Keystone includes a 5-character random suffix (e.g. `-29090`) to avoid collisions during rotation.

## Rotation
When the next reconcile hits the grace window (`now ≥ expiresAt - gracePeriodDays`), `needsRotation()` returns `true` again and the controller:

- Service-Scoped Client - authenticates as the service user
- Extract User ID from the service user's token
- Create New AC
  - Generates a new Keystone AC with a fresh 5-char suffix
  - Uses the same roles, unrestricted flag, access rules, and expirationDays
  - Does _not_ revoke the old AC, the old credential naturally expires
- Store Updated Secret
  - Overwrites the existing `ac-barbican-secret` with the new `AC_ID` and `AC_SECRET`
- Update Status
  - Replaces `.status.ACID`, `.status.createdAt`, `.status.expiresAt`, and `.status.rotationEligibleAt` with the new values
  - Sets `.status.lastRotated` to current timestamp
  - Re-marks AC CR ready
  - Emits an event to notify EDPM nodes about the rotation
- Requeue
  - Schedules the next check at `(newExpiresAt - gracePeriodDays)`

## Manual Rotation

Manual rotation can be triggered by patching the AC CR with an expiration timestamp in the past:

```bash
oc patch -n openstack keystoneapplicationcredential ac-barbican \
  --type=merge --subresource=status \
  -p '{"status":{"expiresAt":"2001-05-19T00:00:00Z"}}'
```

This triggers seamless rotation with one pod restart and no authentication fallback.

**Note:** Deleting the AC CR itself also triggers rotation, but causes a brief fallback to password authentication while the openstack-operator recreates the AC CR. This results in two pod restarts instead of one, because the service authentication type is based on the presence of AC Secret.

## ApplicationCredential Lifecycle and Cleanup

ApplicationCredentials in Keystone are **not automatically deleted** by the controller. This design decision prevents disrupting running services, especially EDPM nodes that actively use these credentials.

**Cleanup behavior:**
- **During rotation:** The old AC remains in Keystone and expires naturally based on its `expiresAt` timestamp. The new AC is created with fresh credentials.
- **When AC CR is deleted:** The ApplicationCredential remains in Keystone and continues to be valid until natural expiration.
- **Manual cleanup:** If immediate cleanup is required, operators can manually delete the AC from Keystone:

```bash
openstack application credential delete <ac-id>
```

This approach ensures that deleting the AC CR (intentionally or accidentally) does not cause immediate authentication failures across the control plane and EDPM deployments.

## Client-Side Helper Functions

Service operators can use these helper functions to consume ApplicationCredential data:

```go
import keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"

// Get standard AC Secret name for a service
secretName := keystonev1.GetACSecretName("barbican") // Returns "ac-barbican-secret"

// Get standard AC CR name for a service
crName := keystonev1.GetACCRName("barbican") // Returns "ac-barbican"

// Fetch AC data directly from the Secret
acData, err := keystonev1.GetApplicationCredentialFromSecret(
    ctx, client, namespace, serviceName)
if err != nil {
    // Handle error
}
if acData != nil {
    // Use acData.ID and acData.Secret
}
```

## Validation Rules

The API includes validation constraints:
- `gracePeriodDays` must be smaller than `expirationDays`
- `expirationDays` minimum value: 2
- `gracePeriodDays` minimum value: 1
- `roles` must contain at least 1 role
