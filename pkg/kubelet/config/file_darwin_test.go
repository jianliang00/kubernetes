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

package config

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"
)

func TestDarwinDoWatchMissingPathEmitsInitialUpdate(t *testing.T) {
	updates := make(chan sourceUpdate, 1)
	source := newSourceFile(filepath.Join(t.TempDir(), "missing"), "node", time.Second, updates)

	err := source.doWatch(klog.Background())
	var retryable *retryableError
	if !errors.As(err, &retryable) {
		t.Fatalf("doWatch() error = %v, want retryableError", err)
	}
	select {
	case update := <-updates:
		if len(update.Pods) != 0 {
			t.Fatalf("initial update contained %d pods, want 0", len(update.Pods))
		}
	default:
		t.Fatal("doWatch() did not emit an initial empty update")
	}
}

func TestDarwinProduceWatchEvent(t *testing.T) {
	tests := []struct {
		name string
		op   fsnotify.Op
		want podEventType
	}{
		{name: "create", op: fsnotify.Create, want: podAdd},
		{name: "write", op: fsnotify.Write, want: podModify},
		{name: "chmod", op: fsnotify.Chmod, want: podModify},
		{name: "remove", op: fsnotify.Remove, want: podDelete},
		{name: "rename", op: fsnotify.Rename, want: podDelete},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := &sourceFile{watchEvents: make(chan *watchEvent, 1)}
			event := &fsnotify.Event{Name: filepath.Join(t.TempDir(), "pod.yaml"), Op: test.op}
			if err := source.produceWatchEvent(klog.Background(), event); err != nil {
				t.Fatalf("produceWatchEvent() error = %v", err)
			}
			select {
			case got := <-source.watchEvents:
				if got.fileName != event.Name || got.eventType != test.want {
					t.Fatalf("watch event = %+v, want file %q and type %v", got, event.Name, test.want)
				}
			default:
				t.Fatal("produceWatchEvent() did not emit an event")
			}
		})
	}
}

func TestDarwinProduceWatchEventIgnoresHiddenAndUnknownEvents(t *testing.T) {
	source := &sourceFile{watchEvents: make(chan *watchEvent, 1)}
	for _, event := range []*fsnotify.Event{
		{Name: filepath.Join(t.TempDir(), ".pod.yaml"), Op: fsnotify.Create},
		{Name: filepath.Join(t.TempDir(), "pod.yaml")},
	} {
		if err := source.produceWatchEvent(klog.Background(), event); err != nil {
			t.Fatalf("produceWatchEvent() error = %v", err)
		}
	}
	select {
	case event := <-source.watchEvents:
		t.Fatalf("produceWatchEvent() emitted ignored event: %+v", event)
	default:
	}
}
