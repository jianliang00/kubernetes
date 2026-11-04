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

package kuberuntime

import (
	"reflect"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestDarwinSandboxResources(t *testing.T) {
	container := func(name, cpuRequest, cpuLimit, memoryRequest, memoryLimit string) v1.Container {
		resources := v1.ResourceRequirements{Requests: v1.ResourceList{}, Limits: v1.ResourceList{}}
		for name, value := range map[v1.ResourceName]string{v1.ResourceCPU: cpuRequest, v1.ResourceMemory: memoryRequest} {
			if value != "" {
				resources.Requests[name] = resource.MustParse(value)
			}
		}
		for name, value := range map[v1.ResourceName]string{v1.ResourceCPU: cpuLimit, v1.ResourceMemory: memoryLimit} {
			if value != "" {
				resources.Limits[name] = resource.MustParse(value)
			}
		}
		return v1.Container{Name: name, Resources: resources}
	}
	tests := []struct {
		name                   string
		containers             []v1.Container
		init                   []v1.Container
		cpu, memory, errorText string
	}{
		{name: "limits override requests", containers: []v1.Container{container("app", "4", "8", "8Gi", "16384M")}, cpu: "8000", memory: "16384000000"},
		{name: "requests only", containers: []v1.Container{container("app", "1500m", "", "16Gi", "")}, cpu: "1500", memory: "17179869184"},
		{name: "mixed limits and requests", containers: []v1.Container{container("a", "1", "2", "2Gi", "4Gi"), container("b", "500m", "", "1Gi", "")}, cpu: "2500", memory: "5368709120"},
		{name: "init peak", containers: []v1.Container{container("a", "2", "", "2Gi", "")}, init: []v1.Container{container("init", "4", "", "6Gi", "")}, cpu: "4000", memory: "6442450944"},
		{name: "unspecified defaults", containers: []v1.Container{container("app", "", "", "", "")}},
		{name: "one dimension unspecified", containers: []v1.Container{container("app", "2", "", "", "")}, cpu: "2000"},
		{name: "partially unspecified rejects", containers: []v1.Container{container("a", "1", "", "", ""), container("b", "", "", "", "")}, errorText: "for every container"},
		{name: "zero rejected", containers: []v1.Container{container("a", "0", "", "", "")}, errorText: "must be positive"},
		{name: "negative rejected", containers: []v1.Container{container("a", "-1", "", "", "")}, errorText: "must be positive"},
		{name: "memory overflow", containers: []v1.Container{container("a", "", "", "9223372036854775808", "")}, errorText: "out of range"},
		{name: "cpu overflow", containers: []v1.Container{container("a", "9223372036854775808m", "", "", "")}, errorText: "out of range"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := &v1.Pod{Spec: v1.PodSpec{Containers: test.containers, InitContainers: test.init,
				Overhead: v1.ResourceList{v1.ResourceCPU: resource.MustParse("1"), v1.ResourceMemory: resource.MustParse("1Gi")}}}
			original := pod.DeepCopy()
			config := &runtimeapi.PodSandboxConfig{Annotations: map[string]string{
				darwinCRIPodSandboxConfigVersionAnnotation: "spoofed", darwinCPUMillisAnnotation: "999999", darwinMemoryBytesAnnotation: "1", "unrelated": "retained"}}
			err := applyDarwinSandboxResources(pod, config)
			if test.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), test.errorText) {
					t.Fatalf("error = %v, want %q", err, test.errorText)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string]string{darwinCRIPodSandboxConfigVersionAnnotation: darwinCRIPodSandboxConfigVersion, "unrelated": "retained"}
			if test.cpu != "" {
				expected[darwinCPUMillisAnnotation] = test.cpu
			}
			if test.memory != "" {
				expected[darwinMemoryBytesAnnotation] = test.memory
			}
			if !reflect.DeepEqual(config.Annotations, expected) {
				t.Fatalf("annotations = %v, want %v", config.Annotations, expected)
			}
			if !reflect.DeepEqual(pod, original) {
				t.Fatal("mutated original Pod")
			}
			if config.Linux != nil || config.Windows != nil {
				t.Fatal("populated foreign platform config")
			}
		})
	}
}
