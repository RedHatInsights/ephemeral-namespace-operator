module github.com/RedHatInsights/ephemeral-namespace-operator

go 1.16

require (
	github.com/RedHatInsights/clowder v0.30.0
	github.com/RedHatInsights/frontend-operator v0.0.2
	github.com/RedHatInsights/rhc-osdk-utils v0.4.1
	github.com/go-logr/logr v0.4.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.com/openshift/api v0.0.0-20200217161739-c99157bc6492
	k8s.io/api v0.22.4
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
	sigs.k8s.io/cluster-api v1.0.1
	sigs.k8s.io/controller-runtime v0.10.3
)
