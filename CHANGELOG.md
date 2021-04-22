<!--
This file includes chronologically ordered list of notable changes visible to end users for each version of the Open Liberty Operator. Keep a summary of the change and link to the pull request.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
-->

# Changelog

All notable changes to this project will be documented in this file.

## [0.7.1]

### Fixed

- Updated Image Stream lookup logic to query for Image Stream Tags. ([#229](https://github.com/OpenLiberty/open-liberty-operator/pull/229)), [#156](https://github.com/application-stacks/runtime-component-operator/pull/156))
- Concurrency issue. ([#226](https://github.com/OpenLiberty/open-liberty-operator/pull/226))

## [0.7.0]

### Added

- Added support for automatic registration with OIDC providers. Tested with Red Hat Single Sign-on (RH-SSO) and IBM Security Verify. ([#152](https://github.com/OpenLiberty/open-liberty-operator/pull/152))
- Added support to use application as a backing service ([#192](https://github.com/OpenLiberty/open-liberty-operator/pull/192), [#148](https://github.com/application-stacks/runtime-component-operator/pull/148))
- Added support to specify storage class name for the `PersistentVolumeClaim` created for serviceability. ([#188](https://github.com/OpenLiberty/open-liberty-operator/pull/188))


## [0.6.0]

### Added

- Added support for embedding service bindings into a custom resource ([#164](https://github.com/OpenLiberty/open-liberty-operator/pull/164), [#111](https://github.com/application-stacks/runtime-component-operator/pull/111))
- Added support for affinity settings such as _nodeAffinity, podAffinity_ and _podAntiAffinity_ ([#164](https://github.com/OpenLiberty/open-liberty-operator/pull/164), [#116](https://github.com/application-stacks/runtime-component-operator/pull/116))

### Changed

- **Breaking change:** In order for the operator to detect Service Binding custom resources automatically ("auto-detect" functionality), the name of the binding resource must follow the `<CR_NAME>-binding` format (e.g. `my-app-binding`) ([#164](https://github.com/OpenLiberty/open-liberty-operator/pull/164), [#111](https://github.com/application-stacks/runtime-component-operator/pull/111))

### Fixed

- Some monitoring configurations such as `bearerTokenSecret` are not propagated to the created `ServiceMonitor` ([#164](https://github.com/OpenLiberty/open-liberty-operator/pull/164), [#157](https://github.com/OpenLiberty/open-liberty-operator/issues/157), [#116](https://github.com/application-stacks/runtime-component-operator/pull/116))

## [0.5.1]

### Fixed

- Operator crash when Ingress is created without defining spec.route field ([#147](https://github.com/OpenLiberty/open-liberty-operator/pull/147))
- Unnecessary pod restarts due to adding kubectl.kubernetes.io/last-applied-configuration to resources created by the operator ([#147](https://github.com/OpenLiberty/open-liberty-operator/pull/147))

## [0.5.0]

### Added

- Added Ingress (vanilla) support ([#79](https://github.com/application-stacks/runtime-component-operator/pull/79), [#141](https://github.com/OpenLiberty/open-liberty-operator/pull/141))
- Added support for external service bindings ([#76](https://github.com/application-stacks/runtime-component-operator/pull/76), [#141](https://github.com/OpenLiberty/open-liberty-operator/pull/141))
- Added additional service ports support ([#80](https://github.com/application-stacks/runtime-component-operator/pull/80), [#141](https://github.com/OpenLiberty/open-liberty-operator/pull/141))
- Added support to specify NodePort on service ([#60](https://github.com/application-stacks/runtime-component-operator/pull/60), [#141](https://github.com/OpenLiberty/open-liberty-operator/pull/141))

### Fixed

- Auto-scaling (HPA) not working as expected ([#72](https://github.com/application-stacks/runtime-component-operator/pull/72))
- Operator crashes on some cluster due to optional CRDs (Knative Service, ServiceMonitor) not being present ([#141](https://github.com/OpenLiberty/open-liberty-operator/pull/141))


## [0.4.0]

### Added

- Added support for single sign-on using social login providers and any OIDC & OAuth 2.0 based clients. ([#123](https://github.com/OpenLiberty/open-liberty-operator/pull/122))
- Added support for integration with cert-manager (Certificate CRD). ([#122](https://github.com/OpenLiberty/open-liberty-operator/pull/122))
- Added support for referencing images in image streams. ([#122](https://github.com/OpenLiberty/open-liberty-operator/pull/122))
- Added support to specify application name to group related resources. ([#122](https://github.com/OpenLiberty/open-liberty-operator/pull/122))
- Added optional targetPort to service in the CRD. ([#122](https://github.com/OpenLiberty/open-liberty-operator/pull/122))
- Added support for sidecar containers. ([#122](https://github.com/OpenLiberty/open-liberty-operator/pull/122))
- Added support for naming service port.  ([#122](https://github.com/OpenLiberty/open-liberty-operator/pull/122))
- Added OpenShift specific annotations ([#54](https://github.com/application-stacks/runtime-component-operator/pull/54))
- Set port name for Knative service if specified ([#55](https://github.com/application-stacks/runtime-component-operator/pull/55))

### Changed

- Changed the match label of the ServiceMonitor created by operator from `app.openliberty.io/monitor` to `monitor.openliberty.io/enabled` ([#122](https://github.com/OpenLiberty/open-liberty-operator/pull/122))
- Updated default environment variable for liberty logging to remove tracing from container logs ([#95](https://github.com/OpenLiberty/open-liberty-operator/issues/95))
- **Breaking change:** When `service.consumes[].namespace` is not specified, injected name of environment variable follows `<SERVICE-NAME>_<KEY>` format and binding information are mounted at `<mountPath>/<service_name>`. ([#27](https://github.com/application-stacks/runtime-component-operator/pull/27) and [#46](https://github.com/application-stacks/runtime-component-operator/pull/46))

## [0.3.0]

### Changed

- The initial release of the go-based Open Liberty Operator. 
- **Breaking change:** You can not upgrade from helm-based operator (v0.0.1) to go-based operator as the APIs have changed. 

## [0.0.1]

The initial release of the helm-based Open Liberty Operator.

[Unreleased]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.7.1...HEAD
[0.7.1]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.6.0...v0.7.1
[0.7.0]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.0.1...v0.3.0
[0.0.1]: https://github.com/OpenLiberty/open-liberty-operator/releases/tag/v0.0.1
