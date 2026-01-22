# fnOS FUSE 挂载与上传行为（ClearVault 兼容性记录）

本文记录在飞牛 fnOS 上，文件管理器/上传服务通过 FUSE 挂载点进行文件上传时的典型行为序列，以及 ClearVault 为兼容这些行为所做的实现要点与排障方法。

## 目标与约束

- 目标：挂载后普通用户/文件管理器可访问目录并能上传文件。
- 目标：大文件上传采用流式上传（不落本地临时文件）。
- 约束：对象存储语义下不支持随机写（只支持顺序写）。

## 挂载生命周期（fnOS）

### root 挂载与 allow_other

在 fnOS 场景中，Web 文件管理器进程通常不是以 root 身份访问挂载点。FUSE 默认权限模型下，不启用 allow_other 时仅允许挂载用户访问，因此需要：

- 使用 `allow_other,default_permissions` 挂载，使普通用户/文件管理器可访问挂载点。
- 通过 `FUSE_UID/FUSE_GID` 将文件属性映射为挂载点目录的 owner（便于文件管理器显示与权限判断）。

实现位置：

- 挂载命令：cmd/clearvault/mount_fuse.go
- 文件属性：internal/fuse/fs.go（Getattr 填充 uid/gid 与 mode）

### 启动延迟挂载与停止卸载

fnOS 应用启动较早时，系统侧授权路径/环境变量可能尚未准备完全。为提升稳定性：

- 应用启动后延迟一段时间（默认 5 秒）再触发自动挂载。
- 停止应用时优先停止挂载进程，确保挂载点卸载，避免残留进程。

实现位置：deploy/fnos/cmd/main

## fnOS 上传行为（基于日志的可重复序列）

fnOS 文件管理器上传时常使用“临时文件名 + 重命名”的模式。典型临时名格式：

- `⟨filename⟩.~#0`（示例：`favicon.svg.~#0`）

典型调用序列（同一路径可能重复若干次）：

1. `Getattr(final)`：对最终文件名做存在性检查（通常初次为 ENOENT）。
2. `Create(temp)`：创建临时名文件（flags 常见为 `WRONLY|CREAT|EXCL`）。
3. `Release(temp)`：在 0 字节阶段关闭句柄（预创建/预验证阶段）。
4. `Open(temp, WRONLY)`：再次以写方式打开临时名，进入真实写入阶段。
5. `Write(temp)`：多次写入，写入块大小常见为 4096 字节；offset 严格递增，表现为顺序写入。
6. `Rename(temp → final)`：可能发生在写入过程中（上传尚未结束时就尝试重命名）。
7. `Release(temp)`：上传结束关闭句柄。

日志特征：

- 顺序写：`ofst` 递增且 `ofst == expected`。
- 若出现随机写：通常会看到 `non-seq` 并返回 `EOPNOTSUPP`。

## ClearVault 兼容策略（实现要点）

### 1) 顺序写 + 流式上传

- `Write()` 收到数据后直接写入 `io.PipeWriter`，后台 goroutine 将 pipe 的 reader 传给 `Proxy.UploadFile(..., size=-1)`，实现边写边上传。
- 若出现非顺序写（offset 不连续），直接返回不支持，避免对象存储语义下的错误合并。

实现位置：internal/fuse/fs.go（Write/Release）

### 2) 0 字节临时文件的占位（避免远端生成 0B 对象）

fnOS 预创建阶段可能只创建 0B 的 `*.~#0` 临时文件。对于这类路径：

- 在 `Release`（0B 且临时名）阶段不上传远端 0B 对象，而是保存“内存占位符”，用于兼容后续第二阶段真实写入。

实现位置：

- internal/fuse/fs.go（Release 对 `.~#` 路径调用 SavePlaceholder）
- internal/proxy/proxy.go（SavePlaceholder / HasPlaceholder）

### 3) 占位符可见性（避免二次 open 时报 ENOENT）

由于占位符默认仅存在于内存中，若 FUSE 在 `Getattr/Open/Access` 只依赖元数据存储，会导致：

- 上传服务在第二阶段 `open()` 时看到 ENOENT（errno=2），从而返回 500。

因此需要：

- 在 `Getattr/Open/Access` 中将 placeholder 视为“存在的 0B 文件”，保证上传流程进入真实写入阶段。

实现位置：internal/fuse/fs.go（Getattr/Open/Access）

### 4) 写入中 Rename 的时序问题（确保最终文件名正确）

fnOS 可能在上传尚未结束时调用 `Rename(temp → final)`。如果此时立即对元数据层做 Rename，常见结果是：

- Rename 发生时真实元数据尚未落盘 → rename 时机不对 → 最终残留临时名 `*.~#0`。

兼容方案：

- 当 `Rename(old,new)` 命中“正在写入中的句柄”时，先记录 `renameTo` 并返回成功（延迟执行）。
- 在 `Release` 等上传完成后，再执行一次最终 `Rename(uploadPath → renameTo)`，确保元数据最终落到正式名。
- 同时在上传期间 `Getattr(new)` 需要能返回合成 stat，避免 rename 后立即 getattr 新路径持续 ENOENT。

实现位置：internal/fuse/fs.go（Rename/Release/Getattr/pendingSize）

### 5) 写入日志降噪（便于分析）

fnOS 上传服务常以 4KB 粒度调用 Write。逐次打印会导致日志爆炸，因此采用采样策略：

- 首次写入必打。
- 之后按累计字节数（例如每 1MiB）或时间间隔（例如每 2 秒）打印进度。
- 错误（非顺序写/EBADF/EIO）保持逐次完整输出。

实现位置：internal/fuse/fs.go（Write）

## 排障建议（如何对齐 fnOS 上传日志）

对照以下关键日志即可还原上传状态机：

- `FUSE Create/Open/Write/Release/Rename`：判断 fnOS 使用的 open flags、是否顺序写、rename 发生时机。
- `Proxy: SavePlaceholder / HasPlaceholder / RemoveAll / RenameFile`：判断占位符是否参与了流程以及是否被正确迁移/清理。
- fnOS 上传服务日志里的 `open file error: 2`：通常对应 FUSE 侧 ENOENT，需要检查 placeholder 可见性与 Getattr/Open 逻辑。

