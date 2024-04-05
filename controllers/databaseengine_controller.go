// everest-operator
// Copyright (C) 2022 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	opfwv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
	"github.com/percona/everest-operator/controllers/version"
)

var (
	errInstallPlanNotFound = errors.New("install plan not found")
)

var operatorEngine = map[string]everestv1alpha1.EngineType{
	common.PXCDeploymentName:   everestv1alpha1.DatabaseEnginePXC,
	common.PSMDBDeploymentName: everestv1alpha1.DatabaseEnginePSMDB,
	common.PGDeploymentName:    everestv1alpha1.DatabaseEnginePostgresql,
}

// DatabaseEngineReconciler reconciles a DatabaseEngine object.
type DatabaseEngineReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	versionService *version.Service
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseengines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseengines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseengines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *DatabaseEngineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	engineType, ok := operatorEngine[req.NamespacedName.Name]
	if !ok {
		// Unknown operator, nothing to do here
		return ctrl.Result{}, nil
	}

	dbEngine := &everestv1alpha1.DatabaseEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.NamespacedName.Name,
			Namespace: req.NamespacedName.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, dbEngine, func() error {
		dbEngine.Spec.Type = engineType
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}

	dbEngine.Status.State = everestv1alpha1.DBEngineStateNotInstalled
	dbEngine.Status.OperatorVersion = ""
	ready, version, err := r.getOperatorStatus(ctx, req.NamespacedName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	if version != "" {
		dbEngine.Status.OperatorVersion = version
		dbEngine.Status.State = everestv1alpha1.DBEngineStateInstalling
	}

	defer func() {
		if updErr := r.Status().Update(ctx, dbEngine); updErr != nil {
			res = ctrl.Result{}
			err = updErr
		}
	}()

	// Not ready yet, check again later.
	if !ready {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	dbEngine.Status.State = everestv1alpha1.DBEngineStateInstalled
	matrix, err := r.versionService.GetVersions(engineType, dbEngine.Status.OperatorVersion)
	if err != nil {
		return ctrl.Result{}, err
	}

	versions := everestv1alpha1.Versions{
		Backup: matrix.Backup,
	}

	if dbEngine.Spec.Type == everestv1alpha1.DatabaseEnginePXC {
		for key := range matrix.PXC {
			// We do not need supporting mysql 5
			if strings.HasPrefix(key, "5") {
				delete(matrix.PXC, key)
			}
		}
		versions.Engine = matrix.PXC
		versions.Proxy = map[everestv1alpha1.ProxyType]everestv1alpha1.ComponentsMap{
			everestv1alpha1.ProxyTypeHAProxy:  matrix.HAProxy,
			everestv1alpha1.ProxyTypeProxySQL: matrix.ProxySQL,
		}
		versions.Tools = map[string]everestv1alpha1.ComponentsMap{
			"logCollector": matrix.LogCollector,
		}
	}

	if dbEngine.Spec.Type == everestv1alpha1.DatabaseEnginePSMDB {
		versions.Engine = matrix.Mongod
	}

	if dbEngine.Spec.Type == everestv1alpha1.DatabaseEnginePostgresql {
		versions.Engine = matrix.Postgresql
		versions.Backup = matrix.PGBackRest
		versions.Proxy = map[everestv1alpha1.ProxyType]everestv1alpha1.ComponentsMap{
			everestv1alpha1.ProxyTypePGBouncer: matrix.PGBouncer,
		}
	}
	dbEngine.Status.AvailableVersions = versions

	// Handle operator upgrade.
	if done, err := r.handleOperatorUpgrade(ctx, dbEngine); err != nil {
		return ctrl.Result{}, err
	} else if !done {
		// Not yet done, check again later.
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

// handleOperatorUpgrade handles operator upgrades for the database engine.
// Returns true if the upgrade is complete.
func (r *DatabaseEngineReconciler) handleOperatorUpgrade(
	ctx context.Context,
	dbEngine *everestv1alpha1.DatabaseEngine,
) (bool, error) {
	// Check if upgrade was requested?
	annotations := dbEngine.GetAnnotations()
	upgradeTo, found := annotations[everestv1alpha1.DatabaseOperatorUpgradeAnnotation]
	upgradeTo = strings.TrimPrefix(upgradeTo, "v")
	if !found {
		// upgrade not requested, we're done.
		dbEngine.Status.OperatorUpgrade = nil
		return true, nil
	}

	// Check if we're already at the desired version?
	if dbEngine.Status.OperatorVersion == upgradeTo {
		// Clean-up and return.
		dbEngine.Status.OperatorUpgrade = nil
		delete(annotations, everestv1alpha1.DatabaseOperatorUpgradeAnnotation)
		dbEngine.SetAnnotations(annotations)
		return true, r.Update(ctx, dbEngine)
	}

	if dbEngine.Status.OperatorUpgrade == nil {
		dbEngine.Status.OperatorUpgrade = &everestv1alpha1.OperatorUpgradeStatus{}
	}
	dbEngine.Status.OperatorUpgrade.TargetVersion = upgradeTo

	// TODO(EVEREST-961): Expose a list of available upgrade versions in the status
	// and check if 'upgradeTo' is listed in it.
	// This will ensure that we're always moving to a higher version.

	// List all InstallPlans in the namespace.
	ipList := &opfwv1alpha1.InstallPlanList{}
	if err := r.List(ctx, ipList, client.InNamespace(dbEngine.Namespace)); err != nil {
		return false, err
	}

	// Find the InstallPlan that contains the CSV we want to upgrade to.
	findIPWithCSV := func(targetCSVName string) *opfwv1alpha1.InstallPlan {
		for _, ip := range ipList.Items {
			for _, c := range ip.Spec.ClusterServiceVersionNames {
				if c == targetCSVName {
					return &ip
				}
			}
		}
		return nil
	}
	csvName := fmt.Sprintf("%s.v%s", dbEngine.GetName(), upgradeTo)
	foundIP := findIPWithCSV(csvName)
	if foundIP == nil {
		dbEngine.Status.OperatorUpgrade.Phase = everestv1alpha1.UpgradePhaseFailed
		dbEngine.Status.OperatorUpgrade.Message = fmt.Sprintf("InstallPlan for version '%s' not found", upgradeTo)
		return false, errInstallPlanNotFound
	}

	// Approve the InstallPlan if not done already.
	if foundIP.Status.Phase == opfwv1alpha1.InstallPlanPhaseRequiresApproval {
		foundIP.Spec.Approved = true
		if err := r.Update(ctx, foundIP); err != nil {
			dbEngine.Status.OperatorUpgrade.Phase = everestv1alpha1.UpgradePhaseFailed
			dbEngine.Status.OperatorUpgrade.Message = "Failed to approve InstallPlan: " + err.Error()
			return false, err
		}
		now := metav1.Now()
		dbEngine.Status.OperatorUpgrade.StartedAt = &now
	}

	dbEngine.Status.OperatorUpgrade.Phase = everestv1alpha1.UpgradePhaseStarted
	dbEngine.Status.OperatorUpgrade.Message = ""

	if foundIP.Status.Phase == opfwv1alpha1.InstallPlanPhaseComplete {
		dbEngine.Status.OperatorUpgrade.Phase = everestv1alpha1.UpgradePhaseCompleted
		return false, nil
	}

	if foundIP.Status.Phase == opfwv1alpha1.InstallPlanPhaseFailed {
		dbEngine.Status.OperatorUpgrade.Phase = everestv1alpha1.UpgradePhaseFailed
		return false, nil
	}

	return false, nil
}

func (r *DatabaseEngineReconciler) getOperatorStatus(ctx context.Context, name types.NamespacedName) (bool, string, error) {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, name, unstructuredResource); err != nil {
		return false, "", err
	}
	err := runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructuredResource.Object, deployment)
	if err != nil {
		return false, "", err
	}
	version := strings.Split(deployment.Spec.Template.Spec.Containers[0].Image, ":")[1]
	ready := deployment.Status.ReadyReplicas == deployment.Status.Replicas
	return ready, version, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseEngineReconciler) SetupWithManager(mgr ctrl.Manager, namespaces []string) error {
	// There's a good chance that the reconciler's client cache is not started
	// yet so we use the client.Reader returned from manager.GetAPIReader() to
	// hit the API server directly and avoid an ErrCacheNotStarted.
	clientReader := mgr.GetAPIReader()
	r.versionService = version.NewVersionService()
	for _, namespaceName := range namespaces {
		namespaceName := namespaceName
		for operatorName, engineType := range operatorEngine {
			dbEngine := &everestv1alpha1.DatabaseEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operatorName,
					Namespace: namespaceName,
				},
				Spec: everestv1alpha1.DatabaseEngineSpec{
					Type: engineType,
				},
			}

			found := &everestv1alpha1.DatabaseEngine{}
			err := clientReader.Get(context.Background(), types.NamespacedName{Name: dbEngine.Name, Namespace: dbEngine.Namespace}, found)
			if err != nil && apierrors.IsNotFound(err) {
				err = r.Create(context.Background(), dbEngine)
				if err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
		}
	}

	if err := opfwv1alpha1.AddToScheme(r.Scheme); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.DatabaseEngine{}).
		Watches(&appsv1.Deployment{}, &handler.EnqueueRequestForObject{}).
		Watches(&opfwv1alpha1.InstallPlan{}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}
