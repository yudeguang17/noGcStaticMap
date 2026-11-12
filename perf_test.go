// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"path/filepath"
	"testing"
)

// stringerCounter 带副作用的Stringer，用于统计String()被调用的次数
type stringerCounter struct {
	tag   string
	calls *int
}

func (s stringerCounter) String() string {
	*s.calls++
	return s.tag
}

// TestGetDataBeginPosNoValueCopy 只要位置的查询路径不允许复制value
func TestGetDataBeginPosNoValueCopy(t *testing.T) {
	value := make([]byte, 1024) //1KB的值，一旦被复制分配数会很显眼
	for i := range value {
		value[i] = byte(i)
	}
	key := []byte("k")

	a := NewDefault(filepath.Join(t.TempDir(), "noCopyAny"))
	a.Set(key, value)
	a.SetFinished()
	if allocs := testing.AllocsPerRun(200, func() {
		_, _ = a.GetDataBeginPosOfKVPair(key)
	}); allocs != 0 {
		t.Fatalf("any GetDataBeginPosOfKVPair allocs=%v, want 0", allocs)
	}
	if allocs := testing.AllocsPerRun(200, func() {
		_, _ = a.GetUnsafe(key)
	}); allocs != 0 {
		t.Fatalf("any GetUnsafe allocs=%v, want 0", allocs)
	}

	h := NewHuge(filepath.Join(t.TempDir(), "noCopyHuge"))
	h.Set(key, value)
	h.SetFinished()
	if allocs := testing.AllocsPerRun(200, func() {
		_, _ = h.GetDataBeginPosOfKVPair(key)
	}); allocs != 0 {
		t.Fatalf("huge GetDataBeginPosOfKVPair allocs=%v, want 0", allocs)
	}
	if allocs := testing.AllocsPerRun(200, func() {
		_, _ = h.GetUnsafe(key)
	}); allocs != 0 {
		t.Fatalf("huge GetUnsafe allocs=%v, want 0", allocs)
	}

	//值仍然必须能正确取到
	if v, exist := a.GetUnsafe(key); !exist || len(v) != len(value) {
		t.Fatalf("unexpected value; got len=%v exist=%v", len(v), exist)
	}
}

// TestJoinInterfaceStringerOnce 每个元素的String()只能被调用一次
func TestJoinInterfaceStringerOnce(t *testing.T) {
	calls := 0
	a := stringerCounter{tag: "a", calls: &calls}
	b := stringerCounter{tag: "b", calls: &calls}
	if got := JoinInterface("`", a, b); got != "a`b" {
		t.Fatalf("unexpected value; got %q want %q", got, "a`b")
	}
	if calls != 2 {
		t.Fatalf("String() called %v times, want 2 (once per element)", calls)
	}
	//单元素路径同样只能调用一次
	calls = 0
	if got := JoinInterface("`", a); got != "a" {
		t.Fatalf("unexpected value; got %q want %q", got, "a")
	}
	if calls != 1 {
		t.Fatalf("String() called %v times, want 1", calls)
	}
}
