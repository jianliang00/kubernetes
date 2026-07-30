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
	"context"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/utils/ptr"
)

func TestValidatePlatformSpecificContainerEnvDarwin(t *testing.T) {
	if err := validatePlatformSpecificContainerEnv("VALID", []byte("macOS-节点")); err != nil {
		t.Fatalf("valid UTF-8 value was rejected: %v", err)
	}

	if err := validatePlatformSpecificContainerEnv("INVALID", []byte{0xff, 0xfe}); err == nil {
		t.Fatal("non-UTF-8 value was accepted")
	}
}

func TestApplySandboxResourcesRejectsUnsupportedDarwinFields(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		mutatePod func(*v1.Pod)
	}{
		{
			name:  "host network",
			field: "spec.hostNetwork",
			mutatePod: func(pod *v1.Pod) {
				pod.Spec.HostNetwork = true
			},
		},
		{
			name:  "shared process namespace",
			field: "spec.shareProcessNamespace",
			mutatePod: func(pod *v1.Pod) {
				pod.Spec.ShareProcessNamespace = ptr.To(true)
			},
		},
		{
			name:  "pod user namespace",
			field: "spec.hostUsers",
			mutatePod: func(pod *v1.Pod) {
				pod.Spec.HostUsers = ptr.To(false)
			},
		},
		{
			name:  "pod run as user",
			field: "spec.securityContext.runAsUser",
			mutatePod: func(pod *v1.Pod) {
				pod.Spec.SecurityContext = &v1.PodSecurityContext{RunAsUser: ptr.To[int64](1000)}
			},
		},
		{
			name:  "pod run as non-root",
			field: "spec.securityContext.runAsNonRoot",
			mutatePod: func(pod *v1.Pod) {
				pod.Spec.SecurityContext = &v1.PodSecurityContext{RunAsNonRoot: ptr.To(true)}
			},
		},
		{
			name:  "container capabilities",
			field: `spec.containers["app"].securityContext.capabilities`,
			mutatePod: func(pod *v1.Pod) {
				pod.Spec.Containers[0].SecurityContext = &v1.SecurityContext{
					Capabilities: &v1.Capabilities{Drop: []v1.Capability{"ALL"}},
				}
			},
		},
		{
			name:  "container privilege escalation",
			field: `spec.containers["app"].securityContext.allowPrivilegeEscalation`,
			mutatePod: func(pod *v1.Pod) {
				pod.Spec.Containers[0].SecurityContext = &v1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false)}
			},
		},
		{
			name:  "host port",
			field: ".hostPort",
			mutatePod: func(pod *v1.Pod) {
				pod.Spec.Containers[0].Ports = []v1.ContainerPort{{Name: "http", HostPort: 8080}}
			},
		},
	}

	manager := &kubeGenericRuntimeManager{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := &v1.Pod{Spec: v1.PodSpec{Containers: []v1.Container{{Name: "app"}}}}
			test.mutatePod(pod)
			err := manager.applySandboxResources(context.Background(), pod, &runtimeapi.PodSandboxConfig{})
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("applySandboxResources() error = %v, want field %q", err, test.field)
			}
		})
	}
}

func TestApplyPlatformSpecificContainerConfigRejectsTargetContainer(t *testing.T) {
	manager := &kubeGenericRuntimeManager{}
	pod := &v1.Pod{}
	container := &v1.Container{Name: "debugger"}
	target := &kubecontainer.ContainerID{Type: "macos", ID: "target"}

	err := manager.applyPlatformSpecificContainerConfig(context.Background(), &runtimeapi.ContainerConfig{}, container, pod, nil, "", target)
	if err == nil || !strings.Contains(err.Error(), "container.targetContainerName") {
		t.Fatalf("applyPlatformSpecificContainerConfig() error = %v, want target container rejection", err)
	}
}

func TestDarwinDefaultConfigAddsContractAnnotations(t *testing.T) {
	manager := &kubeGenericRuntimeManager{}
	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "app"}},
		},
	}
	sandboxConfig := &runtimeapi.PodSandboxConfig{}
	if err := manager.applySandboxResources(context.Background(), pod, sandboxConfig); err != nil {
		t.Fatalf("applySandboxResources() error = %v", err)
	}
	if got := sandboxConfig.Annotations[darwinCRIPlatformAnnotation]; got != darwinCRIPlatform {
		t.Fatalf("sandbox platform annotation = %q, want %q", got, darwinCRIPlatform)
	}

	containerConfig := &runtimeapi.ContainerConfig{}
	if err := manager.applyPlatformSpecificContainerConfig(context.Background(), containerConfig, &pod.Spec.Containers[0], pod, nil, "", nil); err != nil {
		t.Fatalf("applyPlatformSpecificContainerConfig() error = %v", err)
	}
	if got := containerConfig.Annotations[darwinCRIPlatformAnnotation]; got != darwinCRIPlatform {
		t.Fatalf("container platform annotation = %q, want %q", got, darwinCRIPlatform)
	}
}
