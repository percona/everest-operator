// dbaas-operator
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
	"os"
	"strings"

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
	"sigs.k8s.io/controller-runtime/pkg/source"

	dbaasv1 "github.com/percona/dbaas-operator/api/v1"
)

var operatorEngine = map[string]dbaasv1.EngineType{
	pxcDeploymentName:   dbaasv1.PXCEngine,
	psmdbDeploymentName: dbaasv1.PSMDBEngine,
}

// DatabaseEngineReconciler reconciles a DatabaseEngine object.
type DatabaseEngineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseengines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseengines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseengines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *DatabaseEngineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	engineType, ok := operatorEngine[req.NamespacedName.Name]
	if !ok {
		// Unknown operator, nothing to do here
		return ctrl.Result{}, nil
	}

	dbEngine := &dbaasv1.DatabaseEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.NamespacedName.Name,
			Namespace: req.NamespacedName.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dbEngine, func() error {
		dbEngine.Spec.Type = engineType
		return nil
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	dbEngine.Status.State = dbaasv1.DBEngineStateNotInstalled
	dbEngine.Status.Version = ""
	ready, version, err := r.getOperatorStatus(ctx, req.NamespacedName)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	if version != "" {
		dbEngine.Status.Version = version
		dbEngine.Status.State = dbaasv1.DBEngineStateInstalling
	}
	if ready {
		dbEngine.Status.State = dbaasv1.DBEngineStateInstalled
	}

	if err := r.Status().Update(ctx, dbEngine); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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
func (r *DatabaseEngineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// There's a good chance that the reconciler's client cache is not started
	// yet so we use the client.Reader returned from manager.GetAPIReader() to
	// hit the API server directly and avoid an ErrCacheNotStarted.
	clientReader := mgr.GetAPIReader()

	for operatorName, engineType := range operatorEngine {
		dbEngine := &dbaasv1.DatabaseEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operatorName,
				Namespace: os.Getenv("WATCH_NAMESPACE"),
			},
			Spec: dbaasv1.DatabaseEngineSpec{
				Type: engineType,
			},
		}

		found := &dbaasv1.DatabaseEngine{}
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&dbaasv1.DatabaseEngine{}).
		Watches(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}
