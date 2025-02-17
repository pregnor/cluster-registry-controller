// Copyright (c) 2021, and 2022 Cisco and/or its affiliates. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"os"
	"time"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/banzaicloud/operator-tools/pkg/logger"
	// +kubebuilder:scaffold:imports
	clusterregistryv1alpha1 "github.com/cisco-open/cluster-registry-controller/api/v1alpha1"
	"github.com/cisco-open/cluster-registry-controller/controllers"
	"github.com/cisco-open/cluster-registry-controller/internal/config"
	"github.com/cisco-open/cluster-registry-controller/pkg/clusters"
	"github.com/cisco-open/cluster-registry-controller/pkg/signals"
	"github.com/cisco-open/cluster-registry-controller/pkg/util"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const FriendlyServiceName = "cluster-registry"

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = clusterregistryv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	configuration := configure()

	if configuration.Logging.Format == config.LogFormatConsole {
		logger.GlobalLogLevel = int(configuration.Logging.Verbosity)
		ctrl.SetLogger(logger.New(logger.WithTime(time.RFC3339))) // , logger.Out(ioutil.Discard)))
	} else {
		ctrl.SetLogger(zap.New(
			zap.UseDevMode(false),
			zap.Level(zapcore.Level(0-configuration.Logging.Verbosity)),
		))
	}

	if configuration.ProvisionLocalCluster != "" {
		client, err := client.New(ctrl.GetConfigOrDie(), client.Options{
			Scheme: scheme,
		})
		if err != nil {
			setupLog.Error(err, "cannot connect to kubernetes cluster")
			os.Exit(1)
		}

		err = util.ProvisionLocalClusterObject(client,
			ctrl.Log.WithName("provision-local-cluster"),
			config.Configuration(configuration))
		if err != nil {
			setupLog.Error(err, "cannot provision local cluster object")
			os.Exit(1)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      configuration.MetricsAddr,
		LeaderElection:          configuration.LeaderElection.Enabled,
		LeaderElectionID:        configuration.LeaderElection.Name,
		LeaderElectionNamespace: configuration.LeaderElection.Namespace,
		Port:                    0,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := signals.NotifyContext(context.Background())

	clustersManager := clusters.NewManager(ctx)

	// // sync rule for cluster resources
	// AddClustersSyncRule(clustersManager, mgr, ctrl.Log, config.Configuration(configuration))

	// // sync rule for cluster secrets
	// AddClusterSecretsSyncRule(clustersManager, mgr, ctrl.Log, config.Configuration(configuration))

	// // sync rule for sync rules
	// AddResourceSyncRuleSyncRule(clustersManager, mgr, ctrl.Log, config.Configuration(configuration))

	if err = controllers.NewResourceSyncRuleReconciler("resource-sync-rules", ctrl.Log.WithName("controllers").WithName("resource-sync-rule"), clustersManager, config.Configuration(configuration)).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "resource-sync-rule")
		os.Exit(1)
	}

	if err = controllers.NewClusterReconciler("clusters", ctrl.Log.WithName("controllers").WithName("cluster"), clustersManager, config.Configuration(configuration)).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "cluster")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
