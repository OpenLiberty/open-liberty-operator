This directory contains the output of `operator-sdk new` for the Open
Liberty operator, before any modifications.  It is not intended for
use other than to compare against output of future versions of
`operator-sdk`, or future versions of the Open Liberty helm chart.

The contents were generated using the following version of
`operator-sdk`: `operator-sdk version: v0.7.0+git`

The version of the Open Liberty helm chart used was 1.9.1.

Following is one way to generate the baseline directory 
* cd open-liberty-operator/baseline

* `operator-sdk new open-liberty-operator --kind=OpenLiberty --type=helm --helm-chart=ibm-open-liberty --helm-chart-repo=https://raw.githubusercontent.com/IBM/charts/master/repo/stable/`

This readme should be updated when the version of `operator-sdk` or
the version of the Open Liberty helm chart used to build the Open
Liberty operator changes.
