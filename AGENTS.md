# AGENTS.md - keystone-operator

## Project overview

keystone-operator is a Kubernetes operator that manages
[OpenStack Keystone](https://docs.openstack.org/keystone/latest/) (the identity
service: authentication, authorization, service catalog, application
credentials, and federation) on OpenShift/Kubernetes. It is part of the
[openstack-k8s-operators](https://github.com/openstack-k8s-operators) project.

Key Keystone domain concepts: **domains**, **projects**, **users**, **roles**,
**service catalog**, **endpoints**, **application credentials**, **access
control rules**, **federation**, **fernet tokens**.

Go module: `github.com/openstack-k8s-operators/keystone-operator`
API group: `keystone.openstack.org`
API version: `v1beta1`

## Tech stack

| Layer | Technology |
|-------|------------|
| Language | Go (modules, multi-module workspace via `go.work`) |
| Scaffolding | [Kubebuilder v4](https://book.kubebuilder.io/) + [Operator SDK](https://sdk.operatorframework.io/) |
| CRD generation | controller-gen (DeepCopy, CRDs, RBAC, webhooks) |
| Config management | Kustomize |
| Packaging | OLM bundle |
| Testing | Ginkgo/Gomega + envtest (functional), KUTTL (integration) |
| Linting | golangci-lint (`.golangci.yaml`) |
| CI | Zuul (`zuul.d/`), Prow (`.ci-operator.yaml`), GitHub Actions |

## Custom Resources

| Kind | Purpose |
|------|---------|
| `KeystoneAPI` | Manages the Keystone API deployment (httpd/WSGI). The main user-facing CR. |
| `KeystoneService` | Registers an OpenStack service in the Keystone catalog. Used by other operators. |
| `KeystoneEndpoint` | Registers an endpoint for a Keystone service. Used by other operators. |
| `KeystoneApplicationCredential` | Manages application credential resources. |

The `KeystoneAPI` CR has defaulting and validating admission webhooks.
`KeystoneService` and `KeystoneEndpoint` CRs are typically created by other
OpenStack service operators (not directly by users) to register themselves
in the Keystone service catalog.

## Directory structure

| Directory | Contents |
|-----------|----------|
| `api/v1beta1/` | CRD types (`keystoneapi_types.go`, `keystoneservice_types.go`, `keystoneendpoint_types.go`, `keystoneapplicationcredential_types.go`), conditions, webhook markers |
| `api/test/` | Test helpers (`TestHelper`, `KeystoneAPIFixture`) for use by other operators' tests |
| `cmd/` | `main.go` entry point |
| `internal/controller/` | Reconcilers: `keystoneapi_controller.go`, `keystoneservice_controller.go`, `keystoneendpoint_controller.go`, `keystoneapplicationcredential_controller.go` |
| `internal/keystone/` | Keystone resource builders (db-sync, bootstrap, deployment, fernet) |
| `internal/webhook/` | Webhook implementation |
| `templates/` | Config files and scripts mounted into pods via `OPERATOR_TEMPLATES` env var |
| `config/crd,rbac,manager,webhook/` | Generated Kubernetes manifests (CRDs, RBAC, deployment, webhooks) |
| `config/samples/` | Example CRs (Kustomize overlays). Includes TLS variant. |
| `test/functional/` | envtest-based Ginkgo/Gomega tests |
| `test/kuttl/` | KUTTL integration tests |
| `hack/` | Helper scripts (CRD schema checker, local webhook runner) |
| `docs/` | Documentation |

## Build commands

After modifying Go code, always run: `make generate manifests fmt vet`.

## Code style guidelines

- Follow standard openstack-k8s-operators conventions and lib-common patterns.
- Use `lib-common` modules for conditions, endpoints, TLS, storage, and other
  cross-cutting concerns rather than re-implementing them.
- CRD types go in `api/v1beta1/`. Controller logic goes in
  `internal/controller/`. Resource-building helpers go in `internal/keystone/`.
- Config templates are plain files in `templates/` -- they are mounted at
  runtime via the `OPERATOR_TEMPLATES` environment variable.
- Webhook logic is split between the kubebuilder markers in `api/v1beta1/` and
  the implementation in `internal/webhook/`.
- Test helpers in `api/test/` are consumed by other operators for their
  functional tests.

## Testing

- Functional tests use the envtest framework with Ginkgo/Gomega and live in
  `test/functional/`.
- KUTTL integration tests live in `test/kuttl/`.
- Run all functional tests: `make test`.
- When adding a new field or feature, add corresponding test cases in
  `test/functional/` and update fixture data accordingly.

## Key dependencies

- [lib-common](https://github.com/openstack-k8s-operators/lib-common): shared modules for conditions, endpoints, database, TLS, secrets, etc.
- [infra-operator](https://github.com/openstack-k8s-operators/infra-operator): RabbitMQ and topology APIs.
- [mariadb-operator](https://github.com/openstack-k8s-operators/mariadb-operator): database provisioning.
- [gophercloud](https://github.com/gophercloud/gophercloud): Go OpenStack SDK (used for Keystone API calls).
