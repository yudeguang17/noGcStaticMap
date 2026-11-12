// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"fmt"
	"path/filepath"
	"testing"
)

// sprintElem 测试用的参照实现：单个元素按fmt.Sprint转换
func sprintElem(elem interface{}) string {
	return fmt.Sprint(elem)
}

// TestUnsafeEmptyValueNilSemantics 空值时 Unsafe 系接口在四种类型上必须返回一致的nil语义
func TestUnsafeEmptyValueNilSemantics(t *testing.T) {
	a := NewDefault(filepath.Join(t.TempDir(), "emptyNilAny"))
	a.Set([]byte("k"), []byte(""))
	a.Set([]byte("k2"), nil)
	a.SetFinished()
	if v, exist := a.GetUnsafe([]byte("k")); !exist || v != nil {
		t.Fatalf("any GetUnsafe empty value: got v==nil:%v exist:%v, want v==nil:true exist:true", v == nil, exist)
	}
	if v, exist := a.GetUnsafe([]byte("k2")); !exist || v != nil {
		t.Fatalf("any GetUnsafe nil value: got v==nil:%v exist:%v, want v==nil:true exist:true", v == nil, exist)
	}
	pos, exist := a.GetDataBeginPosOfKVPair([]byte("k"))
	if !exist {
		t.Fatal("any: unexpected exist; got false want true")
	}
	if v := a.GetValFromDataBeginPosOfKVPairUnSafe(int(pos)); v != nil {
		t.Fatalf("any GetValFromDataBeginPosOfKVPairUnSafe empty value: got v==nil:%v, want true", v == nil)
	}

	h := NewHuge(filepath.Join(t.TempDir(), "emptyNilHuge"))
	h.Set([]byte("k"), []byte(""))
	h.SetFinished()
	if v, exist := h.GetUnsafe([]byte("k")); !exist || v != nil {
		t.Fatalf("huge GetUnsafe empty value: got v==nil:%v exist:%v, want v==nil:true exist:true", v == nil, exist)
	}

	i := NewInt(filepath.Join(t.TempDir(), "emptyNilInt"))
	i.Set(1, []byte(""))
	i.SetFinished()
	if v, exist := i.GetUnsafe(1); !exist || v != nil {
		t.Fatalf("int GetUnsafe empty value: got v==nil:%v exist:%v, want v==nil:true exist:true", v == nil, exist)
	}

	u := NewUint32(filepath.Join(t.TempDir(), "emptyNilUint32"))
	u.Set(1, []byte(""))
	u.SetFinished()
	if v, exist := u.GetUnsafe(1); !exist || v != nil {
		t.Fatalf("uint32 GetUnsafe empty value: got v==nil:%v exist:%v, want v==nil:true exist:true", v == nil, exist)
	}
}

// TestUnsafePosCallBeforeFinished 未完成存储时用位置取值必须报状态错误，而不是越界崩溃
func TestUnsafePosCallBeforeFinished(t *testing.T) {
	a := NewDefault(filepath.Join(t.TempDir(), "stateAny"))
	defer a.SetFinished()
	a.Set([]byte("k"), []byte("v"))
	assertPanic(t, "can't Get before SetFinished", func() { a.GetValFromDataBeginPosOfKVPairUnSafe(0) })

	h := NewHuge(filepath.Join(t.TempDir(), "stateHuge"))
	defer h.SetFinished()
	h.Set([]byte("k"), []byte("v"))
	assertPanic(t, "can't Get before SetFinished", func() { h.GetValFromDataBeginPosOfKVPairUnSafe(0) })

	i := NewInt(filepath.Join(t.TempDir(), "stateInt"))
	defer i.SetFinished()
	i.Set(1, []byte("v"))
	assertPanic(t, "can't Get before SetFinished", func() { i.GetValFromDataBeginPosOfKVPairUnSafe(0) })

	u := NewUint32(filepath.Join(t.TempDir(), "stateUint32"))
	defer u.SetFinished()
	u.Set(1, []byte("v"))
	assertPanic(t, "can't Get before SetFinished", func() { u.GetValFromDataBeginPosOfKVPairUnSafe(0) })
}

