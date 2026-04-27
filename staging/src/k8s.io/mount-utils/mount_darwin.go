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

package mount

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

var errUnsupported = errors.New("util/mount mutable operations are not supported on darwin")

// Mounter provides the default implementation of mount.Interface for Darwin.
// Darwin kubelet experiments use this for mount table inspection and safe
// cleanup. Kubernetes volume code must not rely on Linux-style mount creation
// on this platform.
type Mounter struct {
	mounterPath string
}

// New returns a mount.Interface for the current system.
func New(mounterPath string) Interface {
	return &Mounter{
		mounterPath: mounterPath,
	}
}

// Mount returns a deterministic unsupported error for Darwin.
func (mounter *Mounter) Mount(source string, target string, fstype string, options []string) error {
	return errUnsupported
}

// MountSensitive returns a deterministic unsupported error for Darwin.
func (mounter *Mounter) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return errUnsupported
}

// MountSensitiveWithoutSystemd returns a deterministic unsupported error for Darwin.
func (mounter *Mounter) MountSensitiveWithoutSystemd(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return errUnsupported
}

// MountSensitiveWithoutSystemdWithMountFlags returns a deterministic unsupported error for Darwin.
func (mounter *Mounter) MountSensitiveWithoutSystemdWithMountFlags(source string, target string, fstype string, options []string, sensitiveOptions []string, mountFlags []string) error {
	return errUnsupported
}

// Unmount unmounts target when it is a Darwin mount point. It treats normal
// directories as already unmounted so kubelet cleanup can remove pod state
// directories without failing on unsupported Linux mount helpers.
func (mounter *Mounter) Unmount(target string) error {
	isMountPoint, err := mounter.IsMountPoint(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if !isMountPoint {
		return nil
	}
	klog.V(4).Infof("Unmounting %s", target)
	output, err := exec.Command("umount", target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount %q failed: %w, output: %s", target, err, string(output))
	}
	return nil
}

// List returns the current Darwin mount table from getfsstat(2).
func (*Mounter) List() ([]MountPoint, error) {
	count, err := unix.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return []MountPoint{}, nil
	}
	stats := make([]unix.Statfs_t, count)
	count, err = unix.Getfsstat(stats, unix.MNT_NOWAIT)
	if err != nil {
		return nil, err
	}
	if count < len(stats) {
		stats = stats[:count]
	}

	mountPoints := make([]MountPoint, 0, len(stats))
	for _, stat := range stats {
		mountPoints = append(mountPoints, MountPoint{
			Device: byteString(stat.Mntfromname[:]),
			Path:   byteString(stat.Mntonname[:]),
			Type:   byteString(stat.Fstypename[:]),
			Opts:   darwinMountOptions(stat.Flags),
		})
	}
	return mountPoints, nil
}

// IsLikelyNotMountPoint determines whether file is not a mount point by
// comparing the device IDs of file and its parent.
func (mounter *Mounter) IsLikelyNotMountPoint(file string) (bool, error) {
	stat, err := os.Stat(file)
	if err != nil {
		return true, err
	}
	parent := filepath.Dir(filepath.Clean(file))
	parentStat, err := os.Stat(parent)
	if err != nil {
		return true, err
	}

	statSys, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return true, fmt.Errorf("could not read stat data for %q", file)
	}
	parentSys, ok := parentStat.Sys().(*syscall.Stat_t)
	if !ok {
		return true, fmt.Errorf("could not read stat data for parent of %q", file)
	}
	if statSys.Dev != parentSys.Dev {
		return false, nil
	}
	return true, nil
}

// CanSafelySkipMountPointCheck returns false because Darwin umount behavior is
// not used as the only source of truth.
func (mounter *Mounter) CanSafelySkipMountPointCheck() bool {
	return false
}

// IsMountPoint determines if file is listed as a mounted filesystem.
func (mounter *Mounter) IsMountPoint(file string) (bool, error) {
	resolvedFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, os.ErrNotExist
		}
		return false, err
	}
	mountPoints, err := mounter.List()
	if err != nil {
		return false, err
	}
	for _, mp := range mountPoints {
		if isMountPointMatch(mp, resolvedFile) {
			return true, nil
		}
	}
	return false, nil
}

// GetMountRefs finds other mount references to pathname.
func (mounter *Mounter) GetMountRefs(pathname string) ([]string, error) {
	pathExists, pathErr := PathExists(pathname)
	if !pathExists {
		return []string{}, nil
	} else if IsCorruptedMnt(pathErr) {
		klog.Warningf("GetMountRefs found corrupted mount at %s, treating as unmounted path", pathname)
		return []string{}, nil
	} else if pathErr != nil {
		return nil, fmt.Errorf("error checking path %s: %v", pathname, pathErr)
	}
	realpath, err := filepath.EvalSymlinks(pathname)
	if err != nil {
		return nil, err
	}
	return getMountRefsByDev(mounter, realpath)
}

func (mounter *SafeFormatAndMount) formatAndMountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string, formatOptions []string) error {
	return mounter.Interface.Mount(source, target, fstype, options)
}

func (mounter *SafeFormatAndMount) diskLooksUnformatted(disk string) (bool, error) {
	return true, errUnsupported
}

// IsMountPoint determines if file is listed as a mounted filesystem.
func (mounter *SafeFormatAndMount) IsMountPoint(file string) (bool, error) {
	return mounter.Interface.IsMountPoint(file)
}

func byteString(bytes []byte) string {
	for i, b := range bytes {
		if b == 0 {
			return string(bytes[:i])
		}
	}
	return string(bytes)
}

func darwinMountOptions(flags uint32) []string {
	var opts []string
	if flags&unix.MNT_RDONLY != 0 {
		opts = append(opts, "ro")
	}
	if flags&unix.MNT_LOCAL != 0 {
		opts = append(opts, "local")
	}
	if flags&unix.MNT_NODEV != 0 {
		opts = append(opts, "nodev")
	}
	if flags&unix.MNT_NOEXEC != 0 {
		opts = append(opts, "noexec")
	}
	if flags&unix.MNT_NOSUID != 0 {
		opts = append(opts, "nosuid")
	}
	return opts
}
