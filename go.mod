module github.com/RedHatInsights/ephemeral-namespace-operator

go 1.16

require (
	github.com/RedHatInsights/clowder v0.27.2
	github.com/RedHatInsights/rhc-osdk-utils v0.3.2
	github.com/go-logr/logr v0.4.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.com/openshift/api v3.9.0+incompatible
	k8s.io/api v0.22.4
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.22.4
	sigs.k8s.io/controller-runtime v0.10.3
)
