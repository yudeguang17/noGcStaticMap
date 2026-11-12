// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// tempName 每个测试用独立的临时文件名，避免测试之间以及与本机其它实例互相干扰
func tempName(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "mapForTest")
}

// assertPanic 断言 f 一定 panic，且 panic 内容包含 want（want 为空时不校验内容）
func assertPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("want panic containing %q, but got no panic", want)
		}
		if want != "" && !strings.Contains(fmt.Sprint(r), want) {
			t.Fatalf("want panic containing %q, but got %v", want, r)
		}
	}()
	f()
}
