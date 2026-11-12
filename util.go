// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// tempFileSuffix 临时文件名后缀
	tempFileSuffix = ".NoGcStaticMap"
	// tempFileBufSize 临时文件的写缓存大小
	tempFileBufSize = 40960
	// maxKVLen 默认类型与整型map中值允许的最大长度
	maxKVLen = 65535
	// maxOffset 位置索引(uint32)能表示的最大偏移，即单个map的数据区上限为4GiB-1
	maxOffset int64 = 1<<32 - 1
)

// createTempFile 创建用于读写的临时文件。
// 1)未指定名称时，交由标准库在系统临时目录下创建唯一文件名，既不落在进程当前工作目录，也不会与其它实例撞名；
// 2)指定名称时，使用 O_EXCL 独占创建，同名文件已存在时直接报错，
// 绝不采用"先删除同名文件再创建"的写法，避免误删其它实例正在使用的文件。
func createTempFile(args ...string) (tempFileName string, tempFile *os.File, bw *bufio.Writer) {
	if len(args) > 0 && args[0] != "" {
		name, err := filepath.Abs(args[0] + tempFileSuffix)
		haserrPanic(err)
		tempFile, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			panic(fmt.Sprintf("can't create temp file %v, maybe an other map is using the same name, error:%v", name, err))
		}
		return name, tempFile, bufio.NewWriterSize(tempFile, tempFileBufSize)
	}
	tempFile, err := os.CreateTemp("", "noGcStaticMap-*"+tempFileSuffix)
	haserrPanic(err)
	return tempFile.Name(), tempFile, bufio.NewWriterSize(tempFile, tempFileBufSize)
}

// fileExist 判断文件或者目录是否确定存在。
// 只有os.Stat成功才算存在；路径语法错误、权限不足等"无法判断"的情况一律返回false，
// 而不是把不确定当作存在。
func fileExist(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// cleanupTempFile 关闭临时文件并清理残留，由 SetFinished 与 GC 终结器共用
func cleanupTempFile(tempFile *os.File, tempFileName string) {
	if tempFile != nil {
		_ = tempFile.Close()
	}
	if tempFileName != "" {
		_ = os.Remove(tempFileName)
	}
}

// checkDataPos 校验位置索引是否超出 uint32 的表示范围。
// 原实现直接做 uint32 转换，数据区超过4GiB后索引会被静默截断，导致查询错乱甚至越界。
func checkDataPos(dataBeginPos int) uint32 {
	if dataBeginPos < 0 || int64(dataBeginPos) > maxOffset {
		panic(fmt.Sprintf("data size is too large, the offset must be <= %v bytes(uint32 limit), current offset is %v", uint64(maxOffset), dataBeginPos))
	}
	return uint32(dataBeginPos)
}

// checkBytesLen 校验单个键或值的长度是否超出 uint32 的表示范围
func checkBytesLen(n int, what string) {
	if int64(n) > maxOffset {
		panic(fmt.Sprintf("%v is too long, the maximum is %v", what, uint64(maxOffset)))
	}
}

// haserrPanic 判断有无错误,并返回true或者false,有错误时调用panic退出
func haserrPanic(err error) bool {
	if err != nil {
		_, file, line, _ := runtime.Caller(1)
		file = file[strings.LastIndex(file, "/")+1:]
		panic(fmt.Sprintf("%v,第%v行,错误类型:%v", file, line, err))
	}
	return false
}
