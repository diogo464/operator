/*
Copyright 2023.

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

package main

import (
	"flag"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	infrav1 "git.d464.sh/infra/operator/api/infra/v1"
	corecontroller "git.d464.sh/infra/operator/internal/controller/core"
	infracontroller "git.d464.sh/infra/operator/internal/controller/infra"
	networkingcontroller "git.d464.sh/infra/operator/internal/controller/networking"
	infraweb "git.d464.sh/infra/operator/internal/web"
	//+kubebuilder:scaffold:imports
)

const (
	MODULE_INGRESS     = "ingress"
	MODULE_MINIO       = "minio"
	MODULE_POSTGRES    = "postgres"
	MODULE_PORTFORWARD = "portforward"
	MODULE_DOMAINNAME  = "domainname"
	MODULE_SERVICE     = "service"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

type MinioConfig struct {
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(infrav1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	hosts := infraweb.NewHosts()
	forwards := infraweb.NewForwards()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "0d7e8c20.d464.sh",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if isModuleEnabled(MODULE_INGRESS) {
		setupLog.Info("Ingress module enabled")
		if err = (&networkingcontroller.IngressReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Ingress")
			os.Exit(1)
		}
	}

	if isModuleEnabled(MODULE_MINIO) {
		setupLog.Info("Minio module enabled")

		config, err := readMinioConfig()
		if err != nil {
			setupLog.Error(err, "unable to read minio config")
			os.Exit(1)
		}

		minioReconciler, err := infracontroller.NewMinioServiceAccountReconciler(mgr.GetClient(), mgr.GetScheme(), &infracontroller.MinioServiceAccountReconcilerConfig{
			MinioEndpoint:  config.MinioEndpoint,
			MinioAccessKey: config.MinioAccessKey,
			MinioSecretKey: config.MinioSecretKey,
		})
		if err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MinioServiceAccount")
			os.Exit(1)
		}
		if err := minioReconciler.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to setup controller", "controller", "MinioServiceAccount")
			os.Exit(1)
		}
	}

	if isModuleEnabled(MODULE_POSTGRES) {
		setupLog.Info("Postgres module enabled")
		if err = (&infracontroller.PostgresReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Postgres")
			os.Exit(1)
		}
	}

	if isModuleEnabled(MODULE_PORTFORWARD) {
		if err = (&infracontroller.PortForwardReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Forwards: forwards,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "PortForward")
			os.Exit(1)
		}
	}

	if isModuleEnabled(MODULE_DOMAINNAME) {
		if err = (&infracontroller.DomainNameReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
			Hosts:  hosts,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DomainName")
			os.Exit(1)
		}
	}

	if isModuleEnabled(MODULE_SERVICE) {
		if err = (&corecontroller.ServiceReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Service")
			os.Exit(1)
		}
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

	go func() {
		if err := infraweb.Start(forwards, hosts); err != nil {
			setupLog.Error(err, "unable to start web server")
			os.Exit(1)
		}
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func readMinioConfig() (*MinioConfig, error) {
	config := &MinioConfig{}

	config.MinioEndpoint = os.Getenv("MINIO_ENDPOINT")
	config.MinioAccessKey = os.Getenv("MINIO_ACCESS_KEY")
	config.MinioSecretKey = os.Getenv("MINIO_SECRET_KEY")

	return config, nil
}

func isModuleEnabled(module string) bool {
	modulesVar, exists := os.LookupEnv("MODULES")
	if !exists {
		return true
	}
	modules := strings.Split(modulesVar, ",")
	for _, m := range modules {
		if m == module {
			return true
		}
	}
	return false
}

func isLeaderElectioEnabled() bool {
	return os.Getenv("LEADER_ELECTION") == "true"
}
