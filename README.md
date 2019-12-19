[![Build Status](https://travis-ci.org/OpenLiberty/open-liberty-operator.svg?branch=master)](https://travis-ci.org/OpenLiberty/open-liberty-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/OpenLiberty/open-liberty-operator)](https://goreportcard.com/report/github.com/OpenLiberty/open-liberty-operator)

# Open Liberty Operator

The Open Liberty Operator can be used to deploy and manage applications running on Open Liberty into [OKD](https://www.okd.io/) or [OpenShift](https://www.openshift.com/) clusters. You can also perform Day-2 operations such as gathering traces and dumps using the operator.

If there's a certain functionality you would like to see or a bug you would like to report, please use our [issues tab](https://github.com/OpenLiberty/open-liberty-operator/issues) to get in contact with us.

## Operator Installation

You can install the Open Liberty Operator directly via `kubectl` commands or assisted by the [Operator Lifecycle Manager](https://github.com/operator-framework/operator-lifecycle-manager).

Use the instructions for one of the [releases](deploy/releases) to directly install this Operator into a Kubernetes cluster.

## Overview

The architecture of the Open Liberty Operator follows the basic controller pattern: the Operator container with the controller is deployed into a Pod and listens for incoming resources with `Kind: OpenLibertyApplication`.

In addition, Open Liberty Operator makes it very easy to perform Day-2 operations on a Open Liberty server instance running on a container inside a Pod: 
- Gather traces using resource `Kind: OpenLibertyTrace`
- Generate server dumps using resource `Kind: OpenLibertyDump`

## Documentation

For information on how to use the `OpenLibertyApplication` operator, see the [documentation](doc/).
