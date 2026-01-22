# ClearVault 技术实现文档

本文档详细介绍 ClearVault 的技术架构和实现细节。

## 目录

- [架构概览](#架构概览)
- [加密系统](#加密系统)
- [元数据管理](#元数据管理)
- [WebDAV 协议实现](#webdav-协议实现)
- [文件操作流程](#文件操作流程)
- [RaiDrive 兼容性](#raidrive-兼容性)
- [Windows 文件锁定处理](#windows-文件锁定处理)
- [简单分享功能](#简单分享功能)
- [性能优化](#性能优化)

## 架构概览

### 系统架构图

```
┌─────────────────┐
│  WebDAV Client  │ (RaiDrive, Windows Explorer, etc.)
└────────┬────────┘
         │ WebDAV Protocol
         ▼
┌─────────────────────────────────────────┐
│         ClearVault Proxy Server         │
│  ┌─────────────┐      ┌──────────────┐ │
│  │   WebDAV    │◄────►│    Proxy     │ │
│  │   Handler   │      │    Layer     │ │
│  └─────────────┘      └──────┬───────┘ │
│                              │          │
│  ┌─────────────┐      ┌──────▼───────┐ │
│  │  Metadata   │◄────►│   Crypto     │ │
│  │   Manager   │      │   Engine     │ │
│  └─────────────┘      └──────────────┘ │
└────────┬────────────────────────────────┘
         │ WebDAV Protocol (Encrypted)
         ▼
┌─────────────────┐
│  Remote WebDAV  │ (Nextcloud, Nutstore, etc.)
│     Storage     │
└─────────────────┘
```

### 核心组件

1. **WebDAV Handler** (`internal/webdav`)
   - 处理 WebDAV 协议请求
   - 实现文件系统接口
   - 路径规范化和验证

2. **Proxy Layer** (`internal/proxy`)
   - 协调加密/解密操作
   - 管理文件上传/下载流程
   - 处理元数据和远端存储的交互

3. **Crypto Engine** (`internal/crypto`)
   - AES-256-GCM 加密/解密
   - 密钥派生和管理
   - 流式加密支持

4. **Metadata Manager** (`internal/metadata`)
   - 文件元数据存储和检索
   - 使用本地文件系统（JSON 格式）
   - 路径到远程文件名的映射

5. **Remote Storage Backend** (`internal/remote`)
   - **WebDAV** (`internal/remote/webdav`)：标准 WebDAV 协议支持
   - **S3** (`internal/remote/s3`)：AWS S3 及兼容存储支持
   - **Local** (`internal/remote/local`)：本地文件系统支持（适用于 NAS 挂载等场景）

## 加密系统

### 加密算法

- **对称加密**：AES-256-GCM
  - 密钥长度：256 位（32 字节）
  - 认证加密：提供机密性和完整性
  - Nonce：96 位（12 字节），每个文件唯一

- **密钥派生**：PBKDF2
  - 从主密钥派生文件加密密钥（FEK）
  - 每个文件使用独立的随机 FEK

### 加密流程

#### 文件上传加密

```
原始文件
    │
    ▼
┌─────────────────────┐
│ 生成随机 FEK (32B)  │
│ 生成随机 Salt (12B) │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  使用主密钥加密 FEK │
│  (AES-256-GCM)      │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  使用 FEK 加密文件  │
│  (流式 AES-GCM)     │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 生成随机远程文件名  │
│ (SHA-256 Hash)      │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 上传到远端 WebDAV   │
└─────────────────────┘
           │
           ▼
┌─────────────────────┐
│ 保存元数据:         │
│ - 原始路径          │
│ - 远程文件名        │
│ - 加密的 FEK        │
│ - Salt              │
│ - 文件大小          │
└─────────────────────┘
```

#### 文件下载解密

```
客户端请求
    │
    ▼
┌─────────────────────┐
│ 从元数据获取:       │
│ - 远程文件名        │
│ - 加密的 FEK        │
│ - Salt              │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 使用主密钥解密 FEK  │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 从远端下载加密文件  │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 使用 FEK 解密文件   │
│ (流式 AES-GCM)      │
└──────────┬──────────┘
           │
           ▼
      返回给客户端
```

### 文件名加密

远程文件名生成算法：

```go
remoteName = SHA256(randomBytes(32))
```

- 完全随机，无法从远程文件名推断原始文件名
- 使用 SHA-256 确保唯一性
- 远程存储只看到随机哈希值

## 元数据管理

### 元数据管理

#### 元数据结构

```go
type FileMeta struct {
    Name       string    // 文件名（不含路径）
    Path       string    // 目录路径（不含文件名，导入时会自动清除）
    RemoteName string    // 远程文件名 (SHA-256 hash)
    Size       int64     // 原始文件大小
    IsDir      bool      // 是否为目录
    FEK        []byte    // 加密的文件加密密钥
    Salt       []byte    // 加密 Salt/Nonce
    UpdatedAt  time.Time // 更新时间
}
```

> **注意**：`Path` 字段仅在导出（`export`）时用于记录文件的原始目录结构，以便在导入时重建目录层级。在导入并保存到本地存储时，该字段会被自动清除（置空），以节省空间并避免冗余数据，因为本地存储的文件系统结构本身已经隐含了路径信息。

#### 存储后端

#### Local（文件系统）

```
storage/metadata/
├── .clearvault          # 标记文件
├── documents/           # 目录元数据
│   └── file.txt         # 文件元数据 (JSON)
└── photos/
    └── image.jpg
```

**优点**：
- 简单直观
- 易于备份和迁移
- 无额外依赖
- 文件级隔离，避免单点故障

**缺点**：
- 大量文件时性能下降
- 目录遍历开销大

**注意**：ClearVault 已移除 SQLite 支持，统一使用 JSON 文件格式存储元数据，以简化架构并提高可靠性。

## WebDAV 协议实现

### 支持的 WebDAV 方法

| 方法 | 功能 | 实现状态 |
|------|------|----------|
| OPTIONS | 查询服务器能力 | ✅ |
| PROPFIND | 获取资源属性 | ✅ |
| PROPPATCH | 修改资源属性 | ✅ |
| MKCOL | 创建目录 | ✅ |
| GET | 下载文件 | ✅ |
| HEAD | 获取文件头信息 | ✅ |
| PUT | 上传文件 | ✅ |
| DELETE | 删除文件/目录 | ✅ |
| COPY | 复制资源 | ✅ |
| MOVE | 移动/重命名 | ✅ |
| LOCK | 锁定资源 | ✅ |
| UNLOCK | 解锁资源 | ✅ |

### 文件系统接口

实现了 `golang.org/x/net/webdav.FileSystem` 接口：

```go
type FileSystem interface {
    Mkdir(ctx context.Context, name string, perm os.FileMode) error
    OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (File, error)
    RemoveAll(ctx context.Context, name string) error
    Rename(ctx context.Context, oldName, newName string) error
    Stat(ctx context.Context, name string) (os.FileInfo, error)
}
```

## 文件操作流程

### 上传文件（PUT）

```
1. 客户端发送 PUT 请求
2. WebDAV Handler 接收请求
3. FS.OpenFile() 创建 ProxyFile (isNew=true)
4. FS.Write() 被调用
   ├─ 创建 io.Pipe (pr, pw)
   ├─ 启动 goroutine 执行 Proxy.UploadFile(pr)
   └─ 将数据写入 pw
5. Proxy.UploadFile() 执行
   ├─ 检查是否为 0 字节（临时占位）
   │  └─ 是：保存临时元数据 (RemoteName=".pending", 带有效 FEK/Salt)
   │  └─ 否：继续正常流程
   ├─ 生成随机 FEK 和 Salt
   ├─ 加密 FEK（使用主密钥）
   ├─ 生成随机远程文件名
   ├─ 启动 goroutine 进行流式加密 (Engine.EncryptStream)
   ├─ 使用 http.Client (Chunked Encoding) 上传到远端 WebDAV
   └─ 上传完成，保存元数据
6. 返回 201 Created
```

### 下载文件（GET）

```
1. 客户端发送 GET 请求
2. WebDAV Handler 接收请求
3. FS.OpenFile() -> ProxyFile
4. ProxyFile.Read() 被调用
   ├─ 检查是否为临时占位文件
   │  └─ 是：返回 EOF (0字节)
   │  └─ 否：调用 Proxy.DownloadRange()
   ├─ Proxy.DownloadRange()
   │  ├─ 计算请求范围对应的加密块 (Chunk) 范围
   │  ├─ 从远端下载指定范围的加密数据
   │  ├─ 启动 goroutine 进行流式解密 (Engine.DecryptStreamFrom)
   │  └─ 返回 Reader
   └─ 丢弃多余字节以对齐 Offset，返回数据
5. 流式传输给客户端
```

### 重命名（MOVE）

```
1. 客户端发送 MOVE 请求
2. WebDAV Handler 接收请求
3. Proxy.RenameFile() 被调用
   ├─ 检查目标是否存在
   ├─ 获取源文件元数据
   ├─ 更新元数据中的路径
   ├─ 保存新元数据
   └─ 删除旧元数据
4. 返回 201 Created
```

**注意**：重命名不需要重新加密文件，只需更新元数据中的路径映射。

### 删除（DELETE）

```
1. 客户端发送 DELETE 请求
2. WebDAV Handler 接收请求
3. Proxy.RemoveAll() 被调用
   ├─ 递归获取所有子文件
   ├─ 对每个文件:
   │  ├─ 从远端删除加密文件
   │  └─ 删除本地元数据
   └─ 删除目录元数据
4. 返回 204 No Content
```

## RaiDrive 兼容性

### 问题：两阶段上传

RaiDrive 在上传文件时采用两阶段策略：

1. **第一阶段**：发送 0 字节 PUT 请求（占位/测试）
2. **第二阶段**：发送实际内容的 PUT 请求

如果第一阶段后立即返回 404（文件不存在），RaiDrive 会进入**无限重试循环**。

### 问题：文件系统占位符的局限性

之前的实现使用文件系统存储 `.pending` 占位符：

```go
// 旧实现：创建 .pending 元数据文件
meta := &metadata.FileMeta{
    RemoteName: ".pending",
    // ...
}
return p.meta.Save(meta, pname)
```

**问题**：
- 需要创建和管理临时元数据文件（`.pending` 文件）
- 文件系统操作有延迟，可能导致 PROPFIND 返回 404
- 需要额外的清理逻辑
- 可能产生文件系统垃圾

### 解决方案：内存占位符

使用内存中的 map 存储占位符标记，替代文件系统占位符：

```go
// 内存占位符结构
type PendingEntry struct {
    createdAt time.Time
    expiresAt time.Time
}

type PendingFileCache struct {
    data map[string]*PendingEntry
    mu   sync.RWMutex
}
```

#### 工作流程

1. **Raidrive 上传 0 字节文件** → `SavePlaceholder()` 在内存中设置占位符（TTL 30 秒）
2. **Raidrive 读取文件** → `OpenFile()` 检查内存占位符，返回空内容
3. **Raidrive 上传实际内容** → `UploadFile()` 删除内存占位符，创建真实元数据
4. **后台清理** → 每 30 秒自动清理过期占位符

#### 代码实现

**SavePlaceholder()**：
```go
func (p *Proxy) SavePlaceholder(pname string) error {
    pname = p.normalizePath(pname)
    log.Printf("Proxy: Saving memory placeholder for 0-byte file '%s'", pname)

    // 使用内存占位符，TTL 30 秒
    p.pendingCache.Add(pname, 30*time.Second)
    return nil
}
```

**OpenFile()**：
```go
// 检查内存占位符
if fs.p.pendingCache.Exists(name) {
    log.Printf("FS OpenFile: returning empty file for memory placeholder '%s'", name)
    return &ProxyFile{
        fs:   fs,
        name: name,
        meta: &metadata.FileMeta{
            Name:       path.Base(name),
            RemoteName: ".pending",
            Size:       0,
            IsDir:      false,
            UpdatedAt:  time.Now(),
        },
    }, nil
}
```

**Stat()**：
```go
// 检查内存占位符
if fs.p.pendingCache.Exists(name) {
    log.Printf("FS Stat: returning 0-byte file info for memory placeholder '%s'", name)
    return &FileInfo{
        name:    path.Base(name),
        size:    0,
        isDir:   false,
        modTime: time.Now(),
    }, nil
}
```

**UploadFile()**：
```go
// 检查并删除内存占位符
if p.pendingCache.Exists(pname) {
    log.Printf("Proxy: Replacing memory placeholder with real file for '%s'", pname)
    p.pendingCache.Remove(pname)
}
```

#### 过期清理机制

后台协程定期清理过期的占位符：

```go
func (c *PendingFileCache) cleanup() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        c.mu.Lock()
        now := time.Now()
        for path, entry := range c.data {
            if now.After(entry.expiresAt) {
                delete(c.data, path)
            }
        }
        c.mu.Unlock()
    }
}
```

### 优势

- ✅ **无需文件系统操作**：不创建 `.pending` 文件，避免文件系统延迟
- ✅ **立即响应**：内存操作，无 I/O 延迟
- ✅ **自动过期清理**：通过后台协程自动清理，无需手动管理
- ✅ **架构简单**：不引入复杂的文件系统逻辑
- ✅ **无垃圾文件**：不会在文件系统中留下临时文件

### 效果

- ✅ **兼容性增强**：Raidrive 可以正常进行两阶段上传
- ✅ 避免 RaiDrive 无限重试
- ✅ 不产生远端垃圾文件
- ✅ 内存占位符自动过期清理
- ✅ 性能提升：无文件系统 I/O 开销

## Windows 文件锁定处理

### 问题背景

Windows 系统（特别是配合 RaiDrive 使用时）存在文件锁定问题：

1. **Explorer 预览锁定**：Windows 资源管理器预览文件时会持有文件句柄
2. **RaiDrive 缓存锁定**：RaiDrive 可能在枚举目录时锁定文件
3. **重命名失败**：`os.Rename()` 在文件被锁定时会失败

### 解决方案

#### 1. 重试机制

```go
func retryOperation(op func() error) error {
    var err error
    for i := 0; i < 100; i++ {  // 最多重试 100 次 (约 5 秒)
        err = op()
        if err == nil {
            return nil
        }
        time.Sleep(50 * time.Millisecond)
    }
    return err
}

// 使用示例
err := retryOperation(func() error {
    return os.RemoveAll(path)
})
```

#### 2. Rename 的 Copy+Delete 兜底

```go
func Rename(oldPath, newPath string) error {
    // 尝试直接重命名
    err := retryOperation(func() error {
        return os.Rename(oldPath, newPath)
    })
    if err == nil {
        return nil
    }
    
    // 失败则使用 Copy+Delete 兜底
    log.Printf("Rename failed, using Copy+Delete fallback")
    
    // 递归复制
    if err := copyDir(oldPath, newPath); err != nil {
        return err
    }
    
    // 删除原文件（尽力而为）
    RemoveAll(oldPath)
    return nil
}
```

#### 3. 文件同步

确保复制的文件完全写入磁盘：

```go
func copyFile(src, dst string) error {
    in, _ := os.Open(src)
    defer in.Close()
    
    out, _ := os.Create(dst)
    defer out.Close()
    
    io.Copy(out, in)
    
    // 强制刷新到磁盘
    return out.Sync()
}
```

## 性能优化

### 真正的流式上传 (OOM 修复与 Content-Length 优化)

**问题 1：内存溢出 (OOM)**
早期版本使用 `gowebdav` 库时，对于未知大小的流（如加密流），库可能会尝试读取整个流到内存以计算 `Content-Length`，或者未正确处理分块传输，导致上传大文件（如视频）时发生 OOM。

**问题 2：gowebdav 库的流处理行为**
`gowebdav` 库在处理认证重试（如 401 Unauthorized -> 携带 Token 重试）时，若 `http.Request.GetBody` 为 nil（流式 Body 默认情况），会尝试重新读取 Body。如果 Body 是 `io.Pipe` 等不可回溯的流，这将导致上传失败或被库内部缓冲机制读入内存。

**综合解决方案**：

1. **原生 HTTP Client 与流式控制**：
   在 `RemoteClient.Upload` 中，不再依赖 `gowebdav` 的高层封装，而是直接使用 Go 原生 `http.Client` 构造请求。这赋予我们对 `Content-Length` 和 Body 处理的完全控制权。

2. **智能 Content-Length 处理**：
   - **定长流（优选）**：如果 WebDAV 客户端在 `PUT` 请求中提供了 `Content-Length`，代理层会捕获该长度，结合加密算法（AES-GCM 增加的 Tag 开销）预计算出最终密文大小。
     ```go
     // engine.go
     func CalculateEncryptedSize(originalSize int64) int64 {
         numChunks := (originalSize + ChunkSize - 1) / ChunkSize
         return originalSize + numChunks*TagSize
     }
     ```
     随后，`RemoteClient` 会设置准确的 `Content-Length` 头。这不仅避免了分块传输的开销，还让远端服务器能更好地分配资源和显示进度。
   
   - **不定长流（兜底）**：如果无法获取原始长度，则回退到 Chunked Transfer Encoding (`req.ContentLength = -1`)，确保大文件依然可以安全上传，仅牺牲少量进度条准确性。

3. **管道连接 (Pipeline)**：
   数据流向：`Client` -> `ProxyFile.Write` -> `io.Pipe` -> `CryptoEngine` -> `http.Client` -> `Remote WebDAV`。全链路内存占用恒定（约为 ChunkSize * 并发数），与文件大小无关。

```go
// internal/webdav/client.go
func (c *RemoteClient) Upload(name string, data io.Reader, size int64) error {
    // ...
    if size > 0 {
        req.ContentLength = size
    } else {
        req.ContentLength = -1 // Force chunked encoding
    }
    // ...
}
```

### 范围请求支持 (Range Requests)

**问题**：视频播放和随机读取需要支持 HTTP Range 请求，而不是每次都下载整个文件。

**解决**：
1. **块对齐计算**：根据请求的字节范围 `[start, end]`，计算出覆盖该范围的加密块（AES-GCM Chunk）范围。
2. **DecryptStreamFrom**：加密引擎新增方法，支持从指定的 Block Index 开始解密，而不是必须从头开始。
3. **按需下载**：只向远端 WebDAV 请求必要的加密数据块。

```go
// 计算 Chunk
startChunk := offset / ChunkSize
encStart := startChunk * CipherChunkSize
encLength := ...

// 下载加密片段
cipherStream := remote.DownloadRange(remoteName, encStart, encLength)

// 从指定 Chunk 开始解密
engine.DecryptStreamFrom(cipherStream, output, salt, startChunk)
```

### 元数据缓存

对于频繁访问的元数据，可以考虑添加内存缓存：

```go
type CachedMetadata struct {
    cache map[string]*FileMeta
    mu    sync.RWMutex
}

func (c *CachedMetadata) Get(path string) (*FileMeta, error) {
    c.mu.RLock()
    if meta, ok := c.cache[path]; ok {
        c.mu.RUnlock()
        return meta, nil
    }
    c.mu.RUnlock()
    
    // 从存储加载
    meta, err := c.storage.Get(path)
    if err == nil {
        c.mu.Lock()
        c.cache[path] = meta
        c.mu.Unlock()
    }
    return meta, err
}
```

### 并发控制

使用 Goroutine 池限制并发数，避免资源耗尽：

```go
type WorkerPool struct {
    sem chan struct{}
}

func NewWorkerPool(size int) *WorkerPool {
    return &WorkerPool{
        sem: make(chan struct{}, size),
    }
}

func (p *WorkerPool) Do(fn func()) {
    p.sem <- struct{}{}
    go func() {
        defer func() { <-p.sem }()
        fn()
    }()
}
```

## 安全考虑

### 安全考虑

#### 密钥管理

1. **主密钥**：
   - 永远不传输到远端
   - 仅存储在本地配置文件（若缺失或默认，服务首次启动时会自动生成并保存）
   - 建议使用环境变量或密钥管理服务

2. **文件加密密钥（FEK）**：
   - 每个文件独立的随机密钥
   - 使用主密钥加密后存储在元数据中
   - 即使元数据泄露，没有主密钥也无法解密

#### 导出/导入流程安全性

在元数据导出/导入（`export`/`import`）过程中，系统采用了特殊的密钥管理策略以确保数据能够在不同用户（不同主密钥）之间安全传输：

1. **FEK 解密与再加密**：
   - **导出时**：代理层使用发送者的主密钥解密元数据中的 FEK，获取**原始 FEK**。然后，该原始 FEK 被封装进 JSON 元数据，并使用临时生成的**会话密钥（Session Key）**进行 AES-GCM 加密。
     - *注意*：虽然原始 FEK 短暂存在于内存中，但写入 tar 包的是被会话密钥加密后的密文。
   - **导入时**：代理层使用会话密钥解密元数据，获取**原始 FEK**。然后，立即使用接收者的主密钥对原始 FEK 进行加密，并保存到本地元数据存储中。

2. **会话密钥保护**：
   - 会话密钥（AES-256）是每次导出时随机生成的。
   - 它被临时生成的 RSA 公钥加密，存储在 tar 包的 `manifest.json` 中。
   - 对应的 RSA 私钥被用户提供的**分享密码（Share Key）**派生的密钥（PBKDF2）加密。

3. **安全性分析**：
   - **机密性**：攻击者截获 tar 包后，必须破解分享密码才能解密 RSA 私钥，进而解密会话密钥和 FEK。
   - **依赖性**：此流程的安全性强依赖于**分享密码的强度**。建议用户使用高强度的随机分享密码（如系统默认生成的 16 位随机字符）。
   - **隔离性**：发送者的主密钥和接收者的主密钥从未离开各自的本地环境，互不泄露。

#### 认证

- HTTP Basic Authentication
- 建议生产环境使用 HTTPS（通过反向代理）
- 支持自定义认证中间件

#### 数据完整性

- AES-GCM 提供认证加密
- 自动检测数据篡改
- 解密失败时拒绝返回数据

## 简单分享功能

### 概述

ClearVault 支持通过密码加密的 tar 包分享元数据，无需 WebDAV 协议，可直接通过文件传输。该功能基于密钥模式实现，使用用户提供的密码加密临时生成的 RSA 密钥对。

### 安全模型

#### 加密流程

```
用户密码
    ↓ PBKDF2 (100,000 次迭代)
派生密钥 (32字节)
    ↓ AES-GCM
加密的临时私钥
    ↓
临时 RSA 私钥
    ↓ RSA-OAEP
AES 密钥
    ↓ AES-GCM
元数据文件
```

#### 安全特性

- **PBKDF2**：100,000 次密钥派生迭代，防止暴力破解
- **AES-256-GCM**：元数据加密，提供认证加密
- **RSA-2048**：临时密钥加密，非对称加密保护
- **临时密钥**：每次分享生成新的密钥对，避免密钥复用
- **随机密码**：可自动生成 16 位随机密码，提高安全性

### 核心组件

#### 密钥管理器 (internal/key/manager.go)

负责临时 RSA 密钥对的生成和序列化。

```go
type KeyManager struct {}

func NewKeyManager() (*KeyManager, error)

// 生成临时 RSA 密钥对
func (km *KeyManager) GenerateTempKeyPair(bits int) (*rsa.PrivateKey, *rsa.PublicKey, error)

// 序列化私钥为 PEM 格式
func (km *KeyManager) SerializePrivateKey(privKey *rsa.PrivateKey) ([]byte, error)

// 从 PEM 格式反序列化私钥
func (km *KeyManager) DeserializePrivateKey(pemData []byte) (*rsa.PrivateKey, error)
```

#### 非对称加密引擎 (internal/crypto/asymmetric.go)

负责 RSA-OAEP 加密和解密。

```go
type AsymmetricEngine struct {
    pubKey  *rsa.PublicKey
    privKey *rsa.PrivateKey
}

func NewAsymmetricEngine(pubKey *rsa.PublicKey, privKey *rsa.PrivateKey) *AsymmetricEngine

// RSA-OAEP 加密
func (ae *AsymmetricEngine) EncryptKey(key []byte) ([]byte, error)

// RSA-OAEP 解密
func (ae *AsymmetricEngine) DecryptKey(encryptedKey []byte) ([]byte, error)
```

#### Tar 打包工具 (internal/proxy/tar_util.go)

负责创建和提取 tar 包。

```go
// 创建 tar 包（密钥模式）
func (p *Proxy) CreateTarPackage(
    paths []string,
    outputDir string,
    tempPrivKey *rsa.PrivateKey,
    tempPubKey *rsa.PublicKey,
    aesKey []byte,
) (string, error)

// 提取 tar 包
func (p *Proxy) ExtractTarPackage(
    tarPath string,
    outputDir string,
    privateKey *rsa.PrivateKey,
) (*TarPackage, error)

// 生成随机 ID（时间戳 + 随机数 + 路径哈希 + 文件名哈希）
func generateRandomID(virtualPath string) string
```

#### 简单分享实现 (internal/proxy/proxy_asymmetric.go)

负责创建和接收分享包。

```go
// 创建分享包
func (p *Proxy) CreateSharePackage(
    paths []string,
    outputDir string,
    shareKey string,
) (string, error)

// 接收分享包
func (p *Proxy) ReceiveSharePackage(
    tarPath string,
    outputDir string,
    shareKey string,
) (*TarPackage, error)
```

### 文件名生成算法

#### 算法设计

```go
func generateRandomID(virtualPath string) string {
    // 时间戳（纳秒）
    timestamp := time.Now().UnixNano()

    // 随机数（8字节）
    randomBytes := make([]byte, 8)
    rand.Read(randomBytes)

    // 解析路径和文件名
    dir := filepath.Dir(virtualPath)
    name := filepath.Base(virtualPath)

    // 组合路径 + 文件名作为哈希输入
    hashInput := dir + "/" + name
    hash := sha256.Sum256([]byte(hashInput))
    nameHash := hex.EncodeToString(hash[:8])

    // 组合：时间戳_随机数_路径文件名哈希
    return fmt.Sprintf("%d_%s_%s",
        timestamp,
        hex.EncodeToString(randomBytes),
        nameHash)
}
```

#### 文件名格式

**格式**：`时间戳_随机数_路径文件名哈希.enc`

**示例**：`1768852296770834969_253a47900c42af81_947a215fa18d419b.enc`

#### 组成部分

1. **时间戳**（纳秒）：约 19 位数字
   - 保证同一纳秒内不会生成两个文件
   - 便于追踪文件生成时间

2. **随机数**（8字节）：16 位十六进制字符
   - 提供额外的唯一性保证
   - 防止时间戳冲突

3. **路径文件名哈希**（8字节）：16 位十六进制字符
   - SHA-256 哈希的前 8 字节
   - 区分不同目录下的同名文件
   - 区分同一目录下的不同文件

#### 碰撞概率分析

- **时间戳**：纳秒级精度，同一纳秒内生成两个文件的概率极低
- **随机数**：8字节 = 64位随机数，碰撞概率为 2^-64
- **路径文件名哈希**：SHA-256 哈希，碰撞概率极低
- **综合**：理论碰撞概率接近于 0

### 使用流程

#### 导出分享包

```bash
# 1. 指定密码导出
./clearvault export \
    --paths "/documents/report.pdf" \
    --output /tmp/export \
    --share-key "my-secret-password"

# 2. 自动生成随机密码（16位）
./clearvault export \
    --paths "/documents/report.pdf" \
    --output /tmp/export

# 3. 使用指定配置文件
./clearvault export \
    --config config-s3.yaml \
    --paths "/documents" \
    --output /tmp/export
```

**内部流程**：

1. 生成临时 RSA 密钥对（2048位）
2. 使用 PBKDF2 派生 AES 密钥（100,000 次迭代）
3. 使用 AES-GCM 加密临时私钥
4. 读取元数据文件
5. 使用 AES 密钥加密元数据
6. 生成随机文件名
7. 创建 tar 包，包含：
   - `manifest.json`：清单文件
   - `metadata/`：加密的元数据文件
   - `private_key.enc`：加密的临时私钥
8. 返回 tar 包路径

#### 导入分享包

```bash
./clearvault import \
    --input /tmp/share_abc123.tar \
    --share-key "my-secret-password"

# 使用指定配置文件
./clearvault import \
    --config config-s3.yaml \
    --input /tmp/share.tar \
    --share-key "password"
```

**内部流程**：

1. 打开 tar 文件
2. 读取清单文件
3. 使用 PBKDF2 派生 AES 密钥（100,000 次迭代）
4. 使用 AES-GCM 解密临时私钥
5. 使用临时私钥解密 AES 密钥
6. 遍历 metadata/ 目录下的所有 .enc 文件
7. 使用 AES 密钥解密元数据
8. 使用主密钥重新加密 FEK（文件加密密钥）
9. 保存元数据到本地存储

### 安全考虑

#### 密钥管理

1. **临时密钥**：每次分享生成新的密钥对，避免密钥复用
2. **密码强度**：建议使用 16 位以上随机密码
3. **密钥派生**：使用 PBKDF2 进行密钥派生，增加暴力破解难度

#### 加密强度

1. **AES-256-GCM**：提供认证加密，防止篡改
2. **RSA-2048**：非对称加密保护临时私钥
3. **SHA-256**：哈希算法保证文件名唯一性

#### 潜在风险

1. **密码泄露**：如果密码泄露，攻击者可以解密分享包
   - **缓解措施**：使用强密码，定期更换

2. **临时私钥泄露**：如果临时私钥泄露，攻击者可以解密元数据
   - **缓解措施**：临时私钥使用 AES-GCM 加密，密码保护

3. **文件名碰撞**：理论上不可能发生
   - **缓解措施**：时间戳 + 随机数 + 路径哈希保证唯一性

### 性能分析

#### 时间复杂度

- **PBKDF2**：O(100,000) 次迭代
- **AES-GCM**：O(n)，n 为数据大小
- **RSA-OAEP**：O(1)，固定大小密钥
- **SHA-256**：O(1)，固定大小输入

#### 空间复杂度

- **临时密钥对**：约 2KB（2048位 RSA）
- **AES 密钥**：32 字节
- **元数据文件**：取决于文件数量和大小

#### 性能影响

- **时间戳获取**：纳秒级时间戳，性能影响可忽略
- **哈希计算**：SHA-256 哈希，对单个文件影响极小
- **字符串拼接**：Go 的字符串拼接性能良好
- **加密操作**：AES-GCM 和 RSA-OAEP 性能良好

### 扩展性

#### 支持更多加密算法

当前使用 AES-256-GCM，可以扩展支持：
- AES-256-CTR
- ChaCha20-Poly1305

#### 支持更多密钥派生算法

当前使用 PBKDF2，可以扩展支持：
- Argon2
- scrypt

#### 支持更多密钥长度

当前使用 RSA-2048，可以扩展支持：
- RSA-3072
- RSA-4096

### 测试验证

#### 单元测试

```bash
go test ./internal/proxy/... -v
```

**测试用例**：

1. `TestCreateTarPackage`：测试创建 tar 包
2. `TestExtractTarPackage`：测试提取 tar 包
3. `TestGenerateRandomID`：测试随机 ID 生成
4. `TestEncryptDecryptPrivateKey`：测试私钥加密/解密
5. `TestDeriveKeyFromPassword`：测试密钥派生

#### 集成测试

```bash
# 1. 导出
./clearvault export \
    --paths "/test" \
    --output /tmp/export \
    --share-key "test-password"

# 2. 导入
./clearvault import \
    --input /tmp/export/share_*.tar \
    --share-key "test-password"
```

#### 性能测试

```bash
# 测试导出 1000 个元数据文件的时间
time ./clearvault export \
    --paths "/large-directory" \
    --output /tmp/export \
    --share-key "test-password"
```

## 命令行接口

### 命令结构

ClearVault 使用子命令结构，所有功能通过明确的子命令访问：

```bash
clearvault <command> [command options]
```

### 可用命令

| 命令 | 功能 | 说明 |
|------|------|------|
| `encrypt` | 本地文件加密 | 离线加密本地文件/目录 |
| `export` | 元数据导出 | 导出加密分享包 |
| `import` | 元数据导入 | 导入加密分享包 |
| `server` | 启动 WebDAV 服务器 | 启动在线服务 |

### 命令参数

#### encrypt 命令

```bash
clearvault encrypt -in <input_path> -out <output_dir> [--config <config_file>]
```

**参数**：
- `-in string`：要加密的本地文件/目录路径（必需）
- `-out string`：加密文件输出目录（必需）
- `--config string`：配置文件路径（默认 "config.yaml"）
- `--help`：显示帮助信息

**实现**：调用 `ExportLocal()`，使用主密钥直接加密文件

#### export 命令

```bash
clearvault export --paths <paths> --output <output_dir> [--share-key <key>] [--config <config_file>]
```

**参数**：
- `--paths string`：虚拟路径列表（逗号分隔）（必需）
- `--output string`：输出目录（必需）
- `--share-key string`：分享密钥（可选，不指定则自动生成）
- `--config string`：配置文件路径（默认 "config.yaml"）
- `--help`：显示帮助信息

**实现**：调用 `CreateSharePackage()`，多层加密（密码→PBKDF2→AES→RSA）

#### import 命令

```bash
clearvault import --input <input_file> --share-key <key> [--config <config_file>]
```

**参数**：
- `--input string`：输入 tar 文件路径（必需）
- `--share-key string`：分享密钥（必需）
- `--config string`：配置文件路径（默认 "config.yaml"）
- `--help`：显示帮助信息

**实现**：调用 `ReceiveSharePackage()`，解密并恢复元数据

#### server 命令

```bash
clearvault server [--config <config_file>] [--help]
```

**参数**：
- `--config string`：配置文件路径（默认 "config.yaml"）
- `--help`：显示帮助信息

**实现**：启动 WebDAV 服务器，连接远程存储

### 帮助系统

```bash
# 查看所有可用命令
./clearvault --help

# 查看特定命令的帮助
./clearvault encrypt --help
./clearvault export --help
./clearvault import --help
./clearvault server --help
```

### 参数优先级

命令行参数的优先级从高到低：
1. 命令行参数（如 `--config custom.yaml`）
2. 环境变量（如 `MASTER_KEY="test-key"`）
3. 配置文件（如 `config.yaml`）
4. 默认值

## 未来改进方向

1. **分块加密**：支持超大文件的分块加密和并行上传
2. **增量同步**：仅同步文件变更部分
3. **客户端缓存**：本地缓存常用文件
4. **多用户支持**：每个用户独立的加密密钥
5. **版本控制**：文件历史版本管理
6. **压缩**：加密前压缩文件以节省空间
7. **分享包格式优化**：支持压缩分享包，减少文件大小
8. **多文件分享**：支持批量选择多个文件进行分享
9. **分享链接**：生成可分享的链接（配合云存储）

## 参考资料

- [WebDAV RFC 4918](https://tools.ietf.org/html/rfc4918)
- [AES-GCM NIST SP 800-38D](https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf)
- [Go crypto/cipher Documentation](https://pkg.go.dev/crypto/cipher)
- [golang.org/x/net/webdav](https://pkg.go.dev/golang.org/x/net/webdav)
