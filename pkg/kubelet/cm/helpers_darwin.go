//go:build darwin

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

package cm

import (
	"math"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
)

// These values mirror the Linux CPU accounting units used in CRI resource
// status. Darwin does not apply them to host cgroups, but the macOS CRI shim
// can report Linux-shaped CPU values for Kubernetes API compatibility.
const (
	MinShares = 2
	MaxShares = 262144

	SharesPerCPU  = 1024
	MilliCPUToCPU = 1000

	QuotaPeriod      = 100000
	MinQuotaPeriod   = 1000
	MinMilliCPULimit = 10
)

func MilliCPUToQuota(milliCPU, period int64) int64 {
	if milliCPU == 0 {
		return 0
	}
	if period <= 0 {
		period = QuotaPeriod
	}
	quota := (milliCPU * period) / MilliCPUToCPU
	if quota < MinQuotaPeriod {
		quota = MinQuotaPeriod
	}
	return quota
}

func MilliCPUToShares(milliCPU int64) uint64 {
	if milliCPU == 0 {
		return MinShares
	}
	shares := (milliCPU * SharesPerCPU) / MilliCPUToCPU
	if shares < MinShares {
		return MinShares
	}
	if shares > MaxShares {
		return MaxShares
	}
	return uint64(shares)
}

func SharesToMilliCPU(shares int64) int64 {
	if shares < MinShares {
		return 0
	}
	return int64(math.Ceil(float64(shares*MilliCPUToCPU) / float64(SharesPerCPU)))
}

func QuotaToMilliCPU(quota, period int64) int64 {
	if quota <= 0 || period <= 0 {
		return 0
	}
	return (quota * MilliCPUToCPU) / period
}

func ResourceConfigForPod(pod *v1.Pod, enforceCPULimit bool, cpuPeriod uint64, enforceMemoryQoS bool, memoryReservationPolicy kubeletconfig.MemoryReservationPolicy) *ResourceConfig {
	return nil
}

func GetCgroupSubsystems() (*CgroupSubsystems, error) {
	return nil, nil
}

func getCgroupProcs(dir string) ([]int, error) {
	return nil, nil
}

func GetPodCgroupNameSuffix(podUID types.UID) string {
	return ""
}

func NodeAllocatableRoot(cgroupRoot string, cgroupsPerQOS bool, cgroupDriver string) string {
	return ""
}

func GetKubeletContainer(logger klog.Logger, kubeletCgroups string) (string, error) {
	return "", nil
}

func CPURequestsFromConfig(podConfig *ResourceConfig) *resource.Quantity {
	if podConfig == nil || podConfig.CPUShares == nil || *podConfig.CPUShares == 0 {
		return nil
	}
	milliCPU := SharesToMilliCPU(int64(*podConfig.CPUShares))
	if milliCPU <= 0 {
		return nil
	}
	return resource.NewMilliQuantity(milliCPU, resource.DecimalSI)
}

func CPULimitsFromConfig(podConfig *ResourceConfig) *resource.Quantity {
	if podConfig == nil || podConfig.CPUQuota == nil || podConfig.CPUPeriod == nil || *podConfig.CPUPeriod == 0 {
		return nil
	}
	milliCPU := QuotaToMilliCPU(*podConfig.CPUQuota, int64(*podConfig.CPUPeriod))
	if milliCPU <= 0 {
		return nil
	}
	return resource.NewMilliQuantity(milliCPU, resource.DecimalSI)
}

func MemoryLimitsFromConfig(podConfig *ResourceConfig) *resource.Quantity {
	if podConfig == nil || podConfig.Memory == nil || *podConfig.Memory <= 0 {
		return nil
	}
	return resource.NewQuantity(*podConfig.Memory, resource.BinarySI)
}
