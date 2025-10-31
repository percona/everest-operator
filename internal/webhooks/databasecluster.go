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

// Package webhooks ...
//
//nolint:lll
package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/AlekSi/pointer"
	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/controller/common"
)

var (
	errInvalidVersion              = errors.New("invalid database engine version provided")
	errDBEngineDowngrade           = errors.New("database engine version cannot be downgraded")
	errDBEngineMajorVersionUpgrade = errors.New("database engine cannot be upgraded to a major version")
	errDBEngineMajorUpgradeNotSeq  = errors.New("database engine major version upgrade is not supported for non-sequential versions")
	errDBEnginePXC80To84Upgrade    = errors.New("cannot upgrade from 8.0 to 8.4")
)

// SetupDatabaseClusterWebhookWithManager sets up the webhook with the manager.
func SetupDatabaseClusterWebhookWithManager(mgr manager.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&everestv1alpha1.DatabaseCluster{}).
		WithValidator(&DatabaseClusterValidator{
			Client: mgr.GetClient(),
		}).
		WithDefaulter(&DatabaseClusterDefaulter{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-everest-percona-com-v1alpha1-databasecluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=databaseclusters,verbs=create;update,versions=v1alpha1,name=vdatabasecluster-v1alpha1.everest.percona.com,admissionReviewVersions=v1

// DatabaseClusterValidator validates the DatabaseCluster resource.
type DatabaseClusterValidator struct {
	Client client.Client
}

// ValidateCreate validates the creation of a DatabaseCluster.
func (v *DatabaseClusterValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	db, ok := obj.(*everestv1alpha1.DatabaseCluster)
	if !ok {
		return nil, fmt.Errorf("expected a DatabaseCluster, got %T", obj)
	}

	log := log.FromContext(ctx).WithName("DatabaseClusterValidator").WithValues(
		"name", db.GetName(),
		"namespace", db.GetNamespace(),
	)

	// Validate the engine version
	if err := v.validateEngineVersion(ctx, db); err != nil {
		log.Error(err, "validateEngineVersion failed")
		return nil, fmt.Errorf("engine version validation failed: %w", err)
	}

	// If a user secret is specified by the user, ensure that it exists.
	if userSecretsName := db.Spec.Engine.UserSecretsName; userSecretsName != "" {
		// ensure that this secret exists.
		secret := corev1.Secret{}
		if err := v.Client.Get(ctx, types.NamespacedName{
			Name:      userSecretsName,
			Namespace: db.GetNamespace(),
		}, &secret); err != nil {
			return nil, fmt.Errorf("failed to get user secrets %s: %w", userSecretsName, err)
		}
	}

	// If a data import source is specified, validate it.
	if di := pointer.Get(db.Spec.DataSource).DataImport; di != nil {
		if err := v.validateDataImport(ctx, db); err != nil {
			log.Error(err, "validateDataImport failed (ValidateCreate)")
			return nil, fmt.Errorf("data import validation failed: %w", err)
		}
	}

	var warns admission.Warnings
	// Check if we have enough memory for PXC 8.4
	if !validatePXC84Memory(db) {
		warns = append(warns, fmt.Sprintf("insufficient memory '%s' for PXC 8.4, cluster may not start", db.Spec.Engine.Resources.Memory.String()))
	}
	return warns, v.ValidateLoadBalancerConfig(ctx, db.Spec.Proxy.Expose.LoadBalancerConfigName)
}

// minimum required memory for PXC 8.4
var minMemoryForPXC84 = resource.MustParse("3G")

func validatePXC84Memory(db *everestv1alpha1.DatabaseCluster) bool {
	engineSpec := db.Spec.Engine
	return engineSpec.Type != everestv1alpha1.DatabaseEnginePXC ||
		!isPXCv84(engineSpec.Version) ||
		engineSpec.Resources.Memory.Cmp(minMemoryForPXC84) >= 0
}

// ValidateUpdate validates the update of a DatabaseCluster.
func (v *DatabaseClusterValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldDB, ok := oldObj.(*everestv1alpha1.DatabaseCluster)
	if !ok {
		return nil, fmt.Errorf("expected a DatabaseCluster, got %T", oldObj)
	}
	newDB, ok := newObj.(*everestv1alpha1.DatabaseCluster)
	if !ok {
		return nil, fmt.Errorf("expected a DatabaseCluster, got %T", newObj)
	}

	log := log.FromContext(ctx).WithName("DatabaseClusterValidator").WithValues(
		"name", newDB.GetName(),
		"namespace", newDB.GetNamespace(),
	)

	// Validate the engine version
	if err := v.validateEngineVersion(ctx, newDB); err != nil {
		log.Error(err, "validateEngineVersion failed (ValidateUpdate)")
		return nil, fmt.Errorf("engine version validation failed: %w", err)
	}

	// Validate engine version change
	if oldDB.Spec.Engine.Version != newDB.Spec.Engine.Version {
		if err := validateEngineVersionChange(newDB.Spec.Engine.Type, oldDB.Spec.Engine.Version, newDB.Spec.Engine.Version); err != nil {
			return nil, fmt.Errorf("invalid version change: %w", err)
		}
	}

	var warns admission.Warnings
	// Check if we have enough memory for PXC 8.4
	if !validatePXC84Memory(newDB) {
		warns = append(warns, fmt.Sprintf("insufficient memory '%s' for PXC 8.4, cluster may not start", newDB.Spec.Engine.Resources.Memory.String()))
	}

	// TODO: move remaining validations from Everest API
	// 1. Validate replica count change
	// 2. Validate storage size change
	// 3. Validate sharding constraints
	return warns, nil
}

func semverNormalize(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func isPXCv84(version string) bool {
	return semver.MajorMinor(semverNormalize(version)) == "v8.4"
}

func isPXCv80(version string) bool {
	return semver.MajorMinor(semverNormalize(version)) == "v8.0"
}

func validateEngineVersionChange(
	engineType everestv1alpha1.EngineType,
	oldVersion, newVersion string,
) error {
	newVersion = semverNormalize(newVersion)
	oldVersion = semverNormalize(oldVersion)

	// Check semver validity.
	if !semver.IsValid(newVersion) {
		return errInvalidVersion
	}

	// We will not allow downgrades.
	if semver.Compare(newVersion, oldVersion) < 0 {
		return errDBEngineDowngrade
	}
	// We will not allow major upgrades for PXC and PG.
	// - PXC: Major upgrades are not supported.
	// - PG: Major upgrades are in technical preview. https://docs.percona.com/percona-operator-for-postgresql/2.0/update.html#major-version-upgrade
	if engineType != everestv1alpha1.DatabaseEnginePSMDB && semver.Major(oldVersion) != semver.Major(newVersion) {
		return errDBEngineMajorVersionUpgrade
	}

	// We will not allow upgrade of PXC from 8.0 to 8.4
	if engineType == everestv1alpha1.DatabaseEnginePXC && isPXCv80(oldVersion) && isPXCv84(newVersion) {
		return errDBEnginePXC80To84Upgrade
	}

	// It's fine to ignore the errors here because we have already validated the version.
	newMajorInt, _ := strconv.Atoi(semver.Major(newVersion)[1:])
	oldMajorInt, _ := strconv.Atoi(semver.Major(oldVersion)[1:])
	// We will not allow major upgrades if the versions are not sequential.
	if newMajorInt-oldMajorInt > 1 {
		return errDBEngineMajorUpgradeNotSeq
	}
	return nil
}

// ValidateDelete validates the deletion of a DatabaseCluster.
func (v *DatabaseClusterValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *DatabaseClusterValidator) validateDataImport(
	ctx context.Context,
	db *everestv1alpha1.DatabaseCluster,
) error {
	dataImport := pointer.Get(db.Spec.DataSource).DataImport
	if dataImport == nil {
		return nil
	}

	dataimporter := &everestv1alpha1.DataImporter{}
	if err := v.Client.Get(ctx, types.NamespacedName{
		Name: dataImport.DataImporterName,
	}, dataimporter); err != nil {
		return fmt.Errorf("failed to get DataImporter: %w", err)
	}

	// Validate that the DataImporter supports the specified engine.
	engineType := db.Spec.Engine.Type
	if !dataimporter.Spec.SupportedEngines.Has(engineType) {
		return fmt.Errorf("data importer %s does not support engine %s", dataImport.DataImporterName, engineType)
	}

	if requiredFields := dataimporter.Spec.DatabaseClusterConstraints.RequiredFields; len(requiredFields) > 0 {
		if err := validateDataImportRequiredFields(requiredFields, db); err != nil {
			return fmt.Errorf("data import validation failed: %w", err)
		}
	}

	// Validate the params against the schema of the DataImporter
	return dataimporter.Spec.Config.Validate(dataImport.Config)
}

func validateDataImportRequiredFields(requiredFields []string, db *everestv1alpha1.DatabaseCluster) error {
	for _, field := range requiredFields {
		exists, err := checkJSONKeyExists(field, db)
		if err != nil {
			return fmt.Errorf("error checking key %s: %w", field, err)
		}
		if !exists {
			return fmt.Errorf("required field %s is missing in databasecluster", field)
		}
	}
	return nil
}

// checkJSONKeyExists returns true if the specified key exists in the JSON object.
// The keyExpr is a dot-separated string representing the path to the key in the JSON object.
func checkJSONKeyExists(keyExpr string, obj any) (bool, error) {
	objB, err := json.Marshal(obj)
	if err != nil {
		return false, fmt.Errorf("failed to marshal object: %w", err)
	}
	objMap := make(map[string]any)
	if err := json.Unmarshal(objB, &objMap); err != nil {
		return false, fmt.Errorf("failed to unmarshal object: %w", err)
	}

	subKeys := slices.DeleteFunc(strings.Split(keyExpr, "."), func(s string) bool {
		return s == ""
	})
	currentSubObject := objMap
	for _, key := range subKeys {
		if value, exists := currentSubObject[key]; exists {
			switch v := value.(type) {
			case map[string]any:
				currentSubObject = v
				continue
			case int, int32, int64, float32, float64:
				return value != 0, nil
			case string:
				return value != "", nil
			}
			continue
		}
		return false, nil
	}
	return true, nil
}

func (v *DatabaseClusterValidator) validateEngineVersion(ctx context.Context, db *everestv1alpha1.DatabaseCluster) error {
	// Get the DatabaseEngine
	engine, err := common.GetDatabaseEngineForType(ctx, v.Client, db.Spec.Engine.Type, db.GetNamespace())
	if err != nil {
		return fmt.Errorf("failed to get engine: %w", err)
	}
	// Check if the engine version is available
	_, ok := engine.Status.AvailableVersions.Engine[db.Spec.Engine.Version]
	if !ok {
		return fmt.Errorf("engine version %s not available", db.Spec.Engine.Version)
	}
	return nil
}

// ValidateLoadBalancerConfig validates if the LoadBalancerConfig with the given name exists.
func (v *DatabaseClusterValidator) ValidateLoadBalancerConfig(ctx context.Context, lbcName string) error {
	if lbcName == "" {
		return nil
	}

	lbc := everestv1alpha1.LoadBalancerConfig{}

	err := v.Client.Get(ctx, client.ObjectKey{
		Name: lbcName,
	}, &lbc)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("load balancer config %s not found: %w", lbcName, err)
		}

		return fmt.Errorf("failed to get load balancer config %s: %w", lbcName, err)
	}

	return nil
}
