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
	"testing"

	"github.com/golang/mock/gomock"
	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	cadvisortest "k8s.io/kubernetes/pkg/kubelet/cadvisor/testing"
)

func TestDarwinContainerManagerStartSetsEphemeralStorageCapacity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const rootfsCapacity = 994662584320
	mockCadvisor := cadvisortest.NewMockInterface(ctrl)
	mockCadvisor.EXPECT().RootFsInfo().Return(cadvisorapiv2.FsInfo{Capacity: rootfsCapacity}, nil)

	manager := &darwinContainerManager{
		containerManagerStub: containerManagerStub{shouldResetExtendedResourceCapacity: false},
		cadvisorInterface:    mockCadvisor,
		capacity:             v1.ResourceList{},
	}
	require.NoError(t, manager.Start(&v1.Node{}, nil, nil, nil, nil, true))

	capacity := manager.GetCapacity(true)
	ephemeralStorage := capacity[v1.ResourceEphemeralStorage]
	require.Equal(t, int64(rootfsCapacity), ephemeralStorage.Value())
	require.Empty(t, manager.GetCapacity(false))
}

func TestDarwinContainerManagerGetCapacityFallsBackToRootFsInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const rootfsCapacity = 560869621760
	mockCadvisor := cadvisortest.NewMockInterface(ctrl)
	mockCadvisor.EXPECT().RootFsInfo().Return(cadvisorapiv2.FsInfo{Capacity: rootfsCapacity}, nil)

	manager := &darwinContainerManager{
		containerManagerStub: containerManagerStub{shouldResetExtendedResourceCapacity: false},
		cadvisorInterface:    mockCadvisor,
		capacity:             v1.ResourceList{},
	}

	capacity := manager.GetCapacity(true)
	ephemeralStorage := capacity[v1.ResourceEphemeralStorage]
	require.Equal(t, int64(rootfsCapacity), ephemeralStorage.Value())
}
