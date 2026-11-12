// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestHashKeyEqualsStdFnv 索引用的哈希必须就是标准库 FNV-1a 64 位
func TestHashKeyEqualsStdFnv(t *testing.T) {
	for _, k := range []string{"", "a", "1000000", "hello world", "中文字符"} {
		h := fnv.New64a()
		_, _ = h.Write([]byte(k))
		if got, want := hashKey([]byte(k)), h.Sum64(); got != want {
			t.Fatalf("hashKey(%q) = %v, want %v", k, got, want)
		}
	}
}

// TestProbeSequence 探测序列必须由键唯一确定且不自我重复
func TestProbeSequence(t *testing.T) {
	h := hashKey([]byte("abc"))
	seen := make(map[uint64]bool, 1024)
	seen[h] = true
	for i := 0; i < 1000; i++ {
		h = nextProbeHash(h)
		if seen[h] {
			t.Fatalf("probe sequence repeated at step %v", i)
		}
		seen[h] = true
	}
	//同一个键派生出的序列必须完全可复现
	h1 := hashKey([]byte("abc"))
	h2 := hashKey([]byte("abc"))
	for i := 0; i < 10; i++ {
		h1 = nextProbeHash(h1)
		h2 = nextProbeHash(h2)
		if h1 != h2 {
			t.Fatal("probe sequence is not reproducible")
		}
	}
}

// TestCheckDataPos 位置索引的uint32上限校验
func TestCheckDataPos(t *testing.T) {
	if got := checkDataPos(0); got != 0 {
		t.Fatalf("unexpected value; got %v want 0", got)
	}
	if got := checkDataPos(123456); got != 123456 {
		t.Fatalf("unexpected value; got %v want 123456", got)
	}
	assertPanic(t, "data size is too large", func() { checkDataPos(int(maxOffset) + 1) })
	assertPanic(t, "data size is too large", func() { checkDataPos(-1) })
}

