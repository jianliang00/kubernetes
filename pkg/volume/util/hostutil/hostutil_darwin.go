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

package hostutil

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	utilpath "k8s.io/utils/path"
)

// HostUtil implements HostUtils for darwin platforms.
type HostUtil struct{}

// NewHostUtil returns a struct that implements HostUtils on darwin platforms.
func NewHostUtil() *HostUtil {
	return &HostUtil{}
}

func (hu *HostUtil) DeviceOpened(pathname string) (bool, error) {
	return false, nil
}

func (hu *HostUtil) PathIsDevice(pathname string) (bool, error) {
	pathType, err := hu.GetFileType(pathname)
	isDevice := pathType == FileTypeCharDev || pathType == FileTypeBlockDev
	return isDevice, err
}

func (hu *HostUtil) GetDeviceNameFromMount(mounter mount.Interface, mountPath, pluginMountDir string) (string, error) {
	return getDeviceNameFromMount(mounter, mountPath, pluginMountDir)
}

func getDeviceNameFromMount(mounter mount.Interface, mountPath, pluginMountDir string) (string, error) {
	refs, err := mounter.GetMountRefs(mountPath)
	if err != nil {
		klog.V(4).Infof("GetMountRefs failed for mount path %q: %v", mountPath, err)
		return "", err
	}
	if len(refs) == 0 {
		return "", fmt.Errorf("directory %s is not mounted", mountPath)
	}
	for _, ref := range refs {
		if !filepath.IsAbs(ref) {
			continue
		}
		if rel, err := filepath.Rel(pluginMountDir, ref); err == nil && rel != "." && !filepath.IsAbs(rel) && rel != ".." {
			return rel, nil
		}
	}
	return filepath.Base(mountPath), nil
}

func (hu *HostUtil) MakeRShared(path string) error {
	return nil
}

func (hu *HostUtil) GetFileType(pathname string) (FileType, error) {
	return getFileType(pathname)
}

func (hu *HostUtil) PathExists(pathname string) (bool, error) {
	return utilpath.Exists(utilpath.CheckFollowSymlink, pathname)
}

func (hu *HostUtil) EvalHostSymlinks(pathname string) (string, error) {
	return filepath.EvalSymlinks(pathname)
}

func (hu *HostUtil) GetOwner(pathname string) (int64, int64, error) {
	realpath, err := filepath.EvalSymlinks(pathname)
	if err != nil {
		return -1, -1, err
	}
	info, err := os.Stat(realpath)
	if err != nil {
		return -1, -1, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return -1, -1, fmt.Errorf("could not read owner for %q", pathname)
	}
	return int64(stat.Uid), int64(stat.Gid), nil
}

func (hu *HostUtil) GetSELinuxSupport(pathname string) (bool, error) {
	return false, nil
}

func (hu *HostUtil) GetMode(pathname string) (os.FileMode, error) {
	info, err := os.Stat(pathname)
	if err != nil {
		return 0, err
	}
	return info.Mode(), nil
}

func (hu *HostUtil) GetSELinuxMountContext(pathname string) (string, error) {
	return "", nil
}
