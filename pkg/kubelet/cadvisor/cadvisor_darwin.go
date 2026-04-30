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

package cadvisor

/*
#cgo CFLAGS: -Wno-deprecated-declarations
#include <mach/mach.h>
#include <mach/mach_host.h>
#include <mach/processor_info.h>
#include <mach/vm_statistics.h>
#include <stdlib.h>
#include <unistd.h>

typedef struct {
	uint64_t user;
	uint64_t nice;
	uint64_t system;
	uint64_t idle;
} darwin_cpu_load_t;

static kern_return_t darwin_get_cpu_load(darwin_cpu_load_t **loads, natural_t *cpu_count) {
	processor_info_array_t cpu_info;
	mach_msg_type_number_t cpu_info_count;
	natural_t count;
	kern_return_t kr = host_processor_info(mach_host_self(), PROCESSOR_CPU_LOAD_INFO, &count, &cpu_info, &cpu_info_count);
	if (kr != KERN_SUCCESS) {
		return kr;
	}

	darwin_cpu_load_t *out = calloc(count, sizeof(darwin_cpu_load_t));
	if (out == NULL) {
		vm_deallocate(mach_task_self(), (vm_address_t)cpu_info, cpu_info_count * sizeof(integer_t));
		return KERN_RESOURCE_SHORTAGE;
	}

	for (natural_t i = 0; i < count; i++) {
		integer_t *cpu = cpu_info + (CPU_STATE_MAX * i);
		out[i].user = (uint64_t)cpu[CPU_STATE_USER];
		out[i].nice = (uint64_t)cpu[CPU_STATE_NICE];
		out[i].system = (uint64_t)cpu[CPU_STATE_SYSTEM];
		out[i].idle = (uint64_t)cpu[CPU_STATE_IDLE];
	}

	vm_deallocate(mach_task_self(), (vm_address_t)cpu_info, cpu_info_count * sizeof(integer_t));
	*loads = out;
	*cpu_count = count;
	return KERN_SUCCESS;
}

static void darwin_free_cpu_load(darwin_cpu_load_t *loads) {
	free(loads);
}

typedef struct {
	uint64_t free_count;
	uint64_t active_count;
	uint64_t inactive_count;
	uint64_t wire_count;
	uint64_t purgeable_count;
	uint64_t speculative_count;
	uint64_t compressor_page_count;
	uint64_t throttled_count;
	uint64_t external_page_count;
	uint64_t internal_page_count;
	uint64_t swapped_count;
} darwin_vm_stats_t;

static kern_return_t darwin_get_vm_stats(darwin_vm_stats_t *out) {
	vm_statistics64_data_t vm;
	mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
	kern_return_t kr = host_statistics64(mach_host_self(), HOST_VM_INFO64, (host_info64_t)&vm, &count);
	if (kr != KERN_SUCCESS) {
		return kr;
	}

	out->free_count = vm.free_count;
	out->active_count = vm.active_count;
	out->inactive_count = vm.inactive_count;
	out->wire_count = vm.wire_count;
	out->purgeable_count = vm.purgeable_count;
	out->speculative_count = vm.speculative_count;
	out->compressor_page_count = vm.compressor_page_count;
	out->throttled_count = vm.throttled_count;
	out->external_page_count = vm.external_page_count;
	out->internal_page_count = vm.internal_page_count;
	out->swapped_count = vm.swapped_count;
	return KERN_SUCCESS;
}

static uint64_t darwin_page_size(void) {
	return (uint64_t)getpagesize();
}
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/cadvisor/events"
	cadvisorapi "github.com/google/cadvisor/info/v1"
	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"golang.org/x/sys/unix"
)

type cadvisorDarwin struct {
	rootPath    string
	machineInfo *cadvisorapi.MachineInfo
	bootTime    time.Time
}

var _ Interface = new(cadvisorDarwin)

var errDarwinNonRootContainer = errors.New("darwin cAdvisor only provides host root container stats; pod and workload stats must come from CRI")

const darwinRootContainerName = "/"

// New creates a cAdvisor Interface backed by real Darwin host statistics.
func New(imageFsInfoProvider ImageFsInfoProvider, rootPath string, cgroupsRoots []string, usingLegacyStats, localStorageCapacityIsolation bool) (Interface, error) {
	bootTime := darwinBootTime()
	return &cadvisorDarwin{
		rootPath:    rootPath,
		machineInfo: darwinMachineInfo(bootTime),
		bootTime:    bootTime,
	}, nil
}

func (c *cadvisorDarwin) Start() error {
	return nil
}

func (c *cadvisorDarwin) DockerContainer(name string, req *cadvisorapi.ContainerInfoRequest) (cadvisorapi.ContainerInfo, error) {
	info, err := c.ContainerInfo(name, req)
	if err != nil {
		return cadvisorapi.ContainerInfo{}, err
	}
	return *info, nil
}

func (c *cadvisorDarwin) ContainerInfo(name string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error) {
	if !isDarwinRootContainer(name) {
		return nil, fmt.Errorf("%w: %q", errDarwinNonRootContainer, name)
	}
	sample, err := c.hostContainerStats()
	if err != nil {
		return nil, err
	}
	return c.rootContainerInfoV1(sample), nil
}

func (c *cadvisorDarwin) ContainerInfoV2(name string, options cadvisorapiv2.RequestOptions) (map[string]cadvisorapiv2.ContainerInfo, error) {
	if !isDarwinRootContainer(name) {
		return nil, fmt.Errorf("%w: %q", errDarwinNonRootContainer, name)
	}
	sample, err := c.hostContainerStats()
	if err != nil {
		return nil, err
	}
	return map[string]cadvisorapiv2.ContainerInfo{
		darwinRootContainerName: {
			Spec:  c.rootContainerSpecV2(),
			Stats: []*cadvisorapiv2.ContainerStats{sample},
		},
	}, nil
}

func (c *cadvisorDarwin) GetRequestedContainersInfo(containerName string, options cadvisorapiv2.RequestOptions) (map[string]*cadvisorapi.ContainerInfo, error) {
	if !isDarwinRootContainer(containerName) {
		return nil, fmt.Errorf("%w: %q", errDarwinNonRootContainer, containerName)
	}
	sample, err := c.hostContainerStats()
	if err != nil {
		return nil, err
	}
	return map[string]*cadvisorapi.ContainerInfo{
		darwinRootContainerName: c.rootContainerInfoV1(sample),
	}, nil
}

func (c *cadvisorDarwin) SubcontainerInfo(name string, req *cadvisorapi.ContainerInfoRequest) (map[string]*cadvisorapi.ContainerInfo, error) {
	if !isDarwinRootContainer(name) {
		return nil, fmt.Errorf("%w: %q", errDarwinNonRootContainer, name)
	}
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

func (c *cadvisorDarwin) ImagesFsInfo() (cadvisorapiv2.FsInfo, error) {
	return c.RootFsInfo()
}

func (c *cadvisorDarwin) RootFsInfo() (cadvisorapiv2.FsInfo, error) {
	return c.GetDirFsInfo(c.rootPath)
}

func (c *cadvisorDarwin) WatchEvents(request *events.Request) (*events.EventChannel, error) {
	return &events.EventChannel{}, nil
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
		Device:     darwinByteSliceToString(stat.Mntfromname[:]),
		Mountpoint: path,
		Capacity:   capacity,
		Available:  available,
		Usage:      usage,
		Inodes:     &inodes,
		InodesFree: &inodesFree,
	}, nil
}

func (c *cadvisorDarwin) hostContainerStats() (*cadvisorapiv2.ContainerStats, error) {
	cpu, err := darwinCPUStats()
	if err != nil {
		return nil, err
	}
	memory, err := darwinMemoryStats(c.machineInfo.MemoryCapacity)
	if err != nil {
		return nil, err
	}
	rootFs, err := c.RootFsInfo()
	if err != nil {
		return nil, err
	}
	processes := darwinProcessStats()
	return &cadvisorapiv2.ContainerStats{
		Timestamp: time.Now(),
		Cpu:       cpu,
		Memory:    memory,
		Filesystem: &cadvisorapiv2.FilesystemStats{
			TotalUsageBytes: &rootFs.Usage,
			BaseUsageBytes:  &rootFs.Usage,
			InodeUsage:      darwinInodesUsed(rootFs),
		},
		Processes: processes,
	}, nil
}

func (c *cadvisorDarwin) rootContainerInfoV1(sample *cadvisorapiv2.ContainerStats) *cadvisorapi.ContainerInfo {
	fsInfo, _ := c.RootFsInfo()
	stats := &cadvisorapi.ContainerStats{
		Timestamp: sample.Timestamp,
	}
	if sample.Cpu != nil {
		stats.Cpu = *sample.Cpu
	}
	if sample.Memory != nil {
		stats.Memory = *sample.Memory
	}
	if sample.Processes != nil {
		stats.Processes = *sample.Processes
	}
	if sample.Filesystem != nil {
		fs := cadvisorapi.FsStats{
			Device:     fsInfo.Device,
			Limit:      fsInfo.Capacity,
			Usage:      fsInfo.Usage,
			BaseUsage:  fsInfo.Usage,
			Available:  fsInfo.Available,
			HasInodes:  fsInfo.Inodes != nil && fsInfo.InodesFree != nil,
			Inodes:     darwinUint64PointerValue(fsInfo.Inodes),
			InodesFree: darwinUint64PointerValue(fsInfo.InodesFree),
		}
		stats.Filesystem = []cadvisorapi.FsStats{fs}
	}
	return &cadvisorapi.ContainerInfo{
		ContainerReference: cadvisorapi.ContainerReference{
			Name: darwinRootContainerName,
		},
		Spec:  c.rootContainerSpecV1(),
		Stats: []*cadvisorapi.ContainerStats{stats},
	}
}

func (c *cadvisorDarwin) rootContainerSpecV1() cadvisorapi.ContainerSpec {
	return cadvisorapi.ContainerSpec{
		CreationTime:  c.bootTime,
		HasCpu:        true,
		HasMemory:     true,
		HasProcesses:  true,
		HasFilesystem: true,
	}
}

func (c *cadvisorDarwin) rootContainerSpecV2() cadvisorapiv2.ContainerSpec {
	return cadvisorapiv2.ContainerSpec{
		CreationTime:  c.bootTime,
		HasCpu:        true,
		HasMemory:     true,
		HasProcesses:  true,
		HasFilesystem: true,
	}
}

func darwinCPUStats() (*cadvisorapi.CpuStats, error) {
	var loads *C.darwin_cpu_load_t
	var cpuCount C.natural_t
	if kr := C.darwin_get_cpu_load(&loads, &cpuCount); kr != C.KERN_SUCCESS {
		return nil, fmt.Errorf("failed to read Darwin CPU load from host_processor_info: mach error %d", int(kr))
	}
	defer C.darwin_free_cpu_load(loads)

	hz := int64(100)
	if clockInfo, err := unix.SysctlClockinfo("kern.clockrate"); err == nil && clockInfo.Hz > 0 {
		hz = int64(clockInfo.Hz)
	}
	tickNanos := uint64(time.Second / time.Duration(hz))
	cpuLoads := unsafe.Slice(loads, int(cpuCount))
	perCPU := make([]uint64, 0, len(cpuLoads))
	var userTicks, systemTicks, totalTicks uint64
	for _, cpu := range cpuLoads {
		cpuUserTicks := uint64(cpu.user + cpu.nice)
		cpuSystemTicks := uint64(cpu.system)
		cpuTotalTicks := cpuUserTicks + cpuSystemTicks
		userTicks += cpuUserTicks
		systemTicks += cpuSystemTicks
		totalTicks += cpuTotalTicks
		perCPU = append(perCPU, cpuTotalTicks*tickNanos)
	}
	return &cadvisorapi.CpuStats{
		Usage: cadvisorapi.CpuUsage{
			Total:  totalTicks * tickNanos,
			PerCpu: perCPU,
			User:   userTicks * tickNanos,
			System: systemTicks * tickNanos,
		},
	}, nil
}

func darwinMemoryStats(memoryCapacity uint64) (*cadvisorapi.MemoryStats, error) {
	var vm C.darwin_vm_stats_t
	if kr := C.darwin_get_vm_stats(&vm); kr != C.KERN_SUCCESS {
		return nil, fmt.Errorf("failed to read Darwin VM stats from host_statistics64: mach error %d", int(kr))
	}
	pageSize := uint64(C.darwin_page_size())
	freeBytes := uint64(vm.free_count) * pageSize
	activeBytes := uint64(vm.active_count) * pageSize
	inactiveBytes := uint64(vm.inactive_count) * pageSize
	wireBytes := uint64(vm.wire_count) * pageSize
	purgeableBytes := uint64(vm.purgeable_count) * pageSize
	compressorBytes := uint64(vm.compressor_page_count) * pageSize
	throttledBytes := uint64(vm.throttled_count) * pageSize
	externalBytes := uint64(vm.external_page_count) * pageSize
	internalBytes := uint64(vm.internal_page_count) * pageSize
	swappedBytes := uint64(vm.swapped_count) * pageSize

	usage := darwinSaturatingSub(memoryCapacity, freeBytes)
	workingSet := darwinMin(memoryCapacity, activeBytes+wireBytes+compressorBytes+throttledBytes)
	cache := inactiveBytes + purgeableBytes + externalBytes
	return &cadvisorapi.MemoryStats{
		Usage:      usage,
		MaxUsage:   usage,
		Cache:      cache,
		RSS:        internalBytes,
		Swap:       swappedBytes,
		WorkingSet: workingSet,
	}, nil
}

func darwinProcessStats() *cadvisorapi.ProcessStats {
	processes, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil
	}
	return &cadvisorapi.ProcessStats{
		ProcessCount: uint64(len(processes)),
	}
}

func darwinMachineInfo(bootTime time.Time) *cadvisorapi.MachineInfo {
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
		CPUVendorID:      darwinSysctlString("machdep.cpu.brand_string", darwinSysctlString("machdep.cpu.vendor", "apple")),
		NumCores:         numCPU,
		NumPhysicalCores: numCPU,
		NumSockets:       1,
		MemoryCapacity:   memoryCapacity,
		MachineID:        darwinSysctlString("hw.model", "darwin"),
		SystemUUID:       darwinSysctlString("kern.hostuuid", darwinSysctlString("hw.model", "darwin")),
		BootID:           fmt.Sprintf("%d", bootTime.UnixNano()),
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

func darwinBootTime() time.Time {
	if boottime, err := unix.SysctlTimeval("kern.boottime"); err == nil {
		return time.Unix(boottime.Sec, int64(boottime.Usec)*int64(time.Microsecond))
	}
	return time.Now()
}

func isDarwinRootContainer(name string) bool {
	return name == "" || name == darwinRootContainerName
}

func darwinInodesUsed(fsInfo cadvisorapiv2.FsInfo) *uint64 {
	if fsInfo.Inodes == nil || fsInfo.InodesFree == nil {
		return nil
	}
	used := darwinSaturatingSub(*fsInfo.Inodes, *fsInfo.InodesFree)
	return &used
}

func darwinUint64PointerValue(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func darwinSaturatingSub(value, subtract uint64) uint64 {
	if subtract > value {
		return 0
	}
	return value - subtract
}

func darwinMin(lhs, rhs uint64) uint64 {
	if lhs < rhs {
		return lhs
	}
	return rhs
}

func darwinByteSliceToString(data []byte) string {
	for index, value := range data {
		if value == 0 {
			return string(data[:index])
		}
	}
	return string(data)
}

func darwinSysctlString(name, fallback string) string {
	value, err := syscall.Sysctl(name)
	if err == nil && value != "" {
		return value
	}
	return fallback
}
