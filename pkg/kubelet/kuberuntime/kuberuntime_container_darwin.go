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
	"fmt"
	"strings"
	"unicode/utf8"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	"k8s.io/kubernetes/pkg/kubelet/types"
)

func validatePlatformSpecificContainerEnv(name string, value []byte) error {
	if !utf8.Valid(value) {
		return fmt.Errorf("environment variable %q contains non-UTF-8 bytes, which the macOS CRI v1alpha1 contract does not support", name)
	}
	return nil
}

func validateDarwinPodSandboxConfig(pod *v1.Pod) error {
	var fields []string
	if pod.Spec.HostNetwork {
		fields = append(fields, "spec.hostNetwork")
	}
	if pod.Spec.HostPID {
		fields = append(fields, "spec.hostPID")
	}
	if pod.Spec.HostIPC {
		fields = append(fields, "spec.hostIPC")
	}
	if pod.Spec.ShareProcessNamespace != nil && *pod.Spec.ShareProcessNamespace {
		fields = append(fields, "spec.shareProcessNamespace")
	}
	if pod.Spec.HostUsers != nil && !*pod.Spec.HostUsers {
		fields = append(fields, "spec.hostUsers")
	}
	fields = appendDarwinUnsupportedPodSecurityContextFields(fields, "spec.securityContext", pod.Spec.SecurityContext)

	for i := range pod.Spec.InitContainers {
		container := &pod.Spec.InitContainers[i]
		prefix := fmt.Sprintf("spec.initContainers[%q]", container.Name)
		fields = appendDarwinUnsupportedContainerFields(fields, prefix, container)
	}
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		prefix := fmt.Sprintf("spec.containers[%q]", container.Name)
		fields = appendDarwinUnsupportedContainerFields(fields, prefix, container)
	}

	return darwinUnsupportedFieldsError("pod sandbox", fields)
}

func validateDarwinContainerConfig(pod *v1.Pod, container *v1.Container, nsTarget *kubecontainer.ContainerID) error {
	var fields []string
	fields = appendDarwinUnsupportedPodSecurityContextFields(fields, "spec.securityContext", pod.Spec.SecurityContext)
	fields = appendDarwinUnsupportedContainerSecurityContextFields(fields, "container.securityContext", container.SecurityContext)
	if nsTarget != nil {
		fields = append(fields, "container.targetContainerName")
	}
	return darwinUnsupportedFieldsError("container", fields)
}

func appendDarwinUnsupportedPodSecurityContextFields(fields []string, prefix string, securityContext *v1.PodSecurityContext) []string {
	if securityContext == nil {
		return fields
	}
	if securityContext.SELinuxOptions != nil {
		fields = append(fields, prefix+".seLinuxOptions")
	}
	if securityContext.WindowsOptions != nil {
		fields = append(fields, prefix+".windowsOptions")
	}
	if securityContext.RunAsUser != nil {
		fields = append(fields, prefix+".runAsUser")
	}
	if securityContext.RunAsGroup != nil {
		fields = append(fields, prefix+".runAsGroup")
	}
	if securityContext.RunAsNonRoot != nil && *securityContext.RunAsNonRoot {
		fields = append(fields, prefix+".runAsNonRoot")
	}
	if len(securityContext.SupplementalGroups) > 0 {
		fields = append(fields, prefix+".supplementalGroups")
	}
	if securityContext.SupplementalGroupsPolicy != nil {
		fields = append(fields, prefix+".supplementalGroupsPolicy")
	}
	if securityContext.FSGroup != nil {
		fields = append(fields, prefix+".fsGroup")
	}
	if len(securityContext.Sysctls) > 0 {
		fields = append(fields, prefix+".sysctls")
	}
	if securityContext.FSGroupChangePolicy != nil {
		fields = append(fields, prefix+".fsGroupChangePolicy")
	}
	if securityContext.SeccompProfile != nil {
		fields = append(fields, prefix+".seccompProfile")
	}
	if securityContext.AppArmorProfile != nil {
		fields = append(fields, prefix+".appArmorProfile")
	}
	if securityContext.SELinuxChangePolicy != nil {
		fields = append(fields, prefix+".seLinuxChangePolicy")
	}
	return fields
}

func appendDarwinUnsupportedContainerFields(fields []string, prefix string, container *v1.Container) []string {
	fields = appendDarwinUnsupportedContainerSecurityContextFields(fields, prefix+".securityContext", container.SecurityContext)
	for i := range container.Ports {
		if container.Ports[i].HostPort != 0 {
			fields = append(fields, fmt.Sprintf("%s.ports[%q].hostPort", prefix, container.Ports[i].Name))
		}
	}
	return fields
}