// TestFileExistSemantics 只有确认存在才返回true，"无法判断"不能当作存在
func TestFileExistSemantics(t *testing.T) {
	dir := t.TempDir()
	existPath := filepath.Join(dir, "a.txt")
	m := NewInt(filepath.Join(dir, "fileExist"))
	m.SetString(1, "v")
	m.SetFinished() //同时验证SetFinished后临时文件被删除
	if fileExist(existPath) {
		t.Fatalf("unexpected exist; got true want false for %v", existPath)
	}
	if !fileExist(filepath.Join(dir)) {
		t.Fatal("directory should exist")
	}
	if !fileExist(".") {
		t.Fatal("the working directory should exist")
	}
	//路径语法错误/无法判断时必须是false，而不是true
	for _, p := range []string{
		filepath.Join(dir, "a|b"),
		filepath.Join(dir, "a<b"),
		filepath.Join(dir, "x\x00y"),
	} {
		if fileExist(p) {
			t.Fatalf("unjudgeable path treated as exist: %q", p)
		}
	}
}

// TestJoinInterfaceSameAsSprint JoinInterface的输出必须与逐元素fmt.Sprint拼接完全一致
func TestJoinInterfaceSameAsSprint(t *testing.T) {
	seps := []string{"`", "", "||"}
	cases := [][]interface{}{
		{1, "a", "b", "c"},
		{int64(-2), "中文", "", ""},
		{uint32(7), "x"},
		{"only"},
		{1.5, true, "s"},
		{nil, "s"},
		//异类类型：同时实现error与fmt.Stringer、String()内部panic，输出都必须与fmt.Sprint一致
		{errStringer{}, "x"},
		{"x", errStringer{}},
		{panickyStringer{}, "x"},
	}
	for _, sep := range seps {
		for _, elems := range cases {
			want := ""
			for i := range elems {
				if i > 0 {
					want += sep
				}
				want += sprintElem(elems[i])
			}
			if got := JoinInterface(sep, elems...); got != want {
				t.Fatalf("JoinInterface(%q, %v) = %q, want %q", sep, elems, got, want)
			}
		}
	}
	if got := JoinInterface("`"); got != "" {
		t.Fatalf("unexpected value; got %q want empty", got)
	}
}

// errStringer 同时实现error与fmt.Stringer：fmt优先调用Error()
type errStringer struct{}

func (errStringer) Error() string  { return "from-Error" }
func (errStringer) String() string { return "from-String" }

// panickyStringer String()内部panic：fmt会捕获并输出PANIC标记
type panickyStringer struct{}

func (panickyStringer) String() string { panic("boom") }

// TestJoinInterfaceErrorPriorityOfSprint 同时实现error与Stringer的类型必须按fmt的规则优先用Error()
func TestJoinInterfaceErrorPriorityOfSprint(t *testing.T) {
	got := JoinInterface("", errStringer{}, "x")
	want := fmt.Sprint(errStringer{}) + fmt.Sprint("x")
	if want != "from-Errorx" {
		t.Fatalf("参照实现输出变了；got %q want %q", want, "from-Errorx")
	}
	if got != want {
		t.Fatalf("unexpected value; got %q want %q", got, want)
	}
}

// TestJoinInterfaceStringPanicLikeSprint String()内部panic时必须与fmt一样被捕获，而不是把panic抛给调用方
func TestJoinInterfaceStringPanicLikeSprint(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("JoinInterface把String()的panic抛出来了：%v", r)
		}
	}()
	got := JoinInterface("", panickyStringer{}, "x")
	want := fmt.Sprint(panickyStringer{}) + fmt.Sprint("x")
	if got != want {
		t.Fatalf("unexpected value; got %q want %q", got, want)
	}
}
