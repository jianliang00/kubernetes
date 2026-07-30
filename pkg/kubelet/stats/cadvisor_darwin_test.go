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

package stats

import (
	"os"
	"runtime"
	"testing"

	cadvisorapiv2 "github.com/google/cadvisor/info/v2"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/cadvisor"
)

func TestDarwinRootContainerSummaryConversion(t *testing.T) {
	client, err := cadvisor.New(klog.Background(), nil, os.TempDir(), nil, false, false)
	if err != nil {
		t.Fatalf("cadvisor.New() error = %v", err)
	}

	stopCPULoad := startDarwinSummaryTestCPULoad()
	infos, err := client.ContainerInfoV2("/", cadvisorapiv2.RequestOptions{Count: 2})
	stopCPULoad()
	if err != nil {
		t.Fatalf("ContainerInfoV2() error = %v", err)
	}
	root := infos["/"]
	cpu, memory := cadvisorInfoToCPUandMemoryStats(&root)
	if cpu == nil || cpu.UsageNanoCores == nil || *cpu.UsageNanoCores == 0 {
		t.Fatalf("summary CPU usageNanoCores was not populated: %+v", cpu)
	}
	if memory == nil || memory.AvailableBytes == nil {
		t.Fatalf("summary memory availableBytes was not populated: %+v", memory)
	}
	if *memory.AvailableBytes > root.Spec.Memory.Limit {
		t.Fatalf("summary memory availableBytes underflowed: %d > limit %d", *memory.AvailableBytes, root.Spec.Memory.Limit)
	}

	swap := cadvisorInfoToSwapStats(&root)
	if swap == nil || swap.SwapUsageBytes == nil {
		t.Fatalf("summary swap usage was not populated: %+v", swap)
	}
	if swap.SwapAvailableBytes != nil {
		t.Fatalf("summary swap availableBytes = %d, want omitted for an unlimited host root", *swap.SwapAvailableBytes)
	}
}

func startDarwinSummaryTestCPULoad() func() {
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
