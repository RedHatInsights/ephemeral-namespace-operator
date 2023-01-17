package controllers

import (
	_ "embed"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	configv1 "github.com/openshift/api/config/v1"
	projectv1 "github.com/openshift/api/project/v1"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clowder.AddToScheme(scheme))
	utilruntime.Must(frontend.AddToScheme(scheme))
	utilruntime.Must(projectv1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(crd.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

//go:embed version.txt
var Version string

func Run(metricsAddr string, probeAddr string, enableLeaderElection bool) {
	enoVersion.With(prometheus.Labels{"version": Version}).Inc()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "2ee9ac64.cloud.redhat.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	poller := Poller{
		client:             mgr.GetClient(),
		activeReservations: make(map[string]metav1.Time),
		log:                ctrl.Log.WithName("Poller"),
	}

	if err = (&NamespaceReservationReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		poller: &poller,
		log:    ctrl.Log.WithName("controllers").WithName("ReservationController"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NamespaceReservation")
		os.Exit(1)
	}
	if err = (&NamespacePoolReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		log:    ctrl.Log.WithName("controllers").WithName("NamespacePoolController"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "NamespacePool")
		os.Exit(1)
	}
	if err = (&ClowdenvironmentReconciler{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		log:    ctrl.Log.WithName("controllers").WithName("ClowdEnvController"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Clowdenvironment")
		os.Exit(1)
	}
	if err = (&NamespaceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("Namespace"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Namespace")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	go poller.Poll()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
