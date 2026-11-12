// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"strconv"
	"testing"
)

// TestUint32 键为uint32的类型，与官方map逐键对比
func TestUint32(t *testing.T) {
	var mapOffical = make(map[string]string)
	var mapAny = NewUint32(tempName(t))
	//set
	for i := 0; i < 10000; i++ {
		mapOffical[strconv.Itoa(i)] = strconv.Itoa(i)
		mapAny.SetString(uint32(i), strconv.Itoa(i))
	}
	mapAny.SetFinished()
	//get
	for i := 0; i < 10000; i++ {
		valOffical, _ := mapOffical[strconv.Itoa(i)]
		valAny, _ := mapAny.GetString(uint32(i))
		valAnyUnsafe, _ := mapAny.GetUnsafe(uint32(i))
		valAnyStringUnsafe, _ := mapAny.GetStringUnsafe(uint32(i))
		if valOffical != valAny {
			t.Fatalf("unexpected value obtained; got %q want %q", valAny, valOffical)
		}
		if valOffical != string(valAnyUnsafe) {
			t.Fatalf("unexpected value obtained; got %q want %q", string(valAnyUnsafe), valOffical)
		}
		if valOffical != valAnyStringUnsafe {
			t.Fatalf("unexpected value obtained; got %q want %q", valAnyStringUnsafe, valOffical)
		}
	}
	//不存在的键
	if _, exist := mapAny.Get(999999); exist {
		t.Fatal("unexpected exist; got true want false")
	}
	if mapAny.Len() != 10000 {
		t.Fatalf("unexpected len; got %v want %v", mapAny.Len(), 10000)
	}
}

// TestUint32DuplicateKey 重复键必须立即报错
func TestUint32DuplicateKey(t *testing.T) {
	m := NewUint32(tempName(t))
	m.SetString(1, "v1")
	assertPanic(t, "for twice", func() {
		m.SetString(1, "v2")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after duplicate set; got %v want %v", m.Len(), 1)
	}
	m.SetFinished()
	if v, exist := m.GetString(1); !exist || v != "v1" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
}

// TestUint32ValueTooLong 值的长度上限校验
func TestUint32ValueTooLong(t *testing.T) {
	m := NewUint32(tempName(t))
	m.Set(1, make([]byte, maxKVLen))
	assertPanic(t, "too long", func() {
		m.Set(2, make([]byte, maxKVLen+1))
	})
	m.SetFinished()
	if v, exist := m.Get(1); !exist || len(v) != maxKVLen {
		t.Fatalf("unexpected value; got len=%v exist=%v", len(v), exist)
	}
}

// TestUint32BoundaryKey 键类型边界值
func TestUint32BoundaryKey(t *testing.T) {
	m := NewUint32(tempName(t))
	m.SetString(0, "zero")
	m.SetString(^uint32(0), "max")
	m.SetFinished()
	if v, exist := m.GetString(0); !exist || v != "zero" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if v, exist := m.GetString(^uint32(0)); !exist || v != "max" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
}
