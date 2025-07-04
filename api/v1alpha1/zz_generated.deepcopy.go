//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AffinityConfig) DeepCopyInto(out *AffinityConfig) {
	*out = *in
	if in.PXC != nil {
		in, out := &in.PXC, &out.PXC
		*out = new(PXCAffinityConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.PostgreSQL != nil {
		in, out := &in.PostgreSQL, &out.PostgreSQL
		*out = new(PostgreSQLAffinityConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.PSMDB != nil {
		in, out := &in.PSMDB, &out.PSMDB
		*out = new(PSMDBAffinityConfig)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AffinityConfig.
func (in *AffinityConfig) DeepCopy() *AffinityConfig {
	if in == nil {
		return nil
	}
	out := new(AffinityConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Backup) DeepCopyInto(out *Backup) {
	*out = *in
	if in.Schedules != nil {
		in, out := &in.Schedules, &out.Schedules
		*out = make([]BackupSchedule, len(*in))
		copy(*out, *in)
	}
	in.PITR.DeepCopyInto(&out.PITR)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Backup.
func (in *Backup) DeepCopy() *Backup {
	if in == nil {
		return nil
	}
	out := new(Backup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupSchedule) DeepCopyInto(out *BackupSchedule) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupSchedule.
func (in *BackupSchedule) DeepCopy() *BackupSchedule {
	if in == nil {
		return nil
	}
	out := new(BackupSchedule)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupSource) DeepCopyInto(out *BackupSource) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupSource.
func (in *BackupSource) DeepCopy() *BackupSource {
	if in == nil {
		return nil
	}
	out := new(BackupSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupStorage) DeepCopyInto(out *BackupStorage) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupStorage.
func (in *BackupStorage) DeepCopy() *BackupStorage {
	if in == nil {
		return nil
	}
	out := new(BackupStorage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BackupStorage) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupStorageList) DeepCopyInto(out *BackupStorageList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]BackupStorage, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupStorageList.
func (in *BackupStorageList) DeepCopy() *BackupStorageList {
	if in == nil {
		return nil
	}
	out := new(BackupStorageList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BackupStorageList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupStorageProviderSpec) DeepCopyInto(out *BackupStorageProviderSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupStorageProviderSpec.
func (in *BackupStorageProviderSpec) DeepCopy() *BackupStorageProviderSpec {
	if in == nil {
		return nil
	}
	out := new(BackupStorageProviderSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupStorageSpec) DeepCopyInto(out *BackupStorageSpec) {
	*out = *in
	if in.VerifyTLS != nil {
		in, out := &in.VerifyTLS, &out.VerifyTLS
		*out = new(bool)
		**out = **in
	}
	if in.ForcePathStyle != nil {
		in, out := &in.ForcePathStyle, &out.ForcePathStyle
		*out = new(bool)
		**out = **in
	}
	if in.AllowedNamespaces != nil {
		in, out := &in.AllowedNamespaces, &out.AllowedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupStorageSpec.
func (in *BackupStorageSpec) DeepCopy() *BackupStorageSpec {
	if in == nil {
		return nil
	}
	out := new(BackupStorageSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BackupStorageStatus) DeepCopyInto(out *BackupStorageStatus) {
	*out = *in
	if in.UsedNamespaces != nil {
		in, out := &in.UsedNamespaces, &out.UsedNamespaces
		*out = make(map[string]bool, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BackupStorageStatus.
func (in *BackupStorageStatus) DeepCopy() *BackupStorageStatus {
	if in == nil {
		return nil
	}
	out := new(BackupStorageStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Component) DeepCopyInto(out *Component) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Component.
func (in *Component) DeepCopy() *Component {
	if in == nil {
		return nil
	}
	out := new(Component)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in ComponentsMap) DeepCopyInto(out *ComponentsMap) {
	{
		in := &in
		*out = make(ComponentsMap, len(*in))
		for key, val := range *in {
			var outVal *Component
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = new(Component)
				**out = **in
			}
			(*out)[key] = outVal
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ComponentsMap.
func (in ComponentsMap) DeepCopy() ComponentsMap {
	if in == nil {
		return nil
	}
	out := new(ComponentsMap)
	in.DeepCopyInto(out)
	return *out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConfigServer) DeepCopyInto(out *ConfigServer) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConfigServer.
func (in *ConfigServer) DeepCopy() *ConfigServer {
	if in == nil {
		return nil
	}
	out := new(ConfigServer)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DataSource) DeepCopyInto(out *DataSource) {
	*out = *in
	if in.BackupSource != nil {
		in, out := &in.BackupSource, &out.BackupSource
		*out = new(BackupSource)
		**out = **in
	}
	if in.PITR != nil {
		in, out := &in.PITR, &out.PITR
		*out = new(PITR)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DataSource.
func (in *DataSource) DeepCopy() *DataSource {
	if in == nil {
		return nil
	}
	out := new(DataSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseCluster) DeepCopyInto(out *DatabaseCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseCluster.
func (in *DatabaseCluster) DeepCopy() *DatabaseCluster {
	if in == nil {
		return nil
	}
	out := new(DatabaseCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterBackup) DeepCopyInto(out *DatabaseClusterBackup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterBackup.
func (in *DatabaseClusterBackup) DeepCopy() *DatabaseClusterBackup {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterBackup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseClusterBackup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterBackupList) DeepCopyInto(out *DatabaseClusterBackupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseClusterBackup, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterBackupList.
func (in *DatabaseClusterBackupList) DeepCopy() *DatabaseClusterBackupList {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterBackupList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseClusterBackupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterBackupSpec) DeepCopyInto(out *DatabaseClusterBackupSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterBackupSpec.
func (in *DatabaseClusterBackupSpec) DeepCopy() *DatabaseClusterBackupSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterBackupSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterBackupStatus) DeepCopyInto(out *DatabaseClusterBackupStatus) {
	*out = *in
	if in.CreatedAt != nil {
		in, out := &in.CreatedAt, &out.CreatedAt
		*out = (*in).DeepCopy()
	}
	if in.CompletedAt != nil {
		in, out := &in.CompletedAt, &out.CompletedAt
		*out = (*in).DeepCopy()
	}
	if in.Destination != nil {
		in, out := &in.Destination, &out.Destination
		*out = new(string)
		**out = **in
	}
	if in.LatestRestorableTime != nil {
		in, out := &in.LatestRestorableTime, &out.LatestRestorableTime
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterBackupStatus.
func (in *DatabaseClusterBackupStatus) DeepCopy() *DatabaseClusterBackupStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterBackupStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterList) DeepCopyInto(out *DatabaseClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterList.
func (in *DatabaseClusterList) DeepCopy() *DatabaseClusterList {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterRestore) DeepCopyInto(out *DatabaseClusterRestore) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterRestore.
func (in *DatabaseClusterRestore) DeepCopy() *DatabaseClusterRestore {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterRestore)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseClusterRestore) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterRestoreList) DeepCopyInto(out *DatabaseClusterRestoreList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseClusterRestore, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterRestoreList.
func (in *DatabaseClusterRestoreList) DeepCopy() *DatabaseClusterRestoreList {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterRestoreList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseClusterRestoreList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterRestoreSpec) DeepCopyInto(out *DatabaseClusterRestoreSpec) {
	*out = *in
	in.DataSource.DeepCopyInto(&out.DataSource)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterRestoreSpec.
func (in *DatabaseClusterRestoreSpec) DeepCopy() *DatabaseClusterRestoreSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterRestoreSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterRestoreStatus) DeepCopyInto(out *DatabaseClusterRestoreStatus) {
	*out = *in
	if in.CompletedAt != nil {
		in, out := &in.CompletedAt, &out.CompletedAt
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterRestoreStatus.
func (in *DatabaseClusterRestoreStatus) DeepCopy() *DatabaseClusterRestoreStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterRestoreStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterSpec) DeepCopyInto(out *DatabaseClusterSpec) {
	*out = *in
	in.Engine.DeepCopyInto(&out.Engine)
	in.Proxy.DeepCopyInto(&out.Proxy)
	if in.DataSource != nil {
		in, out := &in.DataSource, &out.DataSource
		*out = new(DataSource)
		(*in).DeepCopyInto(*out)
	}
	in.Backup.DeepCopyInto(&out.Backup)
	if in.Monitoring != nil {
		in, out := &in.Monitoring, &out.Monitoring
		*out = new(Monitoring)
		(*in).DeepCopyInto(*out)
	}
	if in.Sharding != nil {
		in, out := &in.Sharding, &out.Sharding
		*out = new(Sharding)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterSpec.
func (in *DatabaseClusterSpec) DeepCopy() *DatabaseClusterSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseClusterStatus) DeepCopyInto(out *DatabaseClusterStatus) {
	*out = *in
	if in.RecommendedCRVersion != nil {
		in, out := &in.RecommendedCRVersion, &out.RecommendedCRVersion
		*out = new(string)
		**out = **in
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseClusterStatus.
func (in *DatabaseClusterStatus) DeepCopy() *DatabaseClusterStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseEngine) DeepCopyInto(out *DatabaseEngine) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseEngine.
func (in *DatabaseEngine) DeepCopy() *DatabaseEngine {
	if in == nil {
		return nil
	}
	out := new(DatabaseEngine)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseEngine) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseEngineList) DeepCopyInto(out *DatabaseEngineList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]DatabaseEngine, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseEngineList.
func (in *DatabaseEngineList) DeepCopy() *DatabaseEngineList {
	if in == nil {
		return nil
	}
	out := new(DatabaseEngineList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *DatabaseEngineList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseEngineSpec) DeepCopyInto(out *DatabaseEngineSpec) {
	*out = *in
	if in.AllowedVersions != nil {
		in, out := &in.AllowedVersions, &out.AllowedVersions
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseEngineSpec.
func (in *DatabaseEngineSpec) DeepCopy() *DatabaseEngineSpec {
	if in == nil {
		return nil
	}
	out := new(DatabaseEngineSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DatabaseEngineStatus) DeepCopyInto(out *DatabaseEngineStatus) {
	*out = *in
	in.AvailableVersions.DeepCopyInto(&out.AvailableVersions)
	if in.PendingOperatorUpgrades != nil {
		in, out := &in.PendingOperatorUpgrades, &out.PendingOperatorUpgrades
		*out = make([]OperatorUpgrade, len(*in))
		copy(*out, *in)
	}
	if in.OperatorUpgrade != nil {
		in, out := &in.OperatorUpgrade, &out.OperatorUpgrade
		*out = new(OperatorUpgradeStatus)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DatabaseEngineStatus.
func (in *DatabaseEngineStatus) DeepCopy() *DatabaseEngineStatus {
	if in == nil {
		return nil
	}
	out := new(DatabaseEngineStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Engine) DeepCopyInto(out *Engine) {
	*out = *in
	in.Storage.DeepCopyInto(&out.Storage)
	in.Resources.DeepCopyInto(&out.Resources)
	if in.CRVersion != nil {
		in, out := &in.CRVersion, &out.CRVersion
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Engine.
func (in *Engine) DeepCopy() *Engine {
	if in == nil {
		return nil
	}
	out := new(Engine)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Expose) DeepCopyInto(out *Expose) {
	*out = *in
	if in.IPSourceRanges != nil {
		in, out := &in.IPSourceRanges, &out.IPSourceRanges
		*out = make([]IPSourceRange, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Expose.
func (in *Expose) DeepCopy() *Expose {
	if in == nil {
		return nil
	}
	out := new(Expose)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Monitoring) DeepCopyInto(out *Monitoring) {
	*out = *in
	in.Resources.DeepCopyInto(&out.Resources)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Monitoring.
func (in *Monitoring) DeepCopy() *Monitoring {
	if in == nil {
		return nil
	}
	out := new(Monitoring)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonitoringConfig) DeepCopyInto(out *MonitoringConfig) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonitoringConfig.
func (in *MonitoringConfig) DeepCopy() *MonitoringConfig {
	if in == nil {
		return nil
	}
	out := new(MonitoringConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MonitoringConfig) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonitoringConfigList) DeepCopyInto(out *MonitoringConfigList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]MonitoringConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonitoringConfigList.
func (in *MonitoringConfigList) DeepCopy() *MonitoringConfigList {
	if in == nil {
		return nil
	}
	out := new(MonitoringConfigList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *MonitoringConfigList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonitoringConfigSpec) DeepCopyInto(out *MonitoringConfigSpec) {
	*out = *in
	if in.AllowedNamespaces != nil {
		in, out := &in.AllowedNamespaces, &out.AllowedNamespaces
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.PMM = in.PMM
	if in.VerifyTLS != nil {
		in, out := &in.VerifyTLS, &out.VerifyTLS
		*out = new(bool)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonitoringConfigSpec.
func (in *MonitoringConfigSpec) DeepCopy() *MonitoringConfigSpec {
	if in == nil {
		return nil
	}
	out := new(MonitoringConfigSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonitoringConfigStatus) DeepCopyInto(out *MonitoringConfigStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonitoringConfigStatus.
func (in *MonitoringConfigStatus) DeepCopy() *MonitoringConfigStatus {
	if in == nil {
		return nil
	}
	out := new(MonitoringConfigStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OperatorUpgrade) DeepCopyInto(out *OperatorUpgrade) {
	*out = *in
	out.InstallPlanRef = in.InstallPlanRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OperatorUpgrade.
func (in *OperatorUpgrade) DeepCopy() *OperatorUpgrade {
	if in == nil {
		return nil
	}
	out := new(OperatorUpgrade)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *OperatorUpgradeStatus) DeepCopyInto(out *OperatorUpgradeStatus) {
	*out = *in
	out.OperatorUpgrade = in.OperatorUpgrade
	if in.StartedAt != nil {
		in, out := &in.StartedAt, &out.StartedAt
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new OperatorUpgradeStatus.
func (in *OperatorUpgradeStatus) DeepCopy() *OperatorUpgradeStatus {
	if in == nil {
		return nil
	}
	out := new(OperatorUpgradeStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PITR) DeepCopyInto(out *PITR) {
	*out = *in
	if in.Date != nil {
		in, out := &in.Date, &out.Date
		*out = new(RestoreDate)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PITR.
func (in *PITR) DeepCopy() *PITR {
	if in == nil {
		return nil
	}
	out := new(PITR)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PITRSpec) DeepCopyInto(out *PITRSpec) {
	*out = *in
	if in.BackupStorageName != nil {
		in, out := &in.BackupStorageName, &out.BackupStorageName
		*out = new(string)
		**out = **in
	}
	if in.UploadIntervalSec != nil {
		in, out := &in.UploadIntervalSec, &out.UploadIntervalSec
		*out = new(int)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PITRSpec.
func (in *PITRSpec) DeepCopy() *PITRSpec {
	if in == nil {
		return nil
	}
	out := new(PITRSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PMMConfig) DeepCopyInto(out *PMMConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PMMConfig.
func (in *PMMConfig) DeepCopy() *PMMConfig {
	if in == nil {
		return nil
	}
	out := new(PMMConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PSMDBAffinityConfig) DeepCopyInto(out *PSMDBAffinityConfig) {
	*out = *in
	if in.Engine != nil {
		in, out := &in.Engine, &out.Engine
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Proxy != nil {
		in, out := &in.Proxy, &out.Proxy
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.ConfigServer != nil {
		in, out := &in.ConfigServer, &out.ConfigServer
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PSMDBAffinityConfig.
func (in *PSMDBAffinityConfig) DeepCopy() *PSMDBAffinityConfig {
	if in == nil {
		return nil
	}
	out := new(PSMDBAffinityConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PXCAffinityConfig) DeepCopyInto(out *PXCAffinityConfig) {
	*out = *in
	if in.Engine != nil {
		in, out := &in.Engine, &out.Engine
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Proxy != nil {
		in, out := &in.Proxy, &out.Proxy
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PXCAffinityConfig.
func (in *PXCAffinityConfig) DeepCopy() *PXCAffinityConfig {
	if in == nil {
		return nil
	}
	out := new(PXCAffinityConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodSchedulingPolicy) DeepCopyInto(out *PodSchedulingPolicy) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodSchedulingPolicy.
func (in *PodSchedulingPolicy) DeepCopy() *PodSchedulingPolicy {
	if in == nil {
		return nil
	}
	out := new(PodSchedulingPolicy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodSchedulingPolicy) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodSchedulingPolicyList) DeepCopyInto(out *PodSchedulingPolicyList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PodSchedulingPolicy, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodSchedulingPolicyList.
func (in *PodSchedulingPolicyList) DeepCopy() *PodSchedulingPolicyList {
	if in == nil {
		return nil
	}
	out := new(PodSchedulingPolicyList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodSchedulingPolicyList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodSchedulingPolicySpec) DeepCopyInto(out *PodSchedulingPolicySpec) {
	*out = *in
	if in.AffinityConfig != nil {
		in, out := &in.AffinityConfig, &out.AffinityConfig
		*out = new(AffinityConfig)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodSchedulingPolicySpec.
func (in *PodSchedulingPolicySpec) DeepCopy() *PodSchedulingPolicySpec {
	if in == nil {
		return nil
	}
	out := new(PodSchedulingPolicySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodSchedulingPolicyStatus) DeepCopyInto(out *PodSchedulingPolicyStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodSchedulingPolicyStatus.
func (in *PodSchedulingPolicyStatus) DeepCopy() *PodSchedulingPolicyStatus {
	if in == nil {
		return nil
	}
	out := new(PodSchedulingPolicyStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PostgreSQLAffinityConfig) DeepCopyInto(out *PostgreSQLAffinityConfig) {
	*out = *in
	if in.Engine != nil {
		in, out := &in.Engine, &out.Engine
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Proxy != nil {
		in, out := &in.Proxy, &out.Proxy
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PostgreSQLAffinityConfig.
func (in *PostgreSQLAffinityConfig) DeepCopy() *PostgreSQLAffinityConfig {
	if in == nil {
		return nil
	}
	out := new(PostgreSQLAffinityConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Proxy) DeepCopyInto(out *Proxy) {
	*out = *in
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	in.Expose.DeepCopyInto(&out.Expose)
	in.Resources.DeepCopyInto(&out.Resources)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Proxy.
func (in *Proxy) DeepCopy() *Proxy {
	if in == nil {
		return nil
	}
	out := new(Proxy)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Resources) DeepCopyInto(out *Resources) {
	*out = *in
	out.CPU = in.CPU.DeepCopy()
	out.Memory = in.Memory.DeepCopy()
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Resources.
func (in *Resources) DeepCopy() *Resources {
	if in == nil {
		return nil
	}
	out := new(Resources)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RestoreDate) DeepCopyInto(out *RestoreDate) {
	*out = *in
	in.Time.DeepCopyInto(&out.Time)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RestoreDate.
func (in *RestoreDate) DeepCopy() *RestoreDate {
	if in == nil {
		return nil
	}
	out := new(RestoreDate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Sharding) DeepCopyInto(out *Sharding) {
	*out = *in
	out.ConfigServer = in.ConfigServer
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Sharding.
func (in *Sharding) DeepCopy() *Sharding {
	if in == nil {
		return nil
	}
	out := new(Sharding)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Storage) DeepCopyInto(out *Storage) {
	*out = *in
	out.Size = in.Size.DeepCopy()
	if in.Class != nil {
		in, out := &in.Class, &out.Class
		*out = new(string)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Storage.
func (in *Storage) DeepCopy() *Storage {
	if in == nil {
		return nil
	}
	out := new(Storage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Versions) DeepCopyInto(out *Versions) {
	*out = *in
	if in.Engine != nil {
		in, out := &in.Engine, &out.Engine
		*out = make(ComponentsMap, len(*in))
		for key, val := range *in {
			var outVal *Component
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = new(Component)
				**out = **in
			}
			(*out)[key] = outVal
		}
	}
	if in.Backup != nil {
		in, out := &in.Backup, &out.Backup
		*out = make(ComponentsMap, len(*in))
		for key, val := range *in {
			var outVal *Component
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = new(Component)
				**out = **in
			}
			(*out)[key] = outVal
		}
	}
	if in.Proxy != nil {
		in, out := &in.Proxy, &out.Proxy
		*out = make(map[ProxyType]ComponentsMap, len(*in))
		for key, val := range *in {
			var outVal map[string]*Component
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(ComponentsMap, len(*in))
				for key, val := range *in {
					var outVal *Component
					if val == nil {
						(*out)[key] = nil
					} else {
						inVal := (*in)[key]
						in, out := &inVal, &outVal
						*out = new(Component)
						**out = **in
					}
					(*out)[key] = outVal
				}
			}
			(*out)[key] = outVal
		}
	}
	if in.Tools != nil {
		in, out := &in.Tools, &out.Tools
		*out = make(map[string]ComponentsMap, len(*in))
		for key, val := range *in {
			var outVal map[string]*Component
			if val == nil {
				(*out)[key] = nil
			} else {
				inVal := (*in)[key]
				in, out := &inVal, &outVal
				*out = make(ComponentsMap, len(*in))
				for key, val := range *in {
					var outVal *Component
					if val == nil {
						(*out)[key] = nil
					} else {
						inVal := (*in)[key]
						in, out := &inVal, &outVal
						*out = new(Component)
						**out = **in
					}
					(*out)[key] = outVal
				}
			}
			(*out)[key] = outVal
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Versions.
func (in *Versions) DeepCopy() *Versions {
	if in == nil {
		return nil
	}
	out := new(Versions)
	in.DeepCopyInto(out)
	return out
}
