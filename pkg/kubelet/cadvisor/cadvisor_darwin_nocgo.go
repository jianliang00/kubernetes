//go:build darwin && !cgo

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

	"k8s.io/klog/v2"
)

// New returns a deterministic build-time configuration error when Darwin is
// built without cgo. The real Darwin implementation uses Mach host APIs.
func New(_ klog.Logger, _ ImageFsInfoProvider, _ string, _ []string, _, _ bool) (Interface, error) {
	return nil, errors.New("darwin cAdvisor requires CGO_ENABLED=1 to read Mach host statistics")
}

func IsPsiEnabled(_ klog.Logger) bool {
	return false
}
