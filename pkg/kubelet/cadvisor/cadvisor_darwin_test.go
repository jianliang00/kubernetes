//go:build darwin && cgo
// +build darwin,cgo

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
	"os"
	"strings"
	"testing"

	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
)

func TestDarwinRootContainerStats(t *testing.T) {
	cadvisor, err := New(nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	infos, err := cadvisor.ContainerInfoV2(darwinRootContainerName, cadvisorapiv2.RequestOptions{Count: 1})
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
	if len(root.Stats) != 1 {
		t.Fatalf("got %d stats, want 1", len(root.Stats))
	}

	stats := root.Stats[0]
	if stats.Cpu == nil || stats.Cpu.Usage.Total == 0 || len(stats.Cpu.Usage.PerCpu) == 0 {
		t.Fatalf("CPU stats were not populated: %+v", stats.Cpu)
	}
	if stats.Memory == nil || stats.Memory.Usage == 0 || stats.Memory.WorkingSet == 0 {
		t.Fatalf("memory stats were not populated: %+v", stats.Memory)
	}
	if stats.Filesystem == nil || stats.Filesystem.TotalUsageBytes == nil || *stats.Filesystem.TotalUsageBytes == 0 {
		t.Fatalf("filesystem stats were not populated: %+v", stats.Filesystem)
	}
}

func TestDarwinRejectsNonRootContainerStats(t *testing.T) {
	cadvisor, err := New(nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = cadvisor.ContainerInfoV2("/kubepods/pod123/container", cadvisorapiv2.RequestOptions{Count: 1})
	if !errors.Is(err, errDarwinNonRootContainer) {
		t.Fatalf("ContainerInfoV2(non-root) error = %v, want %v", err, errDarwinNonRootContainer)
	}
}

func TestDarwinMachineInfoUsesPlatformUUID(t *testing.T) {
	cadvisor, err := New(nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	info, err := cadvisor.MachineInfo()
	if err != nil {
		t.Fatalf("MachineInfo() error = %v", err)
	}
	want := strings.ToLower(strings.TrimSpace(darwinPlatformUUID()))
	if want == "" {
		t.Fatal("IOPlatformUUID was unavailable on the test host")
	}
	if info.MachineID != want || info.SystemUUID != want {
		t.Fatalf("MachineInfo() identities = (%q, %q), want platform UUID %q", info.MachineID, info.SystemUUID, want)
	}
}

func TestDarwinMachineIdentifiers(t *testing.T) {
	for _, test := range []struct {
		name       string
		uuid       string
		serial     string
		machineID  string
		systemUUID string
	}{
		{
			name:       "platform UUID takes precedence over serial",
			uuid:       " 5EE786B0-D581-573D-8FA4-6F5132A2FF6D ",
			serial:     "SERIAL",
			machineID:  "5ee786b0-d581-573d-8fa4-6f5132a2ff6d",
			systemUUID: "5ee786b0-d581-573d-8fa4-6f5132a2ff6d",
		},
		{name: "serial is only a machine ID fallback", serial: " F9PX1F4KX4 ", machineID: "f9px1f4kx4"},
		{name: "unavailable identities remain empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			machineID, systemUUID := darwinMachineIdentifiers(test.uuid, test.serial)
			if machineID != test.machineID || systemUUID != test.systemUUID {
				t.Fatalf("darwinMachineIdentifiers() = (%q, %q), want (%q, %q)", machineID, systemUUID, test.machineID, test.systemUUID)
			}
		})
	}
}
