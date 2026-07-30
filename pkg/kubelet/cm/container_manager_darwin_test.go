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
	"testing"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	cadvisortest "k8s.io/kubernetes/pkg/kubelet/cadvisor/testing"
	evictionapi "k8s.io/kubernetes/pkg/kubelet/eviction/api"
)

func TestDarwinContainerManagerStartSetsEphemeralStorageCapacity(t *testing.T) {
	const rootfsCapacity = 994662584320
	mockCadvisor := cadvisortest.NewMockInterface(t)
	mockCadvisor.EXPECT().RootFsInfo().Return(cadvisorapiv2.FsInfo{Capacity: rootfsCapacity}, nil)

	manager := &darwinContainerManager{
		containerManagerStub: containerManagerStub{shouldResetExtendedResourceCapacity: false},
		cadvisorInterface:    mockCadvisor,
		capacity: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("4"),
			v1.ResourceMemory: resource.MustParse("8Gi"),
		},
	}
	require.NoError(t, manager.Start(context.Background(), &v1.Node{}, nil, nil, nil, nil, nil, true))

	capacity := manager.GetCapacity(true)
	ephemeralStorage := capacity[v1.ResourceEphemeralStorage]
	require.Equal(t, int64(rootfsCapacity), ephemeralStorage.Value())
	require.Equal(t, int64(4), capacity.Cpu().Value())
	require.Equal(t, int64(8*1024*1024*1024), capacity.Memory().Value())
	require.Empty(t, manager.GetCapacity(false))
}

func TestDarwinContainerManagerGetCapacityFallsBackToRootFsInfo(t *testing.T) {
	const rootfsCapacity = 560869621760
	mockCadvisor := cadvisortest.NewMockInterface(t)
	mockCadvisor.EXPECT().RootFsInfo().Return(cadvisorapiv2.FsInfo{Capacity: rootfsCapacity}, nil)

	manager := &darwinContainerManager{
		containerManagerStub: containerManagerStub{shouldResetExtendedResourceCapacity: false},
		cadvisorInterface:    mockCadvisor,
		capacity: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("2"),
			v1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}

	capacity := manager.GetCapacity(true)
	ephemeralStorage := capacity[v1.ResourceEphemeralStorage]
	require.Equal(t, int64(rootfsCapacity), ephemeralStorage.Value())
	require.Equal(t, int64(2), capacity.Cpu().Value())
	require.Equal(t, int64(4*1024*1024*1024), capacity.Memory().Value())
}

func TestDarwinContainerManagerPreservesNodeConfigAndReservations(t *testing.T) {
	const memoryCapacity = uint64(8 * 1024 * 1024 * 1024)
	mockCadvisor := cadvisortest.NewMockInterface(t)
	mockCadvisor.EXPECT().MachineInfo().Return(&cadvisorapi.MachineInfo{
		NumCores:       4,
		MemoryCapacity: memoryCapacity,
	}, nil)

	evictionMemory := resource.MustParse("128Mi")
	nodeConfig := NodeConfig{NodeAllocatableConfig: NodeAllocatableConfig{
		SystemReserved: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("200m"),
			v1.ResourceMemory: resource.MustParse("512Mi"),
		},
		KubeReserved: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("300m"),
			v1.ResourceMemory: resource.MustParse("256Mi"),
		},
		HardEvictionThresholds: []evictionapi.Threshold{{
			Signal:   evictionapi.SignalMemoryAvailable,
			Operator: evictionapi.OpLessThan,
			Value:    evictionapi.ThresholdValue{Quantity: &evictionMemory},
		}},
	}}

	containerManager, err := NewContainerManager(context.Background(), nil, mockCadvisor, nodeConfig, false, nil, nil)
	require.NoError(t, err)
	manager := containerManager.(*darwinContainerManager)
	require.Equal(t, nodeConfig, manager.GetNodeConfig())

	reservation := manager.GetNodeAllocatableReservation()
	require.Equal(t, int64(500), reservation.Cpu().MilliValue())
	require.Equal(t, int64(896*1024*1024), reservation.Memory().Value())

	manager.RLock()
	require.Equal(t, int64(4), manager.capacity.Cpu().Value())
	require.Equal(t, int64(memoryCapacity), manager.capacity.Memory().Value())
	manager.RUnlock()
}
