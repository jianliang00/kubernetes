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

package hostutil

import (
	"errors"
	"path/filepath"
	"testing"

	"k8s.io/mount-utils"
)

func TestMakeRSharedUnsupportedOnDarwin(t *testing.T) {
	hu := NewHostUtil()
	if err := hu.MakeRShared(t.TempDir()); !errors.Is(err, errUnsupported) {
		t.Fatalf("MakeRShared() error = %v, want %v", err, errUnsupported)
	}
}

func TestDeviceOpenedFailsClosedOnDarwin(t *testing.T) {
	hu := NewHostUtil()
	opened, err := hu.DeviceOpened("/dev/disk-test")
	if opened {
		t.Fatal("DeviceOpened() = true, want false")
	}
	if !errors.Is(err, errUnsupported) {
		t.Fatalf("DeviceOpened() error = %v, want %v", err, errUnsupported)
	}
}

func TestGetDeviceNameFromMountRejectsSiblingReference(t *testing.T) {
	root := t.TempDir()
	mountPath := filepath.Join(root, "pods", "volume")
	pluginMountDir := filepath.Join(root, "plugins", "target")
	siblingRef := filepath.Join(root, "plugins", "sibling", "reference")
	mounter := mount.NewFakeMounter([]mount.MountPoint{
		{Device: "/dev/disk-test", Path: mountPath},
		{Device: "/dev/disk-test", Path: siblingRef},
	})

	got, err := getDeviceNameFromMount(mounter, mountPath, pluginMountDir)
	if err != nil {
		t.Fatalf("getDeviceNameFromMount() error = %v", err)
	}
	if want := filepath.Base(mountPath); got != want {
		t.Fatalf("getDeviceNameFromMount() = %q, want fallback %q", got, want)
	}
}
