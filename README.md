# noGcStaticMapV2

`noGcStaticMapV2` 是 [noGcStaticMap](https://github.com/yudeguang17/noGcStaticMap) 的完整重写版：对外 API 与原项目**完全一致**，内部实现改为**纯标准库**（哈希用 `hash/fnv` 的 FNV-1a 64 位，不引入任何第三方依赖），并在保持原有静态模型的前提下修正了原实现的若干缺陷。

静态模型没有变化：键值在加载阶段追加写入临时文件，`SetFinished` 后整体读入内存，此后只读、不再允许增删改。

```go
var m = noGcStaticMap.NewDefault()
m.Set([]byte("key"), []byte("value"))
m.SetFinished()
v, exist := m.GetString("key")
```

## 1. 与原项目的差异

| 项目 | 原实现 | 本实现 |
| --- | --- | --- |
| 哈希 | 第三方 `github.com/cespare/xxhash` | 标准库 `hash/fnv` 的 FNV-1a 64 位 |
| 依赖 | xxhash + `github.com/yudeguang17/iox` | 无第三方依赖 |
| 索引表 | 任意/超大类型用两张 map（主表 + 哈希冲突表） | **只有一张 map** |
| 重复键 | 任意/超大类型第二次写入被静默丢弃，第三次才报错 | 立即报错 |
| `Len()` | 校验前自增，panic 后计数虚高 | 写入成功后才自增 |
| `SetFinished` | 不可重复调用；失败后留下"已完成"的假状态 | 幂等；失败不改变状态 |
| 位置索引 | 直接 `uint32` 转换，超 4GiB 静默截断 | 转换前校验并明确报错 |
| 临时文件 | 相对当前目录、会删除同名文件、名字取自纳秒时间戳 | 系统临时目录 + `O_EXCL` 独占创建，同名时追加纳秒时间戳自动避让，绝不删他人文件 |
| 加载内存 | `ReadFile` 后再复制一份，峰值双倍 | 直接接管读出的切片 |
| 末尾分隔符 | 被算进最后一个字段（`"1\`a\`b\`c\`"` 的 `Col4` 变成 `"c\`"`） | 正确收尾，不污染字段 |

导出的类型名、函数名、方法名与签名全部保持不变，可以直接替换 import 路径使用。

## 2. 为什么只需要一张 map

静态 map 有两个性质可以利用：

1. **没有删除操作**，写入阶段只会让索引变多，已占用的槽位永远保持占用；
2. 因此每个键都可以拥有一条**由自身唯一确定的探测序列**：第 0 个槽位是 `FNV-1a(k)`，其后由 `splitmix64` 逐步派生。

于是冲突处理可以完全在一张 `map[uint64]uint32` 内完成：

- 写入：从 0 号槽位开始试探，遇到的第一个空闲槽位就是该键的位置。试探途中若发现某个已占用槽位里存的是同一个键，说明是重复写入，立即报错。
- 查询：从 0 号槽位开始试探，比对记录中的键；一旦遇到空闲槽位就可以断定键不存在（因为已占用的槽位不会变回空闲）。

原实现里的第二张 `mapForHashCollision`（键为冲突键的字符串）因此被删除，`int`/`uint32` 类型本来就是键即索引，同样只有一张表。**探测只在真正发生哈希冲突时才会走到第 2 个槽位**，FNV-1a 64 位在千万级数据下冲突概率约在 10⁻⁶ 量级，常规路径始终只有一次 map 查找。

记录布局与原实现保持一致（任意类型：2 字节 K 长度 + 2 字节 V 长度 + K + V；超大类型：4 + 4 字节小端长度），所以没有额外的每记录开销。

## 3. 类型与接口

| 类型 | 构造函数 | 键类型 | 值长度上限 |
| --- | --- | --- | --- |
| `NoGcStaticMapAny` | `NewDefault(tempFileName ...string)` | `[]byte` / `string` | 65535 |
| `NoGcStaticMapInt` | `NewInt(tempFileName ...string)` | `int` | 65535 |
| `NoGcStaticMapUint32` | `NewUint32(tempFileName ...string)` | `uint32` | 65535 |
| `NoGcStaticMapHuge` | `NewHuge(tempFileName ...string)` | `[]byte` / `string` | 不受 65535 限制 |
| `NoGcStaticMapBidirect` | `NewBidirect(tempFileName ...string)` | `uint32` ↔ `string` 双向 | 65535 |

前四种类型都提供：`Set`、`SetString`、`SetFinished`、`Get`、`GetString`、`GetUnsafe`、`GetStringUnsafe`、`GetDataBeginPosOfKVPair`、`GetValFromDataBeginPosOfKVPairUnSafe`、`Len`。

`GetUnsafe` / `GetStringUnsafe` 返回的是内存数据的**引用**而非副本，不要修改，也不要长期持有（一个短字符串会把整块数据钉在堆上）。空值时四种类型统一返回 `nil`（`exist` 仍为 `true`），与 `Get` / `GetString` 的语义一致，不要用 `v != nil` 判断"是否存在"。

`GetValFromDataBeginPosOfKVPairUnSafe` 也做了状态校验：在 `SetFinished` 之前调用会 panic `can't Get before SetFinished`，而不是数组越界。

`convert_help.go` 中的 `BytesToStruct`、`BytesToSlice`、`StructToStr`、`SliceToStr`、`JoinInterface`、`UnsafeBytesToString`、`UnsafeStringToBytes`、`SplitSep`、`NoGcStructExample` 也保持原签名，其中长度前缀为 uint32 小端序，改用标准库实现，不再依赖 `iox`。

`JoinInterface` 的每个元素严格按 `fmt.Sprint` 的规则转换：同时实现 `error` 与 `fmt.Stringer` 的类型优先用 `Error()`；`String()` 内部 panic 会被 fmt 捕获成 `%!v(PANIC=String method: ...)`，而不是把 panic 抛给调用方。目前只有 `string`、`int`、`int64`、`uint32`、`nil` 走零分配快路径（`StructToStr` 实际只用到 `int` 与 `string`），其余类型一律交给 `fmt.Sprint`，因此输出与逐元素 `fmt.Sprint` 拼接完全一致。

### 3.1 双向索引类型 NoGcStaticMapBidirect

既可以用 id 查字符串，也可以用字符串查 id：

```go
m := noGcStaticMap.NewBidirect()
m.Set(1001, "alice")          //id 或字符串任一重复都会 panic
m.SetFinished()
id, ok := m.GetID("alice")    // -> 1001, true
str, ok := m.GetValue(1001)   // -> "alice", true
unsafeStr, ok := m.GetValueUnsafe(1001) //零拷贝，引用内存数据
```

写入有三个入口：`Set(id, str string)`、`SetBytes(id, []byte)` 与 `SetSafe(id, str) error`。前两个失败时 panic，`SetSafe` 则把失败原因作为 error 返回，适合"读一批数据、跳过重复项继续处理"不能中断的场景：

```go
err := m.SetSafe(1001, "alice")
switch {
case err == nil:
case errors.Is(err, noGcStaticMap.ErrDuplicateID):   //id 已存在
case errors.Is(err, noGcStaticMap.ErrDuplicateStr):  //字符串已存在
case errors.Is(err, noGcStaticMap.ErrStrTooLong):    //超过 65535
case errors.Is(err, noGcStaticMap.ErrSetFinished):   //SetFinished 之后写入
case errors.Is(err, noGcStaticMap.ErrDataTooLarge):  //数据区超过 4GiB 上限
}
```

`SetSafe` 与 `Set`/`SetBytes` 写入同一份数据与索引，混用时互相能识别重复；失败时数据区、索引与 `Len` 均保持原样，之后仍可继续写入。三个入口都收敛到同一个吃 `[]byte` 的内部实现，构建期用字节切片灌数据时不会额外复制一份字符串，也没有任何字符串常驻内存。

重复字符串时 panic 消息只带前 64 字节并标注总长度（截断点落在 rune 边界上），避免几十 KB 的消息把整段业务数据写进日志或崩溃上报。

**为什么不用两个静态 map 拼**：这里只有一份数据区，每条记录只写一次（4 字节 id + 2 字节长度 + 字符串），两张索引 `map[uint32]uint32`（id→位置）与 `map[uint64]uint32`（字符串哈希→位置）都指向同一条记录，字符串不会被存两遍。字符串索引沿用与其它类型相同的 FNV-1a + 键派生探测序列，所以写到重复字符串时能在写入阶段立刻判定并 panic，不需要额外的冲突表。

同一台机器、20 万条（字符串平均 22.4 字节）实测常驻堆占用：

| 方案 | 数据区 | 常驻堆 |
| --- | --- | --- |
| 单个双向索引 map | 5.43 MB（字符串只存一份） | **12.30 MB** |
| `NewUint32` + `NewDefault` 两个独立静态 map | 11.27 MB（字符串存两份，另存 id 文本） | 17.66 MB |

两种索引本身的固定开销一样（`map[uint32]uint32` 约 2.32 MB、`map[uint64]uint32` 约 4.51 MB），差异全部来自字符串是否重复存储，因此条目越多、字符串越长，节省越明显；本次规模约省 30%。

## 4. 使用约束

- **写入阶段**：单 goroutine 调用 `Set`，`Set` 不是并发安全的（并发写会触发 `fatal error: concurrent map writes`）。
- **查询阶段**：`SetFinished` 之后可以多 goroutine 并发只读，测试覆盖 8 协程并发取值。
- **数据区上限**：位置索引是 `uint32`，单个 map 的数据区上限为 4GiB-1 字节，超出会在写入时明确 panic（原实现会静默截断导致查询错乱）。
- **临时文件**：不指定文件名时创建在系统临时目录；指定文件名时使用独占创建，**同名文件已存在不会报错，也不会删改对方**，而是在基础名后追加"-"+纳秒时间戳换一个名字，例如 `NewBidirect("no_gc_map_bi")` 在冲突时实际使用 `no_gc_map_bi-1762912345123456789.NoGcStaticMap`。因此上次异常退出遗留的同名文件、或另一个实例正在使用的同名文件，都不会让程序启动即 panic。
- **遗留文件**：本包只清理自己在 `SetFinished` 后创建的那个临时文件（未调用 `SetFinished` 就丢弃时由 GC 终结器兜底），不会主动清理磁盘上其它同名/陈旧文件；如果进程被强杀，可能留下 `*.NoGcStaticMap` 残留文件，需要由调用方按需清理。
- **写入中途失败不可恢复**：`Set` 内部按"写失败即 panic"处理，`bufio.Writer` 或磁盘写入出错时直接 panic。这种情况下临时文件里可能残留半条记录，而内存中的游标没有前进，两者已经错位；即使 recover 后继续 `Set` 也会串位。因此不要把 `Set` 的 panic 当作可恢复错误——让它冒泡退出进程，或者丢弃这个 map 重新构建。
- **`SplitSep`**：这是包级变量，`BytesToStruct` / `StructToStr` / `checkStr` 在运行期读取它。只允许在任何并发使用之前（一般是程序初始化阶段）设置，运行期并发修改会构成数据竞争。

## 5. 本机实测（Go 1.27.3 / Windows / 200ms 基准）

同一工作负载下与原实现（`xxhash` + 双 map）对比，`-benchmem` 口径：

| 基准 | 原实现 | 本实现 |
| --- | --- | --- |
| Set（int 键） | 329.7 ns/op | 224.9 ns/op |
| Set（[]byte 键） | 371.4 ns/op | 297.1 ns/op |
| Get（int 键） | 24.75 ns/op，1 alloc | 23.23 ns/op，1 alloc |
| GetUnsafe（int 键） | 11.49 ns/op，0 alloc | 11.96 ns/op，0 alloc |
| FNV-1a 哈希 | — | 14.5 ~ 18.9 ns/op，0 alloc |
| `SliceToStr`+`BytesToSlice`（100 条往返） | — | 54.6 us / 787 allocs → 31.2 us / 607 allocs |
| `GetString`（int 键转 string） | — | 21.9 ns/op，3 B/op，1 alloc |
| `BytesToStruct`（4 字段解析） | — | 130.3 ns/op，40 B/op，5 allocs |

写入变快主要来自把逐字节 `WriteByte` 改成整段写入；查询路径与原实现基本持平。`SliceToStr`+`BytesToSlice` 那行的区间来自两次不同时间点的测量，字符串拼接改造后分配次数与耗时都明显下降；`GetString` 与 `BytesToStruct` 只记录本实现的实测值（原实现没有对应口径的同名基准）。

在此基础上又做了一轮针对性优化（同一台机器、`-benchtime=300ms` 口径，对比的是本工程优化前/后）：

| 路径（值 1KB） | 优化前 | 优化后 |
| --- | --- | --- |
| `GetUnsafe`（any 类型） | 170.9 ns/op，1024 B/op，1 alloc | 19.0 ns/op，0 B/op，0 alloc |
| `GetDataBeginPosOfKVPair`（any 类型） | 152.0 ns/op，1024 B/op，1 alloc | 17.4 ns/op，0 B/op，0 alloc |

原因：这两个接口只要"键在数据区的位置"，原来却借用了会复制完整 value 的 `read`，命中时白白复制一份值。现在改用 `sameKeyAtData`，只按长度前缀找到键并比较字节，不再触碰 value；探测到哈希冲突时也不复制值。`huge` 类型同样处理。

双向索引类型的基准（`-benchmem -benchtime=200ms`，1 万条预置数据，键为 `v0`…`v9999`）：

| 操作 | 耗时 | 分配 |
| --- | --- | --- |
| `Set`（含 id 与字符串双向建索引） | 428.5 ns/op | 2 allocs（其中 1 次来自基准内部的 key 字符串构造） |
| `SetBytes`（`[]byte` 灌数据，零字符串复制） | 446.9 ns/op | 1 alloc（map 扩容摊销） |
| `GetID`（字符串查 id） | 26.3 ns/op | 0 |
| `GetValue`（id 查字符串，复制一份） | 19.8 ns/op | 1 alloc（复制的那份字符串） |
| `GetValueUnsafe`（id 查字符串，零拷贝） | 9.5 ns/op | 0 |

`GetValue` 的分配来自把内存中的字符串复制成独立 string，这是安全版本的固有代价；如果不需要独立副本就用 `GetValueUnsafe`。基准里把取到的值写入一个全局变量，避免编译器把复制过程优化掉导致分配数被测成 0。

## 6. 测试

`go test ./...` 共 76 个用例，覆盖：

- 双向索引类型：与官方双 map 逐条对比、双向互查、重复 id / 重复字符串 panic 且计数不变、空字符串、id 边界值、字符串 65535/65536 边界、`SetFinished` 前后状态机、探测冲突下的重复检测、并发双向只读、`Set` 与 `SetBytes` 结果等价、重复字符串 panic 消息的截断与 rune 边界；
- `SetSafe`：成功路径与 `Set` 等价、四类失败分别能用 `errors.Is` 判定且不 panic、失败后 `Len` 不变并可继续写入、与 `Set` 混用时互相识别重复；

- 四种类型与原 `map` 的逐键对比（各 1 万条），空键、空值、空 map、键类型边界值；
- 重复键立即报错、`Len` 计数、`SetFinished` 幂等与失败后状态、`SetFinished` 后禁止写入；
- 人为构造哈希冲突验证探测序列（读取、重复检测、不存在键返回 false）；
- 值长度 65535/65536 边界、位置索引 uint32 上限；
- 临时文件：默认落点不在工作目录、同名自动避让且不误删/不改动旧文件、切换工作目录后仍能加载；
- 序列化：末尾分隔符、多字节/空分隔符、字段过多、非数字首段、缓存池边界长度、切片往返、损坏数据报错；
- 10 万条键值完整性、8 协程并发只读、零拷贝转换；
- 四类空值 `GetUnsafe` 的 nil 语义一致、`SetFinished` 前用位置取值报状态错误、`fileExist` 的"无法判断不算存在"、`JoinInterface` 输出与逐元素 `fmt.Sprint` 完全一致（含同时实现 `error` 与 `fmt.Stringer` 的类型、以及 `String()` 内部 panic 的类型）；
- 只要位置的查询路径（`GetUnsafe`、`GetDataBeginPosOfKVPair`）对 1KB 值断言 0 分配、`JoinInterface` 对每个元素只做一次转换（纯 `fmt.Stringer` 元素只调用一次 `String()`）。

## 7. License

MIT，见 `LICENSE`。