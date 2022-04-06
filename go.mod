module github.com/OpenLiberty/open-liberty-operator

go 1.16

require (
	github.com/application-stacks/runtime-component-operator v0.8.1-0.20220406013638-131565c1b2b8
	github.com/coreos/prometheus-operator v0.41.1
	github.com/go-logr/logr v0.3.0
	github.com/openshift/api v0.0.0-20201019163320-c6a5ec25f267
	github.com/openshift/library-go v0.0.0-20201026125231-a28d3d1bad23
	github.com/pkg/errors v0.9.1
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v12.0.0+incompatible
	knative.dev/serving v0.18.1
	sigs.k8s.io/controller-runtime v0.7.2
)

// Pinned to kubernetes-1.16.2
replace k8s.io/client-go => k8s.io/client-go v0.19.2

//To resolve license issue - https://github.com/operator-framework/operator-registry/issues/190
replace (
	github.com/otiai10/copy => github.com/otiai10/copy v1.0.2
	github.com/otiai10/mint => github.com/otiai10/mint v1.3.0
)

replace github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309 // Required by Helm
