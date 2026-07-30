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

package cm

import "testing"

func TestDarwinCPUAccountingConversions(t *testing.T) {
	const milliCPU = int64(500)

	shares := MilliCPUToShares(milliCPU)
	if shares != 512 {
		t.Fatalf("MilliCPUToShares(%d) = %d, want 512", milliCPU, shares)
	}
	if got := SharesToMilliCPU(int64(shares)); got != milliCPU {
		t.Fatalf("SharesToMilliCPU(%d) = %d, want %d", shares, got, milliCPU)
	}

	quota := MilliCPUToQuota(milliCPU, QuotaPeriod)
	if quota != 50000 {
		t.Fatalf("MilliCPUToQuota(%d, %d) = %d, want 50000", milliCPU, QuotaPeriod, quota)
	}
	if got := QuotaToMilliCPU(quota, QuotaPeriod); got != milliCPU {
		t.Fatalf("QuotaToMilliCPU(%d, %d) = %d, want %d", quota, QuotaPeriod, got, milliCPU)
	}
}

func TestDarwinCPUAccountingBounds(t *testing.T) {
	if got := MilliCPUToShares(1); got != MinShares {
		t.Fatalf("MilliCPUToShares(1) = %d, want minimum %d", got, MinShares)
	}
	if got := MilliCPUToShares(1 << 40); got != MaxShares {
		t.Fatalf("MilliCPUToShares(large) = %d, want maximum %d", got, MaxShares)
	}
	if got := QuotaToMilliCPU(-1, QuotaPeriod); got != 0 {
		t.Fatalf("QuotaToMilliCPU(-1, %d) = %d, want 0", QuotaPeriod, got)
	}
}
