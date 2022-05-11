/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	clowder "github.com/RedHatInsights/clowder/apis/cloud.redhat.com/v1alpha1"
	crd "github.com/RedHatInsights/ephemeral-namespace-operator/apis/cloud.redhat.com/v1alpha1"
	frontend "github.com/RedHatInsights/frontend-operator/api/v1alpha1"
	core "k8s.io/api/core/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var k8sClient client.Client
var testEnv *envtest.Environment
var stopController context.CancelFunc

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

// routine that will auto-update ClowdEnvironment status during suite test run
func populateClowdEnvStatus(client client.Client) {
	ctx := context.Background()

	for {
		time.Sleep(time.Duration(1 * time.Second))
		clowdEnvs := clowder.ClowdEnvironmentList{}
		err := client.List(ctx, &clowdEnvs)
		if err != nil {
			continue
		}
		for _, env := range clowdEnvs.Items {
			if len(env.Status.Conditions) == 0 {
				status := clowder.ClowdEnvironmentStatus{
					Conditions: []clusterv1.Condition{
						{
							Type:               clowder.ReconciliationSuccessful,
							Status:             core.ConditionTrue,
							LastTransitionTime: metav1.Now(),
						},
						{
							Type:               clowder.DeploymentsReady,
							Status:             core.ConditionTrue,
							LastTransitionTime: metav1.Now(),
						},
					},
				}
				env.Status = status
				err := client.Status().Update(ctx, &env)
				if err != nil {
					fmt.Println("ERROR: ", err)
				}
			}
		}
	}
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "crd", "static"), // added to the project manually
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sscheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(k8sscheme)
	clowder.AddToScheme(k8sscheme)
	frontend.AddToScheme(k8sscheme)

	err = crd.AddToScheme(k8sscheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: k8sscheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: k8sscheme,
	})
	Expect(err).ToNot(HaveOccurred())

	testConfig := crd.NamespacePoolSpec{
		Size:  2,
		Local: true,
		ClowdEnvironment: clowder.ClowdEnvironmentSpec{
			Providers: clowder.ProvidersConfig{
				Kafka: clowder.KafkaConfig{
					Mode: "operator",
					Cluster: clowder.KafkaClusterConfig{
						Name:      "kafka",
						Namespace: "kafka",
						Replicas:  5,
					},
				},
				Database: clowder.DatabaseConfig{
					Mode: "local",
				},
				Logging: clowder.LoggingConfig{
					Mode: "none",
				},
				ObjectStore: clowder.ObjectStoreConfig{
					Mode: "minio",
				},
				InMemoryDB: clowder.InMemoryDBConfig{
					Mode: "redis",
				},
				Web: clowder.WebConfig{
					Port: int32(8000),
					Mode: "none",
				},
				Metrics: clowder.MetricsConfig{
					Port: int32(9000),
					Path: "/metrics",
					Mode: "none",
				},
				FeatureFlags: clowder.FeatureFlagsConfig{
					Mode: "local",
				},
				Testing: clowder.TestingConfig{
					ConfigAccess:   "environment",
					K8SAccessLevel: "edit",
					Iqe: clowder.IqeConfig{
						ImageBase: "quay.io/cloudservices/iqe-tests",
					},
				},
				AutoScaler: clowder.AutoScalerConfig{
					Mode: "keda",
				},
			},
		},
		LimitRange: core.LimitRange{
			Spec: core.LimitRangeSpec{
				Limits: []core.LimitRangeItem{},
			},
		},
		ResourceQuotas: core.ResourceQuotaList{
			Items: []core.ResourceQuota{},
		},
	}

	poller := Poller{
		Client:             k8sManager.GetClient(),
		ActiveReservations: make(map[string]metav1.Time),
		Log:                ctrl.Log.WithName("Poller"),
	}

	err = (&NamespacePoolReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("NamespacePoolReconciler"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&NamespaceReservationReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Poller: &poller,
		Log:    ctrl.Log.WithName("controllers").WithName("NamespaceReconciler"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&ClowdenvironmentReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
		Log:    ctrl.Log.WithName("controllers").WithName("ClowdEnvController"),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	ctx, cancel := context.WithCancel(context.Background())
	stopController = cancel

	default_pool := &crd.NamespacePool{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cloud.redhat.com/",
			Kind:       "NamespacePool",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "default-pool",
		},
		Spec: testConfig,
	}

	Expect(k8sClient.Create(ctx, default_pool)).Should(Succeed())

	go poller.Poll()

	go populateClowdEnvStatus(k8sManager.GetClient())

	go func() {
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred())
	}()

}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	stopController()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
