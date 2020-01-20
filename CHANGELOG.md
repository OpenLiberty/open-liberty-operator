<!--
This file includes chronologically ordered list of notable changes visible to end users for each version of the Open Liberty Operator. Keep a summary of the change and link to the pull request.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
-->

# Changelog
All notable changes to this project will be documented in this file.

## [Unreleased]

### Changed

- Changed default labels for Liberty Logging to disable tracing to container
  logs and turn messages.log off ([#94](https://github.com/OpenLiberty/open-liberty-operator/issues/94))

## [0.3.0]

### Changed

- The initial release of the go-based Open Liberty Operator. 
- **Breaking change:** You can not upgrade from helm-based operator (v0.0.1) to go-based operator as the APIs have changed. 

## [0.0.1]

The initial release of the helm-based Open Liberty Operator. 

[Unreleased]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/OpenLiberty/open-liberty-operator/compare/v0.0.1...v0.3.0
[0.0.1]: https://github.com/OpenLiberty/open-liberty-operator/releases/tag/v0.0.1
