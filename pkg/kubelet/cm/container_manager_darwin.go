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
	"context"
	"fmt"
	"sync"

	"k8s.io/mount-utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	internalapi "k8s.io/cri-api/pkg/apis"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/cadvisor"
	"k8s.io/kubernetes/pkg/kubelet/config"
	"k8s.io/kubernetes/pkg/kubelet/status"
)

type darwinContainerManager struct {
	containerManagerStub
	cadvisorInterface cadvisor.Interface
	capacity          v1.ResourceList
	nodeConfig        NodeConfig
	sync.RWMutex
}

var _ ContainerManager = &darwinContainerManager{}

func (cm *darwinContainerManager) Start(
	ctx context.Context,
	node *v1.Node,
	activePods ActivePodsFunc,
	getNode GetNodeFunc,
	sourcesReady config.SourcesReady,
	podStatusProvider status.PodStatusProvider,
	runtimeService internalapi.RuntimeService,
	localStorageCapacityIsolation bool,
) error {
	if localStorageCapacityIsolation {
		rootfs, err := cm.cadvisorInterface.RootFsInfo()
		if err != nil {
			return fmt.Errorf("failed to get rootfs info: %v", err)
		}
		cm.Lock()
		for resourceName, resourceCapacity := range cadvisor.EphemeralStorageCapacityFromFsInfo(rootfs) {
			cm.capacity[resourceName] = resourceCapacity
		}
		cm.Unlock()
	}
	return cm.containerManagerStub.Start(ctx, node, activePods, getNode, sourcesReady, podStatusProvider, runtimeService, localStorageCapacityIsolation)
}

func (cm *darwinContainerManager) GetCapacity(localStorageCapacityIsolation bool) v1.ResourceList {
	if !localStorageCapacityIsolation {
		return v1.ResourceList{}
	}

	cm.RLock()
	capacity := make(v1.ResourceList, len(cm.capacity)+1)
	for resourceName, resourceCapacity := range cm.capacity {
		capacity[resourceName] = resourceCapacity.DeepCopy()
	}
	_, hasEphemeralStorage := capacity[v1.ResourceEphemeralStorage]
	cadvisorInterface := cm.cadvisorInterface
	cm.RUnlock()

	if hasEphemeralStorage || cadvisorInterface == nil {
		return capacity
	}
	rootfs, err := cadvisorInterface.RootFsInfo()
	if err != nil {
		klog.ErrorS(err, "Unable to get rootfs data from cAdvisor interface")
		return capacity
	}
	for resourceName, resourceCapacity := range cadvisor.EphemeralStorageCapacityFromFsInfo(rootfs) {
		capacity[resourceName] = resourceCapacity
	}
	return capacity
}

func (cm *darwinContainerManager) GetNodeConfig() NodeConfig {
	cm.RLock()
	defer cm.RUnlock()
	return cm.nodeConfig
}

func (cm *darwinContainerManager) GetNodeAllocatableReservation() v1.ResourceList {
	cm.RLock()
	defer cm.RUnlock()

	evictionReservation := hardEvictionReservation(cm.nodeConfig.HardEvictionThresholds, cm.capacity)
	result := make(v1.ResourceList)
	for resourceName := range cm.capacity {
		value := resource.NewQuantity(0, resource.DecimalSI)
		if cm.nodeConfig.SystemReserved != nil {
			value.Add(cm.nodeConfig.SystemReserved[resourceName])
		}
		if cm.nodeConfig.KubeReserved != nil {
			value.Add(cm.nodeConfig.KubeReserved[resourceName])
		}
		if evictionReservation != nil {
			value.Add(evictionReservation[resourceName])
		}
		if !value.IsZero() {
			result[resourceName] = *value
		}
	}
	return result
}

func NewContainerManager(_ context.Context, _ mount.Interface, cadvisorInterface cadvisor.Interface, nodeConfig NodeConfig, _ bool, _ record.EventRecorder, _ clientset.Interface) (ContainerManager, error) {
	machineInfo, err := cadvisorInterface.MachineInfo()
	if err != nil {
		return nil, err
	}
	return &darwinContainerManager{
		containerManagerStub: containerManagerStub{shouldResetExtendedResourceCapacity: false},
		cadvisorInterface:    cadvisorInterface,
		capacity:             cadvisor.CapacityFromMachineInfo(machineInfo),
		nodeConfig:           nodeConfig,
	}, nil
}
