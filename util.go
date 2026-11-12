// Copyright 2022 rateLimit Author(https://github.com/yudeguang17/noGcStaticMap). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/yudeguang17/noGcStaticMap.
package noGcStaticMap

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// tempFileSuffix 临时文件名后缀
	tempFileSuffix = ".NoGcStaticMap"
	// tempFileMaxRetry 同名冲突时最多尝试多少个带时间戳的新名字
	tempFileMaxRetry = 64
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
		tempFile, err = createTempFileNoClash(name)
		haserrPanic(err)
		return tempFile.Name(), tempFile, bufio.NewWriterSize(tempFile, tempFileBufSize)
	}
	tempFile, err := os.CreateTemp("", "noGcStaticMap-*"+tempFileSuffix)
	haserrPanic(err)
	return tempFile.Name(), tempFile, bufio.NewWriterSize(tempFile, tempFileBufSize)
}

// createTempFileNoClash 按给定名字独占创建临时文件。
// 若同名文件已经存在（例如程序上次异常退出留下的、或其它实例正在使用的），
// 不会删除对方，而是在基础名后追加"-"+纳秒时间戳继续尝试，
// 直到拿到一个空闲名字为止；最终名字形如 no_gc_map_bi-1762912345123456789.NoGcStaticMap。
func createTempFileNoClash(baseName string) (*os.File, error) {
	f, err := openExclusive(baseName)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	stem := strings.TrimSuffix(baseName, tempFileSuffix)
	for i := 0; i < tempFileMaxRetry; i++ {
		name := stem + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + tempFileSuffix
		f, err = openExclusive(name)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		//同一纳秒内再次撞名：让出时间片后换一个新的时间戳重试
		time.Sleep(time.Microsecond)
	}
	return nil, fmt.Errorf("can't create temp file for %v after %v attempts", baseName, tempFileMaxRetry)
}

// openExclusive 以"不存在才创建"的方式打开文件，同名时不覆盖也不删除
func openExclusive(name string) (*os.File, error) {
	return os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
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
