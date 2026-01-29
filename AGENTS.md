# ClearVault - Agent 参考文档

本文档整合 README.md、TECHNICAL.md 和测试实现，为大模型提供完整的 ClearVault 项目参考。

## 项目概述

ClearVault 是一个基于 WebDAV 协议的加密云存储代理服务，支持将文件加密后存储到任意 WebDAV/S3 兼容的云存储服务，同时提供本地 WebDAV/FUSE 接口供客户端访问。

### 核心特性
- **端到端加密**：AES-256-GCM 加密，主密钥由用户掌控
- **多协议支持**：WebDAV、S3、本地文件系统
- **流式加密/解密**：支持大文件，内存占用低
- **FUSE 挂载**：可作为本地目录挂载
- **简单分享**：加密 tar 包分享，无需 WebDAV 服务器
- **fnOS 原生应用**：支持飞牛 NAS 原生应用形态

---

## 项目结构

```
clearvault/
├── cmd/clearvault/          # 主程序入口
│   ├── main.go              # CLI 入口
│   └── mount_fuse.go        # FUSE 挂载命令 (go:build fuse)
├── internal/
│   ├── api/                 # HTTP API (初始化/配置)
│   ├── config/              # 配置管理
│   ├── crypto/              # 加密引擎 (AES-GCM, RSA)
│   ├── key/                 # 密钥管理
│   ├── fuse/                # FUSE 文件系统实现
│   ├── metadata/            # 元数据存储 (JSON 文件)
│   ├── proxy/               # 代理层 (核心逻辑)
│   ├── remote/              # 远端存储后端
│   │   ├── s3/              # S3 客户端
│   │   ├── webdav/          # WebDAV 客户端
│   │   └── local/           # 本地文件系统
│   └── webdav/              # WebDAV 协议处理
├── pkg/gowebdav/            # WebDAV 客户端库
├── tests/                   # 测试目录
│   ├── testutil/            # 测试工具 (自动服务器管理)
│   ├── configs/             # 测试配置
│   └── scripts/             # 测试脚本
├── tools/                   # 测试服务器二进制 (zs3, sweb)
└── deploy/                  # 部署配置 (fnOS, Docker)
```

---

## 架构设计

### 系统架构

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
         │ WebDAV/S3 Protocol (Encrypted)
         ▼