func appendDarwinUnsupportedContainerSecurityContextFields(fields []string, prefix string, securityContext *v1.SecurityContext) []string {
	if securityContext == nil {
		return fields
	}
	if securityContext.Capabilities != nil {
		fields = append(fields, prefix+".capabilities")
	}
	if securityContext.Privileged != nil {
		fields = append(fields, prefix+".privileged")
	}
	if securityContext.SELinuxOptions != nil {
		fields = append(fields, prefix+".seLinuxOptions")
	}
	if securityContext.WindowsOptions != nil {
		fields = append(fields, prefix+".windowsOptions")
	}
	if securityContext.RunAsUser != nil {
		fields = append(fields, prefix+".runAsUser")
	}
	if securityContext.RunAsGroup != nil {
		fields = append(fields, prefix+".runAsGroup")
	}
	if securityContext.RunAsNonRoot != nil && *securityContext.RunAsNonRoot {
		fields = append(fields, prefix+".runAsNonRoot")
	}
	if securityContext.ReadOnlyRootFilesystem != nil {
		fields = append(fields, prefix+".readOnlyRootFilesystem")
	}
	if securityContext.AllowPrivilegeEscalation != nil {
		fields = append(fields, prefix+".allowPrivilegeEscalation")
	}
	if securityContext.ProcMount != nil {
		fields = append(fields, prefix+".procMount")
	}
	if securityContext.SeccompProfile != nil {
		fields = append(fields, prefix+".seccompProfile")
	}
	if securityContext.AppArmorProfile != nil {
		fields = append(fields, prefix+".appArmorProfile")
	}
	return fields
}

func darwinUnsupportedFieldsError(scope string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	return fmt.Errorf("unsupported %s field(s) for macOS CRI v1alpha1: %s", scope, strings.Join(fields, ", "))
}

// applyPlatformSpecificContainerConfig applies the macOS CRI extension contract.
func (m *kubeGenericRuntimeManager) applyPlatformSpecificContainerConfig(_ context.Context, config *runtimeapi.ContainerConfig, container *v1.Container, pod *v1.Pod, _ *int64, _ string, nsTarget *kubecontainer.ContainerID) error {
	if err := validateDarwinContainerConfig(pod, container, nsTarget); err != nil {
		return err
	}
	if config.Annotations == nil {
		config.Annotations = map[string]string{}
	}
	config.Annotations[darwinCRIPlatformAnnotation] = darwinCRIPlatform
	config.Annotations[darwinCRIContainerConfigVersionAnnotation] = darwinCRIConfigVersion
	return nil
}

// generateContainerResources returns an explicit empty resource update for macOS.
//
// CRI has only Linux and Windows resource fields. The macOS runtime rejects
// this request with a recreate-required error instead of reporting an in-place
// resize as successful. VM capacity is set by RunPodSandbox annotations.
func (m *kubeGenericRuntimeManager) generateContainerResources(_ context.Context, pod *v1.Pod, container *v1.Container) *runtimeapi.ContainerResources {
	return &runtimeapi.ContainerResources{}
}

// generateUpdatePodSandboxResourcesRequest is unsupported by the macOS CRI
// extension because CRI has no Darwin resource payload.
func (m *kubeGenericRuntimeManager) generateUpdatePodSandboxResourcesRequest(sandboxID string, pod *v1.Pod, podResources *cm.ResourceConfig) *runtimeapi.UpdatePodSandboxResourcesRequest {
	return nil
}

func toKubeContainerResources(statusResources *runtimeapi.ContainerResources) *kubecontainer.ContainerResources {
	var cStatusResources *kubecontainer.ContainerResources
	runtimeStatusResources := statusResources.GetLinux()
	if runtimeStatusResources != nil {
		var cpuLimit, cpuRequest, memLimit *resource.Quantity
		if runtimeStatusResources.CpuPeriod > 0 {
			milliCPU := cm.QuotaToMilliCPU(runtimeStatusResources.CpuQuota, runtimeStatusResources.CpuPeriod)
			if milliCPU > 0 {
				cpuLimit = resource.NewMilliQuantity(milliCPU, resource.DecimalSI)
			}
		}
		if runtimeStatusResources.CpuShares > 0 {
			milliCPU := cm.SharesToMilliCPU(runtimeStatusResources.CpuShares)
			if milliCPU > 0 {
				cpuRequest = resource.NewMilliQuantity(milliCPU, resource.DecimalSI)
			}
		}
		if runtimeStatusResources.MemoryLimitInBytes > 0 {
			memLimit = resource.NewQuantity(runtimeStatusResources.MemoryLimitInBytes, resource.BinarySI)
		}
		if cpuLimit != nil || cpuRequest != nil || memLimit != nil {
			cStatusResources = &kubecontainer.ContainerResources{
				CPULimit:      cpuLimit,
				CPURequest:    cpuRequest,
				MemoryLimit:   memLimit,
				MemoryRequest: nil,
			}
		}
	}
	return cStatusResources
}

func toKubeContainerUser(statusUser *runtimeapi.ContainerUser) *kubecontainer.ContainerUser {
	if statusUser == nil {
		return nil
	}

	user := &kubecontainer.ContainerUser{}
	if statusUser.GetLinux() != nil {
		user.Linux = &kubecontainer.LinuxContainerUser{
			UID:                statusUser.GetLinux().GetUid(),
			GID:                statusUser.GetLinux().GetGid(),
			SupplementalGroups: statusUser.GetLinux().GetSupplementalGroups(),
		}
	}
	return user
}

func (m *kubeGenericRuntimeManager) GetContainerSwapBehavior(pod *v1.Pod, container *v1.Container) types.SwapBehavior {
	return types.NoSwap
}

// initSwapControllerAvailabilityCheck returns a function that always returns
// false because Darwin has no Linux cgroup swap controller.
func initSwapControllerAvailabilityCheck(_ context.Context) func() bool {
	return func() bool { return false }
}
