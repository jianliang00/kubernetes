//go:build darwin
// +build darwin

/*
Copyright 2026 The Kubernetes Authors.

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

package kuberuntime

import (
	"math"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

const (
	milliCPUToCPU = 1000
)

// sharesToMilliCPU converts cpu shares to milli-CPU for CRI status reporting.
func sharesToMilliCPU(shares int64) int64 {
	milliCPU := int64(0)
	if shares >= int64(cm.MinShares) {
		milliCPU = int64(math.Ceil(float64(shares*milliCPUToCPU) / float64(cm.SharesPerCPU)))
	}
	return milliCPU
}

// quotaToMilliCPU converts CFS quota and period values to milli-CPU.
func quotaToMilliCPU(quota int64, period int64) int64 {
	if quota == -1 {
		return int64(0)
	}
	return (quota * milliCPUToCPU) / period
}

// applyPlatformSpecificContainerConfig applies the macOS CRI extension contract.
func (m *kubeGenericRuntimeManager) applyPlatformSpecificContainerConfig(config *runtimeapi.ContainerConfig, container *v1.Container, pod *v1.Pod, uid *int64, username string, nsTarget *kubecontainer.ContainerID) error {
	if config.Annotations == nil {
		config.Annotations = map[string]string{}
	}
	config.Annotations[darwinCRIPlatformAnnotation] = darwinCRIPlatform
	config.Annotations[darwinCRIContainerConfigVersionAnnotation] = darwinCRIConfigVersion
	return nil
}

// generateContainerResources returns an explicit empty resource update for macOS.
//
// CRI v1.27 has only Linux and Windows resource fields. The macOS CRI shim
// treats UpdateContainerResources as a deterministic no-op, so kubelet must send
// a non-nil but platform-empty resource object instead of failing locally.
func (m *kubeGenericRuntimeManager) generateContainerResources(pod *v1.Pod, container *v1.Container) *runtimeapi.ContainerResources {
	return &runtimeapi.ContainerResources{}
}

func toKubeContainerResources(statusResources *runtimeapi.ContainerResources) *kubecontainer.ContainerResources {
	var cStatusResources *kubecontainer.ContainerResources
	runtimeStatusResources := statusResources.GetLinux()
	if runtimeStatusResources != nil {
		var cpuLimit, cpuRequest, memLimit *resource.Quantity
		if runtimeStatusResources.CpuPeriod > 0 {
			milliCPU := quotaToMilliCPU(runtimeStatusResources.CpuQuota, runtimeStatusResources.CpuPeriod)
			cpuLimit = resource.NewMilliQuantity(milliCPU, resource.DecimalSI)
		}
		if runtimeStatusResources.CpuShares > 0 {
			milliCPU := sharesToMilliCPU(runtimeStatusResources.CpuShares)
			cpuRequest = resource.NewMilliQuantity(milliCPU, resource.DecimalSI)
		}
		if runtimeStatusResources.MemoryLimitInBytes > 0 {
			memLimit = resource.NewQuantity(runtimeStatusResources.MemoryLimitInBytes, resource.BinarySI)
		}
		if cpuLimit != nil || cpuRequest != nil || memLimit != nil {
			cStatusResources = &kubecontainer.ContainerResources{
				CPULimit:      cpuLimit,
				CPURequest:    cpuRequest,
				MemoryLimit:   memLimit,
				MemoryRequest: nil,
			}
		}
	}
	return cStatusResources
}