// TestDefaultTempFileInSystemTempDir 未指定文件名时临时文件不能落在进程当前工作目录
func TestDefaultTempFileInSystemTempDir(t *testing.T) {
	m := NewDefault()
	name := m.tempFileName
	if !strings.HasSuffix(name, tempFileSuffix) {
		t.Fatalf("unexpected temp file name; got %q", name)
	}
	if !strings.HasPrefix(filepath.Base(name), "noGcStaticMap-") {
		t.Fatalf("unexpected temp file name; got %q", name)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(filepath.Dir(name), filepath.Clean(cwd)) {
		t.Fatalf("temp file should not be created in the working directory; got %q", name)
	}
	m.SetString("k", "v")
	m.SetFinished()
	if fileExist(name) {
		t.Fatalf("temp file should be removed after SetFinished; got %q", name)
	}
}

// TestSameTempName 两个实例使用同一个文件名时必须明确报错，且不能删掉对方的文件
func TestSameTempName(t *testing.T) {
	name := tempName(t)
	m1 := NewInt(name)
	defer m1.SetFinished()
	//同名不再报错：第二个实例会拿到一个带时间戳的名字
	m2 := NewInt(name)
	defer m2.SetFinished()
	if m1.tempFileName == m2.tempFileName {
		t.Fatalf("two maps share the same temp file: %v", m1.tempFileName)
	}
	if !strings.HasPrefix(filepath.Base(m2.tempFileName), filepath.Base(name)+"-") ||
		!strings.HasSuffix(m2.tempFileName, tempFileSuffix) {
		t.Fatalf("unexpected bumped temp file name: %v", m2.tempFileName)
	}
	if !fileExist(name + tempFileSuffix) {
		t.Fatal("the temp file of the first map was renamed or deleted")
	}
	//两个实例的数据必须互不干扰
	m1.SetString(1, "from-m1")
	m2.SetString(1, "from-m2")
	m1.SetFinished()
	m2.SetFinished()
	if v, exist := m1.GetString(1); !exist || v != "from-m1" {
		t.Fatalf("unexpected value of m1; got %q exist=%v", v, exist)
	}
	if v, exist := m2.GetString(1); !exist || v != "from-m2" {
		t.Fatalf("unexpected value of m2; got %q exist=%v", v, exist)
	}
}

// TestTempFilePreExistingAutoRename 生成指定文件名时若磁盘上已有同名文件（如上次异常退出遗留），
// 不能启动即挂，也不能删改那个旧文件，而是换一个带时间戳的名字
func TestTempFilePreExistingAutoRename(t *testing.T) {
	name := tempName(t)
	oldName := name + tempFileSuffix
	const oldContent = "left over by previous run"
	if err := os.WriteFile(oldName, []byte(oldContent), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewInt(name)
	defer m.SetFinished()
	if m.tempFileName == oldName {
		t.Fatalf("the pre-existing file was reused: %v", m.tempFileName)
	}
	//旧文件的内容不能被改动
	if b, err := os.ReadFile(oldName); err != nil || string(b) != oldContent {
		t.Fatalf("the pre-existing file was modified; content=%q err=%v", string(b), err)
	}
	m.SetString(1, "v1")
	m.SetFinished()
	if v, exist := m.GetString(1); !exist || v != "v1" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	//新文件用完即删，旧文件仍在
	if fileExist(m.tempFileName) {
		t.Fatalf("the bumped temp file should be removed after SetFinished: %v", m.tempFileName)
	}
	if !fileExist(oldName) {
		t.Fatal("the pre-existing file should be kept untouched")
	}
}

// TestSetFinishedAfterChdir 创建临时文件后切换工作目录，SetFinished仍然要能读到数据
func TestSetFinishedAfterChdir(t *testing.T) {
	base := t.TempDir()
	m := NewInt(filepath.Join(base, "chdir"))
	m.SetString(1, "v1")
	sub := filepath.Join(base, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	m.SetFinished()
	if v, exist := m.GetString(1); !exist || v != "v1" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
}

// TestSetFinishedFailureKeepsUsableState 加载失败后不能留下"已完成存储"的假状态
func TestSetFinishedFailureKeepsUsableState(t *testing.T) {
	base := t.TempDir()
	m := NewInt(filepath.Join(base, "broken"))
	m.SetString(1, "v1")
	//模拟临时文件在加载前丢失
	m.tempFileName = filepath.Join(base, "notExist") + tempFileSuffix
	assertPanic(t, "", func() { m.SetFinished() })
	if m.setFinished {
		t.Fatal("setFinished should not be true after a failed SetFinished")
	}
	//此时查询应该得到明确的状态错误，而不是数组越界
	assertPanic(t, "can't Get before SetFinished", func() { m.Get(1) })
}

// TestSetFinishedIdempotent SetFinished可以重复调用
func TestSetFinishedIdempotent(t *testing.T) {
	m := NewInt(tempName(t))
	m.SetString(1, "v1")
	m.SetFinished()
	m.SetFinished()
	m.SetFinished()
	if v, exist := m.GetString(1); !exist || v != "v1" {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if m.Len() != 1 {
		t.Fatalf("unexpected len; got %v want 1", m.Len())
	}
}

// TestLenAfterPanic 写入失败后计数不能虚高
func TestLenAfterPanic(t *testing.T) {
	m := NewInt(tempName(t))
	for _, k := range []int{1, 2, 3} {
		m.Set(k, []byte("v"))
	}
	assertPanic(t, "for twice", func() { m.Set(1, []byte("v")) })
	assertPanic(t, "too long", func() { m.Set(4, make([]byte, maxKVLen+1)) })
	m.SetFinished()
	assertPanic(t, "can't Set after SetFinished", func() { m.Set(5, []byte("v")) })
	if m.Len() != 3 {
		t.Fatalf("unexpected len; got %v want 3", m.Len())
	}
}

// TestEmptyMap 空map加载后查询
func TestEmptyMap(t *testing.T) {
	m := NewInt(tempName(t))
	m.SetFinished()
	if v, exist := m.Get(1); exist || v != nil {
		t.Fatalf("unexpected value; got %q exist=%v", v, exist)
	}
	if m.Len() != 0 {
		t.Fatalf("unexpected len; got %v want 0", m.Len())
	}
}

// TestManyKeysIntegrity 大批量键值写入后逐条校验，确保索引位置没有串位
func TestManyKeysIntegrity(t *testing.T) {
	const n = 100000
	m := NewInt(tempName(t))
	for i := 0; i < n; i++ {
		m.SetString(i, "v"+strconv.Itoa(i))
	}
	m.SetFinished()
	for i := 0; i < n; i++ {
		if v, exist := m.GetString(i); !exist || v != "v"+strconv.Itoa(i) {
			t.Fatalf("unexpected value for key %v; got %q exist=%v", i, v, exist)
		}
	}
	if m.Len() != n {
		t.Fatalf("unexpected len; got %v want %v", m.Len(), n)
	}
}
