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
	"fmt"
	"math"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	resourcehelper "k8s.io/kubernetes/pkg/api/v1/resource"
)

const (
	darwinCPUMillisAnnotation   = "io.kubernetes.cri.macos/cpu-millis"
	darwinMemoryBytesAnnotation = "io.kubernetes.cri.macos/memory-bytes"
)

// VM capacity is fixed at sandbox creation. Limits choose capacity, with requests
// used when a limit is absent. Pod overhead remains host-side accounting and is
// excluded from guest RAM and vCPUs.
func applyDarwinSandboxResources(pod *v1.Pod, config *runtimeapi.PodSandboxConfig) error {
	// These fields are produced by kubelet, never taken from user annotations.
	delete(config.Annotations, darwinCRIPodSandboxConfigVersionAnnotation)
	delete(config.Annotations, darwinCPUMillisAnnotation)
	delete(config.Annotations, darwinMemoryBytesAnnotation)
	values := map[string]string{}
	for _, name := range []v1.ResourceName{v1.ResourceCPU, v1.ResourceMemory} {
		quantity, err := darwinSandboxResource(pod, name)
		if err != nil {
			return err
		}
		if quantity.IsZero() {
			continue
		}
		maximum := resource.NewQuantity(math.MaxInt64, resource.DecimalSI)
		key := darwinMemoryBytesAnnotation
		if name == v1.ResourceCPU {
			maximum = resource.NewMilliQuantity(math.MaxInt64, resource.DecimalSI)
			key = darwinCPUMillisAnnotation
		}
		if quantity.Sign() < 0 || quantity.Cmp(*maximum) > 0 {
			return fmt.Errorf("macOS pod %s capacity is out of range", name)
		}
		value := quantity.Value()
		if name == v1.ResourceCPU {
			value = quantity.MilliValue()
		}
		values[key] = strconv.FormatInt(value, 10)
	}
	if config.Annotations == nil {
		config.Annotations = map[string]string{}
	}
	config.Annotations[darwinCRIPodSandboxConfigVersionAnnotation] = darwinCRIPodSandboxConfigVersion
	for key, value := range values {
		config.Annotations[key] = value
	}
	return nil
}

func darwinSandboxResource(pod *v1.Pod, name v1.ResourceName) (resource.Quantity, error) {
	// Normalize each container before using Kubernetes' init/sidecar aggregation.
	// Aggregating raw limits would silently omit request-only containers.
	normalized := pod.DeepCopy()
	declared, missing := false, false
	for _, containers := range [][]v1.Container{normalized.Spec.Containers, normalized.Spec.InitContainers} {
		for i := range containers {
			value, ok := darwinResourceCapacity(containers[i].Resources, name)
			if !ok {
				missing = true
				continue
			}
			if value.Sign() <= 0 {
				return resource.Quantity{}, fmt.Errorf("macOS container %q %s capacity must be positive", containers[i].Name, name)
			}
			declared = true
			if containers[i].Resources.Limits == nil {
				containers[i].Resources.Limits = v1.ResourceList{}
			}
			containers[i].Resources.Limits[name] = value
		}
	}
	if declared && missing {
		return resource.Quantity{}, fmt.Errorf("macOS pod requires a %s limit or request for every container when that resource is declared", name)
	}
	if !declared {
		return resource.Quantity{}, nil
	}
	limits := resourcehelper.PodLimits(normalized, resourcehelper.PodResourcesOptions{
		ExcludeOverhead: true,
	})
	return limits[name], nil
}

func darwinResourceCapacity(resources v1.ResourceRequirements, name v1.ResourceName) (resource.Quantity, bool) {
	if value, ok := resources.Limits[name]; ok {
		return value, true
	}
	value, ok := resources.Requests[name]
	return value, ok
}
