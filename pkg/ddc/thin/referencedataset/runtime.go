/*
  Copyright 2022 The Fluid Authors.

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

package referencedataset

import (
	"context"
	"fmt"

	datav1alpha1 "github.com/fluid-cloudnative/fluid/api/v1alpha1"
	"github.com/fluid-cloudnative/fluid/pkg/ddc/base"
	"github.com/fluid-cloudnative/fluid/pkg/utils"
	"k8s.io/apimachinery/pkg/types"
)

// getMountedDatasetRuntimeStatus get the runtime status of the mounted dataset
func (e *ReferenceDatasetEngine) getMountedDatasetRuntimeStatus() (status *datav1alpha1.RuntimeStatus, err error) {
	mountedRuntimeInfo, err := e.getMountedRuntimeInfo()
	if err != nil {
		return status, err
	}

	// if mountedRuntimeInfo is nil and no err, the runtime is deleting.
	if mountedRuntimeInfo == nil {
		return nil, nil
	}

	return base.GetRuntimeStatus(e.Client, mountedRuntimeInfo.GetRuntimeType(),
		mountedRuntimeInfo.GetName(), mountedRuntimeInfo.GetNamespace())
}

// getRuntime get the current runtime
func (e *ReferenceDatasetEngine) getRuntime() (*datav1alpha1.ThinRuntime, error) {
	key := types.NamespacedName{
		Name:      e.name,
		Namespace: e.namespace,
	}

	var runtime datav1alpha1.ThinRuntime
	if err := e.Get(context.TODO(), key, &runtime); err != nil {
		return nil, err
	}

	return &runtime, nil
}

func (e *ReferenceDatasetEngine) getRuntimeInfo() (base.RuntimeInfoInterface, error) {
	if e.runtimeInfo != nil {
		return e.runtimeInfo, nil
	}

	runtime, err := e.getRuntime()
	if err != nil {
		return e.runtimeInfo, err
	}

	e.runtimeInfo, err = base.BuildRuntimeInfo(e.name, e.namespace, e.runtimeType, runtime.Spec.TieredStore, base.WithMetadataList(base.GetMetadataListFromAnnotation(runtime)))
	if err != nil {
		return e.runtimeInfo, err
	}

	// Setup Fuse Deploy Mode
	e.runtimeInfo.SetupFuseDeployMode(true, runtime.Spec.Fuse.NodeSelector)

	// Ignore the deprecated common labels and PersistentVolumes, use physical runtime

	// Setup with Dataset Info
	dataset, err := utils.GetDataset(e.Client, e.name, e.namespace)
	if err != nil {
		if utils.IgnoreNotFound(err) == nil {
			e.Log.Info("Dataset is notfound", "name", e.name, "namespace", e.namespace)
			return e.runtimeInfo, nil
		}

		e.Log.Info("Failed to get dataset when get runtimeInfo")
		return e.runtimeInfo, err
	}

	// set exclusive mode
	// TODO: how to handle the exclusive mode ?
	e.runtimeInfo.SetupWithDataset(dataset)

	e.Log.Info("Setup with dataset done", "exclusive", e.runtimeInfo.IsExclusive())

	return e.runtimeInfo, nil
}

// getMountedRuntimeInfo get mountedRuntimeInfo from dataset.
// If could not get dataset, getMountedRuntimeInfo try to get mountedRuntimeInfo from runtime status.
// And if dataset is deleted and no status.mounts, it returns nil, nil to continue runtimeDeletion.
func (e *ReferenceDatasetEngine) getMountedRuntimeInfo() (base.RuntimeInfoInterface, error) {
	if e.mountedRuntimeInfo != nil {
		return e.mountedRuntimeInfo, nil
	}

	dataset, err := utils.GetDataset(e.Client, e.name, e.namespace)
	// if err is not found, try to get mountedRuntimeInfo from runtime status, don't return here.
	if err != nil && utils.IgnoreNotFound(err) != nil {
		return e.mountedRuntimeInfo, err
	}

	runtime, err := e.getRuntime()
	if err != nil {
		return e.mountedRuntimeInfo, err
	}

	var mountedNameSpacedNames []types.NamespacedName
	if dataset != nil {
		// get mountedRuntimeInfo from dataset first
		mountedNameSpacedNames = base.GetMountedDatasetNamespacedName(dataset.Spec.Mounts)
	} else if runtime.Status.Mounts != nil && len(runtime.Status.Mounts) != 0 {
		// then try to get mountedRuntimeInfo from runtime status
		mountedNameSpacedNames = base.GetMountedDatasetNamespacedName(runtime.Status.Mounts)
	} else {
		// The dataset is not found and no status.mounts, in this case, the runtime is deleting, return nil, nil
		e.Log.Info("The dataset is deleted and no runtime.Status.Mounts, in this case, the runtime is deleting, return nil, nil")
		return nil, nil
	}
	if len(mountedNameSpacedNames) != 1 {
		return e.mountedRuntimeInfo, fmt.Errorf("ThinEngine with no profile name can only handle dataset only mounting one dataset")
	}
	namespacedName := mountedNameSpacedNames[0]

	mountedRuntimeInfo, err := base.GetRuntimeInfo(e.Client, namespacedName.Name, namespacedName.Namespace)
	if err != nil {
		return e.mountedRuntimeInfo, err
	}

	e.mountedRuntimeInfo = mountedRuntimeInfo

	return e.mountedRuntimeInfo, nil
}
