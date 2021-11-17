[![Build Status](https://travis-ci.org/OpenLiberty/open-liberty-operator.svg?branch=master)](https://travis-ci.org/OpenLiberty/open-liberty-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/OpenLiberty/open-liberty-operator)](https://goreportcard.com/report/github.com/OpenLiberty/open-liberty-operator)

# Open Liberty Operator

The Open Liberty Operator can be used to deploy and manage applications running on Open Liberty or WebSphere Liberty into Kubernetes-based platforms, such as [Red Hat OpenShift](https://www.openshift.com/). You can also perform Day-2 operations such as gathering traces and dumps using the operator.

If there's a certain functionality you would like to see or a bug you would like to report, please use our [issues tab](https://github.com/OpenLiberty/open-liberty-operator/issues) to get in contact with us.

## Operator Installation

You can install the Open Liberty Operator directly via `kubectl` commands or assisted by the [Operator Lifecycle Manager](https://github.com/operator-framework/operator-lifecycle-manager).

Use the instructions for one of the [releases](deploy/releases) to directly install this Operator into a Kubernetes cluster.

## Overview

The architecture of the Open Liberty Operator follows the basic controller pattern: the Operator container with the controller is deployed into a Pod and listens for incoming resources with `Kind: OpenLibertyApplication`.

In addition, Open Liberty Operator makes it easy to perform [Day-2 operations](doc/user-guide.md#day-2-operations) on an instance of Open Liberty server running inside a Pod: 
- Gather server traces using resource `Kind: OpenLibertyTrace`
- Generate server dumps using resource `Kind: OpenLibertyDump`

## Documentation

For information on how to use the Open Liberty Operator, see the [documentation](doc/).

## License

Usage is provided under the [EPL 1.0 license](https://opensource.org/licenses/EPL-1.0). See [LICENSE](LICENSE) for the full details.

## Contributing 

We welcome all contributions to the Open Liberty Operator project. Please see our [Contributing guidelines](CONTRIBUTING.md).
