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
