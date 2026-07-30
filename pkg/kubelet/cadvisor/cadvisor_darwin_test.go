//go:build darwin && cgo

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
	"errors"
	"math"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"k8s.io/klog/v2"
)

func TestDarwinRootContainerStats(t *testing.T) {
	cadvisor, err := New(klog.Background(), nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	stopCPULoad := startDarwinTestCPULoad()
	infos, err := cadvisor.ContainerInfoV2(darwinRootContainerName, cadvisorapiv2.RequestOptions{Count: 2})
	stopCPULoad()
	if err != nil {
		t.Fatalf("ContainerInfoV2(%q) error = %v", darwinRootContainerName, err)
	}
	root, ok := infos[darwinRootContainerName]
	if !ok {
		t.Fatalf("ContainerInfoV2(%q) did not return root container", darwinRootContainerName)
	}
	if !root.Spec.HasCpu || !root.Spec.HasMemory || !root.Spec.HasProcesses || !root.Spec.HasFilesystem {
		t.Fatalf("root spec does not advertise real host stats: %+v", root.Spec)
	}
	if len(root.Stats) != 2 {
		t.Fatalf("got %d stats, want 2", len(root.Stats))
	}
	machineInfo, err := cadvisor.MachineInfo()
	if err != nil {
		t.Fatalf("MachineInfo() error = %v", err)
	}
	if root.Spec.Memory.Limit != machineInfo.MemoryCapacity {
		t.Fatalf("root memory limit = %d, want machine capacity %d", root.Spec.Memory.Limit, machineInfo.MemoryCapacity)
	}
	if root.Spec.Memory.SwapLimit != math.MaxUint64 {
		t.Fatalf("root swap limit = %d, want unlimited", root.Spec.Memory.SwapLimit)
	}

	stats := root.Stats[1]
	if stats.Cpu == nil || stats.Cpu.Usage.Total == 0 || len(stats.Cpu.Usage.PerCpu) == 0 {
		t.Fatalf("CPU stats were not populated: %+v", stats.Cpu)
	}
	if stats.CpuInst == nil || stats.CpuInst.Usage.Total == 0 {
		t.Fatalf("instantaneous CPU stats were not populated: %+v", stats.CpuInst)
	}
	if stats.Memory == nil || stats.Memory.Usage == 0 || stats.Memory.WorkingSet == 0 {
		t.Fatalf("memory stats were not populated: %+v", stats.Memory)
	}
	if stats.Filesystem == nil || stats.Filesystem.TotalUsageBytes == nil || *stats.Filesystem.TotalUsageBytes == 0 {
		t.Fatalf("filesystem stats were not populated: %+v", stats.Filesystem)
	}

	legacyInfos, err := cadvisor.GetRequestedContainersInfo(darwinRootContainerName, cadvisorapiv2.RequestOptions{Count: 1})
	if err != nil {
		t.Fatalf("GetRequestedContainersInfo(%q) error = %v", darwinRootContainerName, err)
	}
	legacySpec := legacyInfos[darwinRootContainerName].Spec
	if legacySpec.Memory.Limit != machineInfo.MemoryCapacity || legacySpec.Memory.SwapLimit != math.MaxUint64 {
		t.Fatalf("legacy root memory spec = %+v, want limit %d and unlimited swap", legacySpec.Memory, machineInfo.MemoryCapacity)
	}
}

func TestDarwinContainerInfoV2ReusesCPUHistory(t *testing.T) {
	cadvisor, err := New(klog.Background(), nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	longMaxAge := time.Hour
	first, err := cadvisor.ContainerInfoV2(darwinRootContainerName, cadvisorapiv2.RequestOptions{Count: 2, MaxAge: &longMaxAge})
	if err != nil {
		t.Fatalf("first ContainerInfoV2() error = %v", err)
	}
	firstLatest := first[darwinRootContainerName].Stats[1]

	second, err := cadvisor.ContainerInfoV2(darwinRootContainerName, cadvisorapiv2.RequestOptions{Count: 2})
	if err != nil {
		t.Fatalf("second ContainerInfoV2() error = %v", err)
	}
	secondStats := second[darwinRootContainerName].Stats
	if len(secondStats) != 2 {
		t.Fatalf("second ContainerInfoV2() returned %d stats, want 2", len(secondStats))
	}
	if secondStats[1] != firstLatest {
		t.Fatal("second ContainerInfoV2() did not reuse the latest cached sample")
	}
	if secondStats[1].CpuInst == nil {
		t.Fatal("second ContainerInfoV2() latest sample has no instantaneous CPU stats")
	}

	forceRefresh := time.Duration(0)
	third, err := cadvisor.ContainerInfoV2(darwinRootContainerName, cadvisorapiv2.RequestOptions{Count: 2, MaxAge: &forceRefresh})
	if err != nil {
		t.Fatalf("third ContainerInfoV2() error = %v", err)
	}
	thirdStats := third[darwinRootContainerName].Stats
	if len(thirdStats) != 2 || thirdStats[0] != firstLatest || thirdStats[1] == firstLatest {
		t.Fatal("third ContainerInfoV2() did not append a new on-demand sample")
	}
	if thirdStats[1].CpuInst == nil {
		t.Fatal("on-demand sample has no instantaneous CPU stats")
	}
}

func TestDarwinContainerInfoV2CountZeroDoesNotRefresh(t *testing.T) {
	cadvisor := &cadvisorDarwin{}
	stats, err := cadvisor.requestedHostContainerStats(cadvisorapiv2.RequestOptions{Count: 0})
	if err != nil {
		t.Fatalf("requestedHostContainerStats() error = %v", err)
	}
	if len(stats) != 0 || len(cadvisor.statsHistory) != 0 {
		t.Fatalf("Count: 0 unexpectedly collected stats: returned=%+v history=%+v", stats, cadvisor.statsHistory)
	}
}

func TestDarwinContainerInfoV2ConcurrentSampling(t *testing.T) {
	cadvisor, err := New(klog.Background(), nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := cadvisor.ContainerInfoV2(darwinRootContainerName, cadvisorapiv2.RequestOptions{Count: 2}); err != nil {
		t.Fatalf("initial ContainerInfoV2() error = %v", err)
	}

	const callers = 8
	errCh := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			infos, err := cadvisor.ContainerInfoV2(darwinRootContainerName, cadvisorapiv2.RequestOptions{Count: 1})
			if err == nil {
				stats := infos[darwinRootContainerName].Stats
				if len(stats) != 1 || stats[0].CpuInst == nil {
					err = errors.New("latest sample has no instantaneous CPU stats")
				}
			}
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent ContainerInfoV2() error = %v", err)
		}
	}
}

func TestDarwinInstantCPUStats(t *testing.T) {
	previous := &cadvisorapiv2.ContainerStats{
		Timestamp: time.Unix(10, 0),
		Cpu: &cadvisorapi.CpuStats{Usage: cadvisorapi.CpuUsage{
			Total:  1_000_000_000,
			PerCpu: []uint64{400_000_000, 600_000_000},
			User:   700_000_000,
			System: 300_000_000,
		}},
	}
	current := &cadvisorapiv2.ContainerStats{
		Timestamp: time.Unix(12, 0),
		Cpu: &cadvisorapi.CpuStats{Usage: cadvisorapi.CpuUsage{
			Total:  4_000_000_000,
			PerCpu: []uint64{1_400_000_000, 2_600_000_000},
			User:   2_700_000_000,
			System: 1_300_000_000,
		}},
	}

	got, err := darwinInstantCPUStats(previous, current)
	if err != nil {
		t.Fatalf("darwinInstantCPUStats() error = %v", err)
	}
	if got.Usage.Total != 1_500_000_000 || got.Usage.User != 1_000_000_000 || got.Usage.System != 500_000_000 {
		t.Fatalf("darwinInstantCPUStats() usage = %+v", got.Usage)
	}
	wantPerCPU := []uint64{500_000_000, 1_000_000_000}
	for index := range wantPerCPU {
		if got.Usage.PerCpu[index] != wantPerCPU[index] {
			t.Fatalf("darwinInstantCPUStats() per-CPU usage = %v, want %v", got.Usage.PerCpu, wantPerCPU)
		}
	}
}

func TestDarwinCPUCounterWrapResetsBaseline(t *testing.T) {
	previousCPUInst := &cadvisorapiv2.CpuInstStats{
		Usage: cadvisorapiv2.CpuInstUsage{Total: 750_000_000},
	}
	previous := &cadvisorapiv2.ContainerStats{
		Timestamp: time.Unix(10, 0),
		Cpu: &cadvisorapi.CpuStats{Usage: cadvisorapi.CpuUsage{
			Total:  4_000_000_000,
			PerCpu: []uint64{4_000_000_000},
			User:   3_000_000_000,
			System: 1_000_000_000,
		}},
		CpuInst: previousCPUInst,
	}
	wrapped := &cadvisorapiv2.ContainerStats{
		Timestamp: time.Unix(11, 0),
		Cpu: &cadvisorapi.CpuStats{Usage: cadvisorapi.CpuUsage{
			Total:  100_000_000,
			PerCpu: []uint64{100_000_000},
			User:   75_000_000,
			System: 25_000_000,
		}},
	}

	cadvisor := &cadvisorDarwin{
		logger:       klog.Background(),
		statsHistory: []*cadvisorapiv2.ContainerStats{previous},
	}
	cadvisor.appendHostContainerStats(wrapped)
	if len(cadvisor.statsHistory) != 1 || cadvisor.statsHistory[0] != wrapped {
		t.Fatalf("wrapped counter did not reset the sample baseline: %+v", cadvisor.statsHistory)
	}
	if wrapped.CpuInst != previousCPUInst {
		t.Fatalf("wrapped counter rate = %+v, want last valid rate %+v", wrapped.CpuInst, previousCPUInst)
	}

	next := &cadvisorapiv2.ContainerStats{
		Timestamp: time.Unix(12, 0),
		Cpu: &cadvisorapi.CpuStats{Usage: cadvisorapi.CpuUsage{
			Total:  300_000_000,
			PerCpu: []uint64{300_000_000},
			User:   225_000_000,
			System: 75_000_000,
		}},
	}
	cadvisor.appendHostContainerStats(next)
	if len(cadvisor.statsHistory) != 2 || next.CpuInst == nil || next.CpuInst.Usage.Total != 200_000_000 {
		t.Fatalf("CPU sampling did not recover after counter wrap: history=%+v rate=%+v", cadvisor.statsHistory, next.CpuInst)
	}
}

func startDarwinTestCPULoad() func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				runtime.Gosched()
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func TestDarwinRejectsNonRootContainerStats(t *testing.T) {
	cadvisor, err := New(klog.Background(), nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = cadvisor.ContainerInfoV2("/kubepods/pod123/container", cadvisorapiv2.RequestOptions{Count: 1})
	if !errors.Is(err, errDarwinNonRootContainer) {
		t.Fatalf("ContainerInfoV2(non-root) error = %v, want %v", err, errDarwinNonRootContainer)
	}
}

func TestDarwinMachineInfoUsesPlatformUUID(t *testing.T) {
	cadvisor, err := New(klog.Background(), nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	machineInfo, err := cadvisor.MachineInfo()
	if err != nil {
		t.Fatalf("MachineInfo() error = %v", err)
	}
	if machineInfo.MachineID == "" || machineInfo.SystemUUID == "" {
		t.Fatalf("MachineInfo() returned empty platform identity: %+v", machineInfo)
	}
	if machineInfo.MachineID != machineInfo.SystemUUID {
		t.Fatalf("MachineInfo() identities differ: MachineID=%q SystemUUID=%q", machineInfo.MachineID, machineInfo.SystemUUID)
	}
}

func TestDarwinMachineIdentifiers(t *testing.T) {
	machineID, systemUUID := darwinMachineIdentifiers(" 5EE786B0-D581-573D-8FA4-6F5132A2FF6D ", "SERIAL")
	const wantUUID = "5ee786b0-d581-573d-8fa4-6f5132a2ff6d"
	if machineID != wantUUID || systemUUID != wantUUID {
		t.Fatalf("darwinMachineIdentifiers() = (%q, %q), want (%q, %q)", machineID, systemUUID, wantUUID, wantUUID)
	}

	machineID, systemUUID = darwinMachineIdentifiers("", " F9PX1F4KX4 ")
	if machineID != "f9px1f4kx4" || systemUUID != "" {
		t.Fatalf("darwinMachineIdentifiers() fallback = (%q, %q), want serial-only machine ID", machineID, systemUUID)
	}
}
