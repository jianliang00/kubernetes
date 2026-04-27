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

package cadvisor

import (
	"context"
	"runtime"
	"syscall"
	"time"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

type cadvisorDarwin struct {
	rootPath    string
	machineInfo *cadvisorapi.MachineInfo
}

var _ Interface = new(cadvisorDarwin)

// New creates a minimal cAdvisor Interface for darwin kubelet experiments.
func New(_ klog.Logger, imageFsInfoProvider ImageFsInfoProvider, rootPath string, cgroupsRoots []string, usingLegacyStats, localStorageCapacityIsolation bool) (Interface, error) {
	return &cadvisorDarwin{
		rootPath:    rootPath,
		machineInfo: darwinMachineInfo(),
	}, nil
}

func (c *cadvisorDarwin) Start() error {
	return nil
}

func (c *cadvisorDarwin) ContainerInfoV2(name string, options cadvisorapiv2.RequestOptions) (map[string]cadvisorapiv2.ContainerInfo, error) {
	rootFs, err := c.RootFsInfo()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return map[string]cadvisorapiv2.ContainerInfo{
		"/": {
			Spec: cadvisorapiv2.ContainerSpec{
				CreationTime:  now,
				HasCpu:        true,
				HasMemory:     true,
				HasFilesystem: true,
			},
			Stats: []*cadvisorapiv2.ContainerStats{
				{
					Timestamp: now,
					Filesystem: &cadvisorapiv2.FilesystemStats{
						TotalUsageBytes: &rootFs.Usage,
						BaseUsageBytes:  &rootFs.Usage,
					},
				},
			},
		},
	}, nil
}

func (c *cadvisorDarwin) GetRequestedContainersInfo(containerName string, options cadvisorapiv2.RequestOptions) (map[string]*cadvisorapi.ContainerInfo, error) {
	return map[string]*cadvisorapi.ContainerInfo{}, nil
}

func (c *cadvisorDarwin) MachineInfo() (*cadvisorapi.MachineInfo, error) {
	return c.machineInfo.Clone(), nil
}

func (c *cadvisorDarwin) VersionInfo() (*cadvisorapi.VersionInfo, error) {
	return &cadvisorapi.VersionInfo{
		KernelVersion:      darwinSysctlString("kern.osrelease", runtime.GOOS),
		ContainerOsVersion: darwinSysctlString("kern.version", runtime.GOOS),
		CadvisorVersion:    "darwin-minimal",
	}, nil
}

func (c *cadvisorDarwin) ImagesFsInfo(context.Context) (cadvisorapiv2.FsInfo, error) {
	return c.RootFsInfo()
}

func (c *cadvisorDarwin) RootFsInfo() (cadvisorapiv2.FsInfo, error) {
	return c.GetDirFsInfo(c.rootPath)
}

func (c *cadvisorDarwin) ContainerFsInfo(context.Context) (cadvisorapiv2.FsInfo, error) {
	return c.RootFsInfo()
}

func (c *cadvisorDarwin) GetDirFsInfo(path string) (cadvisorapiv2.FsInfo, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return cadvisorapiv2.FsInfo{}, err
	}
	blockSize := uint64(stat.Bsize)
	capacity := stat.Blocks * blockSize
	available := stat.Bavail * blockSize
	usage := (stat.Blocks - stat.Bfree) * blockSize
	inodes := stat.Files
	inodesFree := stat.Ffree
	return cadvisorapiv2.FsInfo{
		Timestamp:  time.Now(),
		Device:     "darwin-rootfs",
		Mountpoint: path,
		Capacity:   capacity,
		Available:  available,
		Usage:      usage,
		Inodes:     &inodes,
		InodesFree: &inodesFree,
	}, nil
}

func IsPsiEnabled(_ klog.Logger) bool {
	return false
}

func darwinMachineInfo() *cadvisorapi.MachineInfo {
	numCPU := runtime.NumCPU()
	if numCPU < 1 {
		numCPU = 1
	}
	memoryCapacity := darwinSysctlUint64("hw.memsize")
	topology := []cadvisorapi.Node{
		{
			Id:     0,
			Memory: memoryCapacity,
			Cores:  darwinCores(numCPU),
		},
	}
	return &cadvisorapi.MachineInfo{
		Timestamp:        time.Now(),
		CPUVendorID:      darwinSysctlString("machdep.cpu.vendor", "apple"),
		NumCores:         numCPU,
		NumPhysicalCores: numCPU,
		NumSockets:       1,
		MemoryCapacity:   memoryCapacity,
		MachineID:        "darwin",
		SystemUUID:       "darwin",
		BootID:           darwinSysctlString("kern.boottime", "darwin"),
		Topology:         topology,
		CloudProvider:    cadvisorapi.UnknownProvider,
		InstanceType:     cadvisorapi.UnknownInstance,
		InstanceID:       cadvisorapi.UnNamedInstance,
	}
}

func darwinCores(numCPU int) []cadvisorapi.Core {
	cores := make([]cadvisorapi.Core, 0, numCPU)
	for cpuID := 0; cpuID < numCPU; cpuID++ {
		cores = append(cores, cadvisorapi.Core{
			Id:       cpuID,
			Threads:  []int{cpuID},
			SocketID: 0,
		})
	}
	return cores
}

func darwinSysctlUint64(name string) uint64 {
	value, err := unix.SysctlUint64(name)
	if err == nil && value > 0 {
		return value
	}
	return 1
}

func darwinSysctlString(name, fallback string) string {
	value, err := syscall.Sysctl(name)
	if err == nil && value != "" {
		return value
	}
	return fallback
}
