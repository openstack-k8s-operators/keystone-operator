
# ApplicationCredential Controller



This document provides a brief overview of the Keystone ApplicationCredential (AC) controller.



## General Information
AC controller watches `KeystoneApplicationCredential` custom resources (CR) and performs these actions:

1. **Create** or **rotate** [ACs in Keystone](https://docs.openstack.org/keystone/2023.1/user/application_credentials.html)
2. **Store** the generated AC ID and AC secret in a k8s `Secret`
3. **Reconcile** the CR’s status (AC ID, creation/expiry time, conditions)

Note: AC controller is not responsible for generating AC CR a service, that responsibility falls under the openstack-operator. AC controller merely watches/updates AC CR.

## Controller's Reconciliation Logic


When the openstack-operator generates a new AC CR for Barbican`oc get appcred ac-barbican -n openstack`:

```
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
````

the AC controller:
1. Reconcile() invoked

	  -   Controller-runtime calls `Reconcile()` with `ac-barbican`

2. Finalizer & Conditions

    -   Adds `openstack.org/applicationcredential` finalizer

    -   Initializes status conditions (`KeystoneAPIReady`, `AdminServiceClientReady`, `Ready`)

3.  Wait for KeystoneAPI
    -   If missing or not ready, marks condition and requeues
4. Determine Rotation Need

    -   `needsRotation()` returns `true` because no AC ID exists yet

5. Lookup User ID (Admin)

    -   Uses admin-scoped client to fetch the Keystone user ID for `barbican`

6. Service‐Scoped Client

    -   Retrieves `BarbicanPassword` from `osp-secret` (or any other specified in the AC CR)

    -   Authenticates as `barbican` user

7. Create AC in Keystone
	- Calls gocloud’s `applicationcredentials.Create(...)`
8. Store Secret
	- Creates a k8s `Secret` named `ac-barbican-secret`
```
apiVersion: v1
kind: Secret
metadata:
  name: ac-barbican-secret
  namespace: openstack
  ## and other references....
data:
  AC_ID:     <base64-of-AC-ID>
  AC_SECRET: <base64-of-AC-secret>
```
9. Update CR status
	- Sets `.status.acID`, `.status.secretName`, `.status.createdAt`, `.status.expiresAt`
	- Marks AC CR ready
10. Requeue for Next Check
	-   Calculates next reconcile at `expiresAt - gracePeriod`
    -   If already in grace window, requeues immediately, otherwise requeues after 24 h

AC in Keystone side:
```
openstack application credential show 7b23dbac20bc4f048f937415c84bb329
+--------------+------------------------------------------------+
| Field        | Value                                          |
+--------------+------------------------------------------------+
| description  | Created by keystone-operator for user barbican |
| expires_at   | 2026-05-29T09:02:28.705012                     |
| id           | 7b23dbac20bc4f048f937415c84bb329               |
| name         | ac-barbican-29090                              |
| project_id   | 24f05745bc2145c6a625b528ce21d7a3               |
| roles        | service                                        |
| system       | None                                           |
| unrestricted | False                                          |
| user_id      | 2ecd25b38f0d432388ad8b838e46f36d               |
+--------------+------------------------------------------------+
```
Note: The actual AC `name` in Keystone includes a 5-character random suffix (e.g. `-29090`) to avoid collisions during rotation.


11. Rotation
When the next reconcile hits the grace window (`now ≥ expiresAt - gracePeriodDays`), `needsRotation()` returns `true` again and the controller:

    - Lookup User ID as before
    - Service-Scoped Client
    - Create New AC
    - Generates a new Keystone AC with a fresh 5-char suffix
    - Uses the same roles, unrestricted flag, and expirationDays
    - Does _not_ revoke the old AC, the old credential naturally expires
    - Store Updated Secret
      Overwrites the existing `ac-barbican-secret` with the new `AC_ID` and `AC_SECRET`

    - Update Status
      Replaces `.status.acID` and `.status.expiresAt` with the new values and re-marks Ready

    - Requeue
      Schedules the next check at `(newExpiresAt - gracePeriodDays)`


## Manual Rotation
Manual rotation can be triggered patching the AC CR with expiration with timestamp in the past, e.g.:

` oc patch -n openstack keystoneapplicationcredential ac-barbican   --type=merge   --subresource=status   -p '{"status":{"expiresAt":"2001-05-19T00:00:00Z"}}'`

Note: Rotation is triggered by deleting AC CR itself, however that triggers services to fallback immediately to password usage. With patching just the expiration timestamp, no fallback is triggered.