┌─────────────────┐
│  Remote Storage │ (Nextcloud, S3, WebDAV)
└─────────────────┘
```

### 核心组件职责

| 组件 | 路径 | 职责 |
|-----|------|------|
| WebDAV Handler | `internal/webdav` | 处理 WebDAV 协议请求，实现 FileSystem 接口 |
| Proxy Layer | `internal/proxy` | 协调加密/解密，管理上传/下载流程 |
| Crypto Engine | `internal/crypto` | AES-256-GCM 加密/解密，流式处理 |
| Metadata Manager | `internal/metadata` | 本地 JSON 文件存储元数据 |
| Remote Backend | `internal/remote/*` | S3/WebDAV/Local 存储适配 |

---

## 加密系统

### 加密算法

- **对称加密**：AES-256-GCM
  - 密钥长度：256 位（32 字节）
  - Nonce：96 位（12 字节），每个文件唯一
  
- **密钥派生**：PBKDF2 (100,000 次迭代)
- **文件加密密钥（FEK）**：每个文件独立的随机密钥

### 加密流程

```
原始文件
    ↓
生成随机 FEK (32B) + Salt (12B)
    ↓
使用主密钥加密 FEK (AES-256-GCM)
    ↓
使用 FEK 加密文件内容 (流式 AES-GCM)
    ↓
生成随机远程文件名 (SHA-256)
    ↓
上传到远端存储
    ↓
保存元数据 (路径、远程文件名、加密的 FEK、Salt)
```

### 文件名加密

远程文件名生成算法：
```go
remoteName = SHA256(randomBytes(32))
```

- 完全随机，无法从远程文件名推断原始文件名
- 使用 SHA-256 确保唯一性

---

## 元数据管理

### 元数据结构

```go
type FileMeta struct {
    Name       string    // 文件名
    Path       string    // 目录路径（导出时使用，本地存储时清空）
    RemoteName string    // 远程文件名 (SHA-256 hash)
    Size       int64     // 原始文件大小
    IsDir      bool      // 是否为目录
    FEK        []byte    // 加密的文件加密密钥
    Salt       []byte    // 加密 Salt/Nonce
    UpdatedAt  time.Time // 更新时间
}
```

### 存储方式

使用本地文件系统（JSON 格式）：
```
storage/metadata/
├── .clearvault          # 标记文件
├── documents/
│   └── file.txt         # 文件元数据 (JSON)
└── photos/
    └── image.jpg
```

---

## WebDAV 协议实现

### 支持的方法

| 方法 | 功能 | 状态 |
|------|------|------|
| OPTIONS | 查询服务器能力 | ✅ |
| PROPFIND | 获取资源属性 | ✅ |
| MKCOL | 创建目录 | ✅ |
| GET/HEAD | 下载文件 | ✅ |
| PUT | 上传文件 | ✅ |
| DELETE | 删除文件/目录 | ✅ |
| COPY/MOVE | 复制/移动 | ✅ |
| LOCK/UNLOCK | 锁定/解锁 | ✅ |

### RaiDrive 兼容性优化

**问题**：RaiDrive 采用两阶段上传（先 PUT 0 字节，再 PUT 实际内容）

**解决方案**：内存占位符机制
- 0 字节 PUT：在内存中设置占位符（TTL 30 秒）
- PROPFIND/GET：检查内存占位符，返回空内容
- 实际上传：替换内存占位符为真实元数据

---

## 文件操作流程

### 上传文件 (PUT)

```
1. 客户端发送 PUT 请求
2. FS.OpenFile() 创建 ProxyFile
3. FS.Write() 被调用
   ├─ 创建 io.Pipe
   ├─ 启动 goroutine 执行 Proxy.UploadFile(pr)
   └─ 将数据写入 pw
4. Proxy.UploadFile() 执行
   ├─ 检查是否为 0 字节（保存内存 placeholder）
   ├─ 生成随机 FEK 和 Salt
   ├─ 加密 FEK（使用主密钥）
   ├─ 生成随机远程文件名
   ├─ 启动 goroutine 进行流式加密
   ├─ 使用 http.Client 上传到远端
   └─ 保存元数据
```

### 下载文件 (GET)

```
1. 客户端发送 GET 请求
2. FS.OpenFile() -> ProxyFile
3. ProxyFile.Read() 被调用
   ├─ 检查是否为临时占位文件
   ├─ 调用 Proxy.DownloadRange()
   ├─ 计算请求范围对应的加密块范围
   ├─ 从远端下载指定范围的加密数据
   ├─ 启动 goroutine 进行流式解密
   └─ 返回 Reader
4. 流式传输给客户端
```

---

## 简单分享功能

### 安全模型

```
用户密码
    ↓ PBKDF2 (100,000 次迭代)
派生密钥 (32字节)
    ↓ AES-GCM
加密的临时私钥
    ↓ RSA-OAEP
AES 密钥
    ↓ AES-GCM
元数据文件
```

### 导出分享包流程

1. 生成临时 RSA 密钥对（2048位）
2. 使用 PBKDF2 派生 AES 密钥（100,000 次迭代）
3. 使用 AES-GCM 加密临时私钥
4. 读取元数据文件
5. 使用 AES 密钥加密元数据
6. 生成随机文件名：`时间戳_随机数_路径哈希.enc`
7. 创建 tar 包，包含：
   - `manifest.json`：清单文件
   - `metadata/`：加密的元数据文件
   - `private_key.enc`：加密的临时私钥

### 命令

```bash
# 导出分享包
./clearvault export \
    --paths "/documents" \
    --output /tmp/export \
    --share-key "my-password"

# 导入分享包
./clearvault import \
    --input /tmp/share.tar \
    --share-key "my-password"
```

---

## FUSE 文件系统

### 写入模型

远端存储（WebDAV/S3）不支持随机写，因此采用：
- **严格顺序写**：若 `ofst != expected`，返回 `EOPNOTSUPP`
- **流式上传**：第一次写入时创建 `io.Pipe()`，边写边上传
- **Release 等待**：关闭句柄时等待上传完成

### fnOS 兼容性

- 0 字节 `*.~#*` 临时文件保存为内存 placeholder
- 写入过程中 `Rename` 记录目标名，在 `Release` 时执行
- `Getattr/Open/Access` 把 placeholder 视为存在的 0B 文件

---

## 配置说明

### 配置文件结构 (config.yaml)

```yaml
server:
  listen: "0.0.0.0:8080"
  base_url: "/dav"
  auth:
    user: "admin"
    pass: "your-password"

security:
  master_key: ""  # 留空自动生成 32 字节随机密钥

storage:
  metadata_path: "storage/metadata"
  cache_dir: "storage/cache"

remote:
  # WebDAV 远端
  url: "https://your-webdav-server.com/dav/"
  user: "username"
  pass: "password"
  
  # 或 S3 远端
  # endpoint: "s3.amazonaws.com"
  # bucket: "my-bucket"
  # access_key: "..."
  # secret_key: "..."
```

### 环境变量支持

| 变量 | 说明 |
|------|------|
| `MASTER_KEY` | 主密钥（无配置文件时必填） |
| `SERVER_LISTEN` | 监听地址 |
| `SERVER_AUTH_USER` | 认证用户名 |
| `SERVER_AUTH_PASS` | 认证密码 |
| `REMOTE_URL` | 远端 WebDAV URL |
| `REMOTE_USER` | 远端用户名 |
| `REMOTE_PASS` | 远端密码 |

---

## 测试框架

### 测试结构

```
tests/
├── testutil/
│   ├── server_manager.go   # 自动服务器管理工具
│   └── doc.go              # 包文档
├── configs/
│   ├── test-zs3.yaml
│   └── test-sweb.yaml
└── scripts/
    ├── start_test_servers.sh
    └── stop_test_servers.sh
```

### 测试服务器工具

`tests/testutil` 包提供自动化的 zs3 (S3) 和 sweb (WebDAV) 测试服务器管理。

#### 使用方式

```go
// 方式1: TestMain (推荐)
func TestMain(m *testing.M) {
    manager, _ := testutil.NewTestServerManager()
    manager.StartAll()  // 自动启动 zs3 + sweb
    code := m.Run()
    manager.StopAll()   // 自动停止
    os.Exit(code)
}

// 方式2: 单个测试
func TestMyFeature(t *testing.T) {
    manager := testutil.EnsureServersForTest(t)
    // 测试结束后自动清理
}

// 方式3: 跳过测试
func TestWithServers(t *testing.T) {
    testutil.SkipIfServersNotRunning(t)
    // 如果服务器未运行则跳过
}
```

#### 默认配置

| 服务 | 地址 | 认证 |
|-----|------|------|
| ZS3 (S3) | localhost:9000 | minioadmin / minioadmin |
| SWEB (WebDAV) | localhost:8081/webdav | admin / admin123 |

### 运行测试

```bash
# 一键运行所有测试（自动管理服务器）
./scripts/run_integration_tests.sh

# 单独运行测试包
go test ./internal/remote/s3 -v
go test ./internal/remote/webdav -v
go test ./internal/crypto -v
go test ./internal/key -v
go test ./internal/api -v

# 带覆盖率
go test ./... -cover
```

### 测试覆盖率

| 模块 | 覆盖率 | 说明 |
|-----|-------|------|
| internal/crypto | 65.2% | AES-GCM, RSA 加密 |
| internal/key | 94.7% | 密钥管理 |
| internal/api | 35.8% | HTTP API |
| internal/remote/webdav | 92.3% | WebDAV 客户端 |
| pkg/gowebdav | 73.6% | WebDAV 库 |

---

## CLI 命令

### 可用命令

| 命令 | 功能 |
|------|------|
| `server` | 启动 WebDAV 服务器 |
| `mount` | FUSE 挂载（需要 fuse build tag） |
| `encrypt` | 本地文件加密导出 |
| `export` | 导出加密分享包 |
| `import` | 导入加密分享包 |
| `config` | 配置管理 |

### 示例

```bash
# 启动服务
./clearvault server --config config.yaml

# FUSE 挂载
./clearvault mount --config config.yaml --mountpoint /mnt/clearvault

# 离线加密
./clearvault encrypt -in /path/to/files -out /path/to/export

# 导出分享包
./clearvault export --paths "/documents" --output /tmp/export

# 导入分享包
./clearvault import --input /tmp/share.tar --share-key "password"
```

---

## 部署方式

### Docker 部署

```bash
# 使用 Docker Compose
docker-compose up -d

# 或使用镜像
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/storage:/app/storage \
  ghcr.io/vicnoah/clearvault:latest
```

### fnOS 原生应用

- 打包目录：`deploy/fnos/`
- 构建脚本：`scripts/build_fnos.sh`
- 特性：WebUI 管理、自动挂载、初始化向导

---

## 开发指南

### 构建要求

- Go 1.21+
- 可选：FUSE 库（`libfuse-dev` 或 `fuse`）用于 FUSE 功能

### 构建命令

```bash
# 标准构建
go build -o clearvault ./cmd/clearvault

# 带 FUSE 支持
go build -tags fuse -o clearvault ./cmd/clearvault

# 测试构建
go test ./...
```

### 安装测试工具

```bash
./scripts/install_zs3.sh
./scripts/install_sweb.sh
```

### 测试覆盖率要求

**编写新功能或功能变更时，必须遵循以下测试规范：**

1. **必须编写测试代码**：所有新功能或功能变更都需要编写对应的测试代码
2. **覆盖率标准**：
   - 除特殊情况（如错误处理路径、系统调用等难以模拟的场景）外，测试覆盖率必须达到 **90% 以上**
   - 在能够实现的情况下，测试覆盖率应尽量接近 **100%**
3. **测试文件命名**：测试文件应以 `_test.go` 结尾，与被测试文件放在同一目录
4. **测试类型**：
   - 单元测试：覆盖正常路径和边界条件
   - 错误处理测试：覆盖错误返回和异常情况
   - 并发测试：对并发敏感的功能进行并发安全测试

### 运行测试并检查覆盖率

```bash
# 运行所有测试
go test ./...

# 运行测试并查看覆盖率
go test ./... -cover

# 生成详细覆盖率报告
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

---

## 故障排除

### 服务器启动失败

```bash
# 检查端口占用
lsof -i :9000  # ZS3
lsof -i :8081  # SWEB

# 停止残留进程
pkill -f zs3
pkill -f sweb

# 查看日志
cat tests/testdata/zs3.log
cat tests/testdata/sweb.log
```

### S3 测试失败

zs3 服务器对时间格式支持有限，某些测试会失败（服务器限制，非代码问题）。

### WebDAV 连接问题

```bash
# 测试 WebDAV
curl -X PROPFIND http://localhost:8081/webdav/
```

---

## 参考资料

- [WebDAV RFC 4918](https://tools.ietf.org/html/rfc4918)
- [AES-GCM NIST SP 800-38D](https://nvlpubs.nist.gov/nistpubs/Legacy/SP/nistspecialpublication800-38d.pdf)
- [Go crypto/cipher](https://pkg.go.dev/crypto/cipher)
- [golang.org/x/net/webdav](https://pkg.go.dev/golang.org/x/net/webdav)
