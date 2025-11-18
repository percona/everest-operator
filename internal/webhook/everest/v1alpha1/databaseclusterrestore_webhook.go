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

package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	everestv1alpha1 "github.com/percona/everest-operator/api/everest/v1alpha1"
	"github.com/percona/everest-operator/utils"
)

var (
	// .spec.
	specPath = field.NewPath("spec")
	// .spec.dbClusterName.
	dbClusterNamePath = specPath.Child("dbClusterName")

	// .spec.dataSource.
	dataSourcePath = specPath.Child("dataSource")
	// .spec.dataSource.dbClusterBackupName.
	dbClusterBackupNamePath = dataSourcePath.Child("dbClusterBackupName")
	// .spec.dataSource.backupSource.
	backupSourcePath = dataSourcePath.Child("backupSource")
	// .spec.dataSource.pitr.type.
	dataSourcePitrTypePath = dataSourcePath.Child("type")
	// .spec.dataSource.pitr.date.
	dataSourcePitrDatePath = dataSourcePath.Child("date")

	// Required field error generator.
	errRequiredField = func(fieldPath *field.Path) *field.Error {
		return field.Required(fieldPath, "can not be empty")
	}
	// Immutable field error generator.
	errImmutableField = func(fieldPath *field.Path) *field.Error {
		return field.Forbidden(fieldPath, "is immutable and cannot be changed")
	}
	// Deletion errors.
	errDeleteInUse = errors.New("is used by some DB cluster and cannot be deleted")
)

// SetupDatabaseClusterRestoreWebhookWithManager registers the webhook for DatabaseClusterRestore in the manager.
func SetupDatabaseClusterRestoreWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&everestv1alpha1.DatabaseClusterRestore{}).
		WithValidator(&DatabaseClusterRestoreCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-everest-percona-com-v1alpha1-databaseclusterrestore,mutating=false,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=databaseclusterrestores,verbs=create;update;delete,versions=v1alpha1,name=vdatabaseclusterrestore-v1alpha1.everest.percona.com,admissionReviewVersions=v1

// DatabaseClusterRestoreCustomValidator struct is responsible for validating the DatabaseClusterRestore resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type DatabaseClusterRestoreCustomValidator struct {
	Client client.Client
}

var (
	_         webhook.CustomValidator = &DatabaseClusterRestoreCustomValidator{} //nolint:gci
	groupKind                         = everestv1alpha1.GroupVersion.WithKind("DatabaseClusterRestore").GroupKind()
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type DatabaseClusterRestore.
func (v *DatabaseClusterRestoreCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	dbcr, ok := obj.(*everestv1alpha1.DatabaseClusterRestore)
	if !ok {
		return nil, fmt.Errorf("expected a DatabaseClusterRestore object but got %T", obj)
	}
	logger := logf.FromContext(ctx).WithName("DatabaseClusterRestoreCustomValidator").WithValues(
		"name", dbcr.GetName(),
		"namespace", dbcr.GetNamespace(),
	)

	logger.Info("Validation for DatabaseClusterRestore upon creation", "name", dbcr.GetName())

	if dbcr.Spec.DBClusterName == "" {
		allErrs = append(allErrs, errRequiredField(dbClusterNamePath))
	} else {
		db := &everestv1alpha1.DatabaseCluster{}
		if err := v.Client.Get(ctx, types.NamespacedName{
			Name:      dbcr.Spec.DBClusterName,
			Namespace: dbcr.Namespace,
		}, db); apierrors.IsNotFound(err) {
			allErrs = append(allErrs, field.NotFound(dbClusterNamePath,
				fmt.Sprintf("DatabaseCluster %s not found in namespace %s", dbcr.Spec.DBClusterName, dbcr.Namespace)))
		}
	}

	if (dbcr.Spec.DataSource.DBClusterBackupName == "" && dbcr.Spec.DataSource.BackupSource == nil) ||
		(dbcr.Spec.DataSource.DBClusterBackupName != "" && dbcr.Spec.DataSource.BackupSource != nil) {
		allErrs = append(allErrs, field.Invalid(specPath, "",
			fmt.Sprintf("either %s or %s must be specified, but not both", dbClusterBackupNamePath, backupSourcePath)))
	}

	if errs := validatePitrRestoreSpec(dbcr.Spec.DataSource); errs != nil {
		allErrs = append(allErrs, errs...)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(groupKind, dbcr.GetName(), allErrs)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type DatabaseClusterRestore.
func (v *DatabaseClusterRestoreCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	oldDbcr, ok := oldObj.(*everestv1alpha1.DatabaseClusterRestore)
	if !ok {
		return nil, fmt.Errorf("expected a DatabaseClusterRestore object for the oldObj but got %T", oldObj)
	}

	newDbcr, ok := newObj.(*everestv1alpha1.DatabaseClusterRestore)
	if !ok {
		return nil, fmt.Errorf("expected a DatabaseClusterRestore object for the newObj but got %T", newObj)
	}
	logger := logf.FromContext(ctx).WithName("DatabaseClusterRestoreCustomValidator").WithValues(
		"name", oldDbcr.GetName(),
		"namespace", oldDbcr.GetNamespace(),
	)

	logger.Info("Validation for DatabaseClusterRestore upon update", "name", oldDbcr.GetName())

	if !newDbcr.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	if !equality.Semantic.DeepEqual(newDbcr.Spec, oldDbcr.Spec) {
		allErrs = append(allErrs, errImmutableField(specPath))
		return nil, apierrors.NewInvalid(groupKind, oldDbcr.GetName(), allErrs)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(groupKind, oldDbcr.GetName(), allErrs)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type DatabaseClusterRestore.
func (v *DatabaseClusterRestoreCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dbcr, ok := obj.(*everestv1alpha1.DatabaseClusterRestore)
	if !ok {
		return nil, fmt.Errorf("expected a DatabaseClusterRestore object but got %T", obj)
	}
	logger := logf.FromContext(ctx).WithName("DatabaseClusterRestoreCustomValidator").WithValues(
		"name", dbcr.GetName(),
		"namespace", dbcr.GetNamespace(),
	)
	logger.Info("Validation for DatabaseClusterRestore upon deletion", "name", dbcr.GetName())

	if !dbcr.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	// we should prevent deletion if it is currently in use.
	if utils.IsEverestObjectInUse(dbcr) {
		return nil, apierrors.NewForbidden(
			everestv1alpha1.GroupVersion.WithResource("databaseclusterrestore").GroupResource(),
			dbcr.GetName(),
			errDeleteInUse)
	}

	return nil, nil
}

// Helper functions

func validatePitrRestoreSpec(dataSource everestv1alpha1.DatabaseClusterRestoreDataSource) field.ErrorList {
	var allErrs field.ErrorList
	if dataSource.PITR == nil {
		return nil
	}

	switch dataSource.PITR.Type {
	case everestv1alpha1.PITRTypeDate:
		if dataSource.PITR.Date == nil {
			allErrs = append(allErrs, errRequiredField(dataSourcePitrDatePath))
		}
	default:
		// TODO: figure out why PITRTypeLatest doesn't work currently for Everest
		allErrs = append(allErrs, field.NotSupported(dataSourcePitrTypePath, dataSource.PITR.Type, []everestv1alpha1.PITRType{everestv1alpha1.PITRTypeDate}))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return allErrs
}
