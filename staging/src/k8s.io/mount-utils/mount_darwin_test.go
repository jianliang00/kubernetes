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

package mount

import (
	"errors"
	"testing"
)

func TestDarwinMutableMountOperationsUnsupported(t *testing.T) {
	mounter := New("").(*Mounter)

	if err := mounter.Mount("/source", "/target", "apfs", nil); !errors.Is(err, errUnsupported) {
		t.Fatalf("Mount() error = %v, want %v", err, errUnsupported)
	}
	if err := mounter.MountSensitive("/source", "/target", "apfs", nil, nil); !errors.Is(err, errUnsupported) {
		t.Fatalf("MountSensitive() error = %v, want %v", err, errUnsupported)
	}
	if err := mounter.MountSensitiveWithoutSystemd("/source", "/target", "apfs", nil, nil); !errors.Is(err, errUnsupported) {
		t.Fatalf("MountSensitiveWithoutSystemd() error = %v, want %v", err, errUnsupported)
	}
	if err := mounter.MountSensitiveWithoutSystemdWithMountFlags("/source", "/target", "apfs", nil, nil, nil); !errors.Is(err, errUnsupported) {
		t.Fatalf("MountSensitiveWithoutSystemdWithMountFlags() error = %v, want %v", err, errUnsupported)
	}
}
