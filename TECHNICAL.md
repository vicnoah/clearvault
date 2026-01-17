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
   - 支持多种后端（Local、SQLite）
   - 路径到远程文件名的映射

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

### 元数据结构

```go
type FileMeta struct {
    Path       string    // 虚拟路径 (如 /documents/file.txt)
    RemoteName string    // 远程文件名 (SHA-256 hash)
    Size       int64     // 原始文件大小
    IsDir      bool      // 是否为目录
    FEK        []byte    // 加密的文件加密密钥
    Salt       []byte    // 加密 Salt/Nonce
    UpdatedAt  time.Time // 更新时间
}
```

### 存储后端

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

**缺点**：
- 大量文件时性能下降
- 目录遍历开销大

#### SQLite（数据库）

```sql
CREATE TABLE metadata (
    path TEXT PRIMARY KEY,
    remote_name TEXT NOT NULL,
    size INTEGER NOT NULL,
    is_dir BOOLEAN NOT NULL,
    fek BLOB,
    salt BLOB,
    updated_at DATETIME
);

CREATE INDEX idx_remote_name ON metadata(remote_name);
```

**优点**：
- 高性能查询
- 支持大量文件
- 事务支持

**缺点**：
- 需要定期备份数据库
- 稍微复杂

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

### 解决方案：临时占位元数据

```go
// 第一阶段：0 字节上传
if fileSize == 0 {
    // 生成有效的随机 FEK 和 Salt
    fek := GenerateRandomBytes(32)
    salt := GenerateRandomBytes(12)
    
    // 保存临时元数据
    meta := &FileMeta{
        Path:       path,
        RemoteName: ".pending",  // 特殊标记
        Size:       0,
        FEK:        Encrypt(fek), // 保存加密的 FEK
        Salt:       salt,
        // ...
    }
    return meta.Save()
}

// 第二阶段：实际内容上传
// 检查并替换临时元数据
existingMeta, _ := GetMetadata(path)
if existingMeta != nil && existingMeta.RemoteName == ".pending" {
    log.Printf("Replacing temporary placeholder")
}
// 正常上传流程 (生成新的 FEK/Salt 并覆盖元数据)...
```

### 读取临时文件

```go
func DownloadFile(path string) (io.ReadCloser, error) {
    meta := GetMetadata(path)
    
    // 如果是临时占位文件，返回空内容
    if meta.RemoteName == ".pending" {
        return io.NopCloser(bytes.NewReader([]byte{})), nil
    }
    
    // 正常下载流程...
}
```

### 效果

- ✅ **兼容性增强**：即使是占位文件也拥有完整的元数据结构（FEK/Salt），防止客户端 PROPFIND 或尝试读取时出错。
- ✅ 避免 RaiDrive 无限重试
- ✅ 不产生远端垃圾文件
- ✅ 临时元数据自动被真实文件覆盖

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

### 真正的流式上传 (OOM 修复)

**问题**：早期版本使用 `gowebdav` 库时，对于未知大小的流（加密流），库可能会尝试读取整个流到内存以计算 `Content-Length`，或者未正确处理分块传输，导致上传大文件（如视频）时发生 OOM (Out Of Memory)。

**解决**：
1. **原生 HTTP Client**：替换 `gowebdav` 的上传实现，直接使用 Go 原生 `http.Client`。
2. **Chunked Transfer Encoding**：显式设置 `req.ContentLength = -1`，强制使用 HTTP 分块传输编码。
3. **管道连接**：`ProxyFile.Write` -> `io.Pipe` -> `CryptoEngine` -> `http.Client` -> Remote WebDAV。数据在内存中仅做极小缓冲，实现真正的流式传输。

```go
// client.go
req, _ := http.NewRequest("PUT", url, pipeReader)
req.ContentLength = -1 // Force chunked encoding
http.DefaultClient.Do(req)
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

### 密钥管理

1. **主密钥**：
   - 永远不传输到远端
   - 仅存储在本地配置文件（若缺失或默认，服务首次启动时会自动生成并保存）
   - 建议使用环境变量或密钥管理服务

2. **文件加密密钥（FEK）**：
   - 每个文件独立的随机密钥
   - 使用主密钥加密后存储在元数据中
   - 即使元数据泄露，没有主密钥也无法解密

### 认证

- HTTP Basic Authentication
- 建议生产环境使用 HTTPS（通过反向代理）
- 支持自定义认证中间件

### 数据完整性

- AES-GCM 提供认证加密
- 自动检测数据篡改
- 解密失败时拒绝返回数据

## 未来改进方向

1. **分块加密**：支持超大文件的分块加密和并行上传
2. **增量同步**：仅同步文件变更部分
3. **客户端缓存**：本地缓存常用文件
4. **多用户支持**：每个用户独立的加密密钥
5. **版本控制**：文件历史版本管理
6. **压缩**：加密前压缩文件以节省空间

## 参考资料

- [WebDAV RFC 4918](https://tools.ietf.org/html/rfc4918)
- [AES-GCM NIST SP 800-38D](https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf)
- [Go crypto/cipher Documentation](https://pkg.go.dev/crypto/cipher)
- [golang.org/x/net/webdav](https://pkg.go.dev/golang.org/x/net/webdav)
