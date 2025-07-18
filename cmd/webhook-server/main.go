package main

import (
	"flag"

	kmmhubv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api-hub/v1beta1"
	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	kmmv1beta2 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta2"
	"github.com/kubernetes-sigs/kernel-module-management/internal/cmd"
	"github.com/kubernetes-sigs/kernel-module-management/internal/config"
	"github.com/kubernetes-sigs/kernel-module-management/internal/constants"
	"github.com/kubernetes-sigs/kernel-module-management/internal/webhook"
	"github.com/kubernetes-sigs/kernel-module-management/internal/webhook/hub"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2/textlogger"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(kmmv1beta1.AddToScheme(scheme))
	utilruntime.Must(kmmv1beta2.AddToScheme(scheme))
	utilruntime.Must(kmmhubv1beta1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	logConfig := textlogger.NewConfig()
	logConfig.AddFlags(flag.CommandLine)

	var (
		enableModule               bool
		enableManagedClusterModule bool
		enableNamespaceDeletion    bool
		enablePreflightValidation  bool
		userConfigMapName          string
	)

	flag.StringVar(&userConfigMapName, "config", "", "Name of the ConfigMap containing user config.")
	flag.BoolVar(&enableModule, "enable-module", false, "Enable the webhook for Module resources")
	flag.BoolVar(&enableManagedClusterModule, "enable-managedclustermodule", false, "Enable the webhook for ManagedClusterModule resources")
	flag.BoolVar(&enableNamespaceDeletion, "enable-namespace", false, "Enable the webhook for Namespace deletion")
	flag.BoolVar(&enablePreflightValidation, "enable-preflightvalidation", false, "Enable the webhook for PreflightValidation resources")

	flag.Parse()

	logger := textlogger.NewLogger(logConfig).WithName("kmm-webhook")

	ctrl.SetLogger(logger)

	setupLogger := logger.WithName("setup")

	commit, err := cmd.GitCommit()
	if err != nil {
		setupLogger.Error(err, "Could not get the git commit; using <undefined>")
		commit = "<undefined>"
	}

	setupLogger.Info("Creating manager", "git commit", commit)
	operatorNamespace := cmd.GetEnvOrFatalError(constants.OperatorNamespaceEnvVar, setupLogger)

	ctx := ctrl.SetupSignalHandler()
	cg := config.NewConfigGetter(setupLogger)

	cfg, err := cg.GetConfig(ctx, userConfigMapName, operatorNamespace, false)
	if err != nil {
		cmd.FatalError(setupLogger, err, "failed to get kmm config")
	}

	options := cg.GetManagerOptionsFromConfig(cfg, scheme)
	options.LeaderElection = false

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		cmd.FatalError(setupLogger, err, "unable to create manager")
	}

	if enableModule {
		logger.Info("Enabling Module webhook")

		if err = webhook.NewModuleValidator(logger).SetupWebhookWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create webhook", "webhook", "ModuleValidator")
		}
	}

	if enableManagedClusterModule {
		logger.Info("Enabling ManagedClusterModule webhook")

		if err = hub.NewManagedClusterModuleValidator(logger).SetupWebhookWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create webhook", "webhook", "ManagedClusterModuleValidator")
		}
	}

	if enableNamespaceDeletion {
		logger.Info("Enabling Namespace deletion webhook")

		if err = (&webhook.NamespaceValidator{}).SetupWebhookWithManager(mgr); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create webhook", "webhook", "NamespaceValidator")
		}
	}

	if enablePreflightValidation {
		if err = ctrl.NewWebhookManagedBy(mgr).For(&kmmv1beta1.PreflightValidation{}).Complete(); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create conversion webhook", "name", "PreflightValidation/v1beta1")
		}

		logger.Info("Enabling PreflightValidation webhook")
		if err = webhook.NewPreflightValidationValidator(logger).SetupWebhookWithManager(mgr, &kmmv1beta2.PreflightValidation{}); err != nil {
			cmd.FatalError(setupLogger, err, "unable to create webhook", "webhook", "PreflightValidationValidator")
		}
	}

	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		cmd.FatalError(setupLogger, err, "unable to set up health check")
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		cmd.FatalError(setupLogger, err, "unable to set up ready check")
	}

	setupLogger.Info("starting manager")
	if err = mgr.Start(ctx); err != nil {
		cmd.FatalError(setupLogger, err, "problem running manager")
	}
}
