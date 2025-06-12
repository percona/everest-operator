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

// Package consts provides constants used across the operator.
package consts

const (

	// EverestAnnotationPrefix is the prefix for all Everest-related annotations.
	EverestAnnotationPrefix = "everest.percona.com/"
	// PauseReconcileAnnotation is the annotation used to pause reconciliation of a resource.
	PauseReconcileAnnotation = EverestAnnotationPrefix + "reconcile-paused"
	// PauseReconcileAnnotationValueTrue is the value for the PauseReconcileAnnotation to indicate that reconciliation is paused.
	PauseReconcileAnnotationValueTrue = "true"
	// RestartAnnotation is the annotation used to trigger a restart of a database cluster.
	RestartAnnotation = EverestAnnotationPrefix + "restart"
)
