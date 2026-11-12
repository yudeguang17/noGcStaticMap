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

// TestHuge 不限制键值长度的类型，与官方map逐键对比
func TestHuge(t *testing.T) {
	var mapOffical = make(map[string]string)
	var m = NewHuge(tempName(t))
	//set
	for i := 0; i < 10000; i++ {
		mapOffical[strconv.Itoa(i)] = strconv.Itoa(i)
		m.SetString(strconv.Itoa(i), strconv.Itoa(i))
	}
	m.SetFinished()
	//get
	for i := 0; i < 10000; i++ {
		valOffical, _ := mapOffical[strconv.Itoa(i)]
		valAny, _ := m.GetString(strconv.Itoa(i))
		valAnyUnsafe, _ := m.GetUnsafe([]byte(strconv.Itoa(i)))
		if valOffical != valAny {
			t.Fatalf("unexpected value obtained; got %q want %q", valAny, valOffical)
		}
		if valOffical != string(valAnyUnsafe) {
			t.Fatalf("unexpected value obtained; got %q want %q", string(valAnyUnsafe), valOffical)
		}
	}
}

// TestHugeLargeValue 本类型不受65535限制，可以存储超长值
func TestHugeLargeValue(t *testing.T) {
	m := NewHuge(tempName(t))
	big := make([]byte, 200000)
	for i := range big {
		big[i] = byte(i % 251)
	}
	m.Set([]byte("big"), big)
	m.Set([]byte("small"), []byte("s"))
	m.SetFinished()
	v, exist := m.Get([]byte("big"))
	if !exist || len(v) != len(big) {
		t.Fatalf("unexpected value; got len=%v exist=%v", len(v), exist)
	}
	for i := range big {
		if v[i] != big[i] {
			t.Fatalf("unexpected byte at %v", i)
		}
	}
	if v, exist := m.Get([]byte("small")); !exist || string(v) != "s" {
		t.Fatalf("unexpected value; got %q exist=%v", string(v), exist)
	}
}

// TestHugeDuplicateKey 重复键必须立即报错
func TestHugeDuplicateKey(t *testing.T) {
	m := NewHuge(tempName(t))
	m.SetString("k", "v1")
	assertPanic(t, "for twice", func() {
		m.SetString("k", "v2")
	})
	if m.Len() != 1 {
		t.Fatalf("unexpected len after duplicate set; got %v want %v", m.Len(), 1)
	}
	m.SetFinished()
	if v, exist := m.GetString("k"); !exist || v != "v1" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
}

// TestHugeProbeCollision 人为制造哈希冲突，验证探测序列
func TestHugeProbeCollision(t *testing.T) {
	m := NewHuge(tempName(t))
	m.SetString("first", "v1")
	m.index[hashKey([]byte("second"))] = 0
	m.SetString("second", "v2")
	assertPanic(t, "for twice", func() {
		m.SetString("second", "v2again")
	})
	m.SetFinished()
	for _, kv := range [][2]string{{"first", "v1"}, {"second", "v2"}} {
		if v, exist := m.GetString(kv[0]); !exist || v != kv[1] {
			t.Fatalf("unexpected value for %v; got %q exist=%v", kv[0], v, exist)
		}
	}
	if _, exist := m.GetString("third"); exist {
		t.Fatal("unexpected exist; got true want false")
	}
}

// TestHugeLenAndFinished 计数与幂等的SetFinished
func TestHugeLenAndFinished(t *testing.T) {
	m := NewHuge(tempName(t))
	for i := 0; i < 100; i++ {
		m.SetString(strconv.Itoa(i), strconv.Itoa(i))
	}
	if m.Len() != 100 {
		t.Fatalf("unexpected len; got %v want %v", m.Len(), 100)
	}
	m.SetFinished()
	m.SetFinished()
	if m.Len() != 100 {
		t.Fatalf("unexpected len after SetFinished; got %v want %v", m.Len(), 100)
	}
	if v, exist := m.GetString("99"); !exist || v != "99" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
}
