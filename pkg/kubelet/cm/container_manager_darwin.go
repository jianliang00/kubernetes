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

package cm

import (
	"fmt"

	"k8s.io/mount-utils"

	v1 "k8s.io/api/core/v1"
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
}

var _ ContainerManager = &darwinContainerManager{}

func (cm *darwinContainerManager) Start(
	node *v1.Node,
	activePods ActivePodsFunc,
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
		cm.capacity = cadvisor.EphemeralStorageCapacityFromFsInfo(rootfs)
	}
	return cm.containerManagerStub.Start(node, activePods, sourcesReady, podStatusProvider, runtimeService, localStorageCapacityIsolation)
}

func (cm *darwinContainerManager) GetCapacity(localStorageCapacityIsolation bool) v1.ResourceList {
	if !localStorageCapacityIsolation {
		return v1.ResourceList{}
	}
	if _, ok := cm.capacity[v1.ResourceEphemeralStorage]; ok {
		return cm.capacity
	}
	if cm.cadvisorInterface == nil {
		return cm.capacity
	}
	rootfs, err := cm.cadvisorInterface.RootFsInfo()
	if err != nil {
		klog.ErrorS(err, "Unable to get rootfs data from cAdvisor interface")
		return cm.capacity
	}
	return cadvisor.EphemeralStorageCapacityFromFsInfo(rootfs)
}

func NewContainerManager(_ mount.Interface, cadvisorInterface cadvisor.Interface, _ NodeConfig, failSwapOn bool, recorder record.EventRecorder, kubeClient clientset.Interface) (ContainerManager, error) {
	return &darwinContainerManager{
		containerManagerStub: containerManagerStub{shouldResetExtendedResourceCapacity: false},
		cadvisorInterface:    cadvisorInterface,
		capacity:             v1.ResourceList{},
	}, nil
}
