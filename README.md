# ClearVault

[English](README.en.md) | 简体中文

ClearVault 是一个基于 WebDAV 协议的加密云存储代理服务，支持将文件加密后存储到任意 WebDAV 兼容的云存储服务（如 Nextcloud、坚果云等），同时提供本地 WebDAV 接口供客户端访问。

## ✨ 核心特性

- 🔐 **端到端加密**：使用 AES-256-GCM 加密算法，主密钥由用户掌控
- 🌐 **WebDAV 协议**：兼容所有 WebDAV 客户端（RaiDrive、Windows 资源管理器、macOS Finder 等）
- 📁 **文件名加密**：文件名和目录结构完全加密，远端存储仅保存随机哈希值
- 🚀 **流式加密/解密**：支持大文件的流式处理，内存占用低
- 💾 **灵活的元数据存储**：支持本地文件系统或 SQLite 数据库存储元数据
- 🔄 **完整的 WebDAV 支持**：支持文件上传、下载、删除、重命名、目录操作等
- 🪟 **Windows 优化**：针对 Windows 文件锁定和 RaiDrive 客户端进行了特殊优化
- 📤 **离线加密导出**：支持本地批量加密导出后手动上传云端，规避不稳定 WebDAV 上传

## 📋 系统要求

- Go 1.21 或更高版本（编译）
- 支持的操作系统：Windows、Linux、macOS
- 远端 WebDAV 存储服务（如 Nextcloud、坚果云等）

## 🚀 快速开始

### 方式一：直接运行（推荐用于开发测试）

1. **克隆仓库**
```bash
git clone https://github.com/vicnoah/clearvault.git
cd clearvault
```

2. **编译项目**
```bash
go build -o clearvault ./cmd/clearvault
```

3. **创建配置文件**

创建 `config.yaml`：
```yaml
server:
  # 监听地址和端口
  listen: "0.0.0.0:8080"
  # WebDAV 基础 URL（默认为 /）
  base_url: "/dav"
  
  # 认证信息
  auth:
    user: "admin"
    pass: "your-secure-password"

security:
  # 主加密密钥（32字节）
  # 如果留空或保持默认值，首次启动时将自动生成安全密钥并回写入此文件
  master_key: "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY"

storage:
  # 元数据存储配置
  metadata_type: "local"  # 可选: local, sqlite
  metadata_path: "storage/metadata"  # local 类型使用此路径
  # db_path: "storage/metadata.db"  # sqlite 类型使用此路径
  cache_dir: "storage/cache"

remote:
  # 远端 WebDAV 存储配置
  url: "https://your-webdav-server.com/remote.php/dav/files/username/"
  user: "your-webdav-username"
  pass: "your-webdav-password"
```

4. **启动服务**
```bash
./clearvault --config config.yaml
```

服务启动后，本地 WebDAV 服务地址为：`http://127.0.0.1:8080/dav/`

### 方式二：Docker 部署（推荐用于生产环境）

详见 [Docker 部署文档](#-docker-部署)

## 📖 使用指南

### Windows 资源管理器

1. 打开"此电脑"
2. 右键点击空白处，选择"添加一个网络位置"
3. 输入地址：`http://127.0.0.1:8080/dav/`
4. 输入用户名和密码（config.yaml 中配置的）
5. 完成后即可像本地磁盘一样使用

### RaiDrive（推荐）

1. 下载安装 [RaiDrive](https://www.raidrive.com/)
2. 点击"添加" → 选择"WebDAV"
3. 配置：
   - 地址：`http://127.0.0.1:8080/dav/`
   - 用户名/密码：config.yaml 中配置的认证信息
4. 点击"连接"，即可将加密存储挂载为本地磁盘

### macOS Finder

1. 打开 Finder
2. 菜单栏选择"前往" → "连接服务器"（或按 Cmd+K）
3. 输入地址：`http://127.0.0.1:8080/dav/`
4. 输入用户名和密码
5. 连接后即可访问

### Linux（davfs2）

```bash
# 安装 davfs2
sudo apt-get install davfs2  # Debian/Ubuntu
sudo yum install davfs2       # CentOS/RHEL

# 创建挂载点
sudo mkdir -p /mnt/clearvault

# 挂载
sudo mount -t davfs http://127.0.0.1:8080/dav/ /mnt/clearvault

# 输入用户名和密码
```

### 离线加密导出（手动上传云端）

在某些环境下，WebDAV 大文件上传可能不稳定，你可以先在本地把一批文件加密导出到一个统一目录，再通过浏览器、客户端或离线工具手动上传到云端。

1. 使用配置文件准备好主密钥和元数据存储：
   - `security.master_key` 必须稳定且与线上服务一致
   - `storage.metadata_type` / `storage.metadata_path` 决定元数据写入位置

2. 运行一次性离线导出命令（不会启动 WebDAV 服务）：

```bash
./clearvault -config config.yaml -in /path/to/plain-dir-or-file -out /path/to/export-dir
```

参数说明：

- `-in`：要导出的本地路径，可以是单个文件或目录
- `-out`：加密后文件输出目录，目录中只包含随机文件名的密文文件
- 兼容旧参数：
  - `-export-input` 等价于 `-in`（当 `-in` 未提供时）
  - `-export-output` 等价于 `-out`（当 `-out` 未提供时）

注意：

- 导出完成后，`storage.metadata_path` 下会写入这批文件的元数据，包含原始路径和密钥信息
- `-out` 目录中的文件名是随机的 `remoteName`，可以整体上传到目标 WebDAV 存储中的任意目录
- 只要之后在服务器端使用同一份 `config.yaml`（尤其是相同的 `master_key` 和 `metadata_path`）启动 ClearVault，即可通过 WebDAV 接口访问这些已上传的加密文件

## 🛠️ 简单分享功能

ClearVault 支持通过密码加密的 tar 包分享元数据，可直接通过文件传输。简单分享具有以下核心优势：

### 🌟 核心优势：元数据与加密数据分离

**秒级分享**：云端的加密文件可以利用各种云盘的官方分享功能（如阿里云盘、百度网盘、Dropbox 等），通常能实现秒级分享，无需等待 WebDAV 上传。

**无需 WebDAV 服务器**：直接生成加密的元数据 tar 包，无需依赖远程 WebDAV 服务，避免网络不稳定问题。

**充分利用云盘特性**：利用云盘的高速上传、秒传、分享链接等功能，大幅提升分享效率。

**分离设计**：元数据分享包与加密文件完全分离，元数据可以离线传输，加密文件通过云盘分享，两者互不影响。

**绝对安全**：使用 PBKDF2 + AES-256-GCM + RSA-2048 多层加密，确保数据安全。

**零配置分享**：生成的元数据 tar 包可在任何支持文件传输的平台间共享（邮件、云盘、即时通讯等）。

**完全离线**：无需联网即可完成加密分享，适合敏感数据传输。

**即用即弃**：每次分享生成独立的临时密钥对，避免密钥复用风险。

### 导出分享包

```bash
# 指定密码导出
./clearvault export \
    --paths "/documents/report.pdf" \
    --output /tmp/export \
    --share-key "my-secret-password"

# 自动生成随机密码（16位）
./clearvault export \
    --paths "/documents/report.pdf" \
    --output /tmp/export
```

### 导入分享包

```bash
./clearvault import \
    --input /tmp/share_abc123.tar \
    --share-key "my-secret-password"
```

### 分享包结构

```
share_abc123.tar
├── manifest.json          # 清单文件
├── metadata/              # 加密的元数据文件
│   └── 时间戳_随机数_路径哈希_文件名哈希.enc
└── private_key.enc        # 加密的临时私钥
```

### 安全特性

- **PBKDF2**：100,000 次密钥派生迭代，防止暴力破解
- **AES-256-GCM**：元数据加密，提供认证加密
- **RSA-2048**：临时密钥加密，非对称加密保护
- **临时密钥**：每次分享生成新的密钥对，避免密钥复用
- **随机密码**：可自动生成 16 位随机密码，提高安全性
- **随机文件名**：时间戳 + 随机数 + 路径哈希，绝对无碰撞

### 使用场景

1. **秒级云盘分享**：利用云盘官方分享功能，实现加密文件的秒级分享
2. **敏感数据分享**：在不安全的网络环境中传输敏感文件
3. **离线工作**：在没有网络连接的环境下完成数据分享
4. **跨平台分享**：在 Windows、Linux、macOS 之间共享加密数据
5. **备份归档**：将加密的元数据备份到本地或其他存储
6. **临时分享**：快速生成加密包进行临时分享，无需配置 WebDAV

### 分享流程示例

```
1. 本地导出元数据分享包
   ↓
2. 云盘上传加密文件（利用云盘秒传功能）
   ↓
3. 云盘官方分享加密文件链接
   ↓
4. 通过其他渠道发送元数据分享包
   ↓
5. 接收方导入元数据分享包
   ↓
6. 接收方通过分享链接下载加密文件
   ↓
7. 接收方解密并访问原始文件
```

## 🐳 Docker 部署

### 使用 Docker Compose（推荐）

1. **创建配置文件**

创建 `config.yaml`（参考上面的配置示例）

2. **启动服务**
```bash
docker-compose up -d
```

3. **查看日志**
```bash
docker-compose logs -f
```

4. **停止服务**
```bash
docker-compose down
```

### 使用 Docker 命令

```bash
# 运行已发布镜像（推荐）
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/storage:/app/storage \
  ghcr.io/vicnoah/clearvault:latest

# 如需本地构建镜像
docker build -t clearvault:latest .
```

### Docker 环境变量配置

除了使用配置文件，也可以通过环境变量配置：

```bash
# 方案 A：使用配置文件启动
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/storage:/app/storage \
  ghcr.io/vicnoah/clearvault:latest

# 方案 B：完全使用环境变量启动（无需配置文件）
# 生成随机密钥的命令：openssl rand -base64 32
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -e MASTER_KEY="your-generated-base64-key" \
  -e SERVER_LISTEN="0.0.0.0:8080" \
  -e SERVER_AUTH_USER="admin" \
  -e SERVER_AUTH_PASS="your-password" \
  -e REMOTE_URL="https://your-webdav.com/dav/" \
  -e REMOTE_USER="user" \
  -e REMOTE_PASS="pass" \
  -e STORAGE_METADATA_TYPE="local" \
  -v $(pwd)/storage:/app/storage \
  ghcr.io/vicnoah/clearvault:latest
```

支持的环境变量列表（可覆盖 config.yaml 或直接作为配置使用）：
- `MASTER_KEY` (无配置文件启动时必填)
- `SERVER_LISTEN`
- `SERVER_BASE_URL`
- `SERVER_AUTH_USER`
- `SERVER_AUTH_PASS`
- `STORAGE_METADATA_TYPE`
- `STORAGE_METADATA_PATH`
- `STORAGE_CACHE_DIR`
- `REMOTE_URL`
- `REMOTE_USER`
- `REMOTE_PASS`

**注意**：
1. 如果不使用 `config.yaml` 启动，必须手动提供 `MASTER_KEY` 环境变量，否则程序将报错退出。
2. 在有 `config.yaml` 的情况下，环境变量将覆盖文件中的对应配置。


## 🔧 配置说明

### 元数据存储类型

**Local（本地文件系统）**
- 优点：简单、无依赖、易于备份
- 缺点：大量小文件时性能较低
- 适用场景：个人使用、文件数量 < 10000

**SQLite（数据库）**
- 优点：性能好、支持大量文件
- 缺点：需要定期备份数据库文件
- 适用场景：文件数量较多、需要更好性能

### 安全建议

1. **主密钥（master_key）**：
   - 使用至少 32 字节的强随机密码（**推荐留空让程序自动生成**）
   - 妥善保管，丢失后无法恢复数据
   - 自动生成的密钥会回写入 config.yaml，请务必备份此文件

2. **认证密码**：
   - 使用强密码
   - 定期更换
   - 不要与主密钥相同

3. **网络安全**：
   - 生产环境建议使用 HTTPS（通过反向代理如 Nginx）
   - 不要将服务直接暴露到公网
   - 使用 VPN 或 SSH 隧道访问

## 🛠️ 高级功能

### 反向代理配置（Nginx）

```nginx
server {
    listen 443 ssl http2;
    server_name vault.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location /dav/ {
        proxy_pass http://127.0.0.1:8080/dav/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebDAV 特殊配置
        client_max_body_size 0;
        proxy_request_buffering off;
    }
}
```

### 性能优化

1. **元数据存储**：文件数量 > 10000 时建议使用 SQLite
2. **远端存储**：选择网络延迟低的 WebDAV 服务
3. **本地缓存**：可以考虑在前端添加缓存层（如 nginx 缓存）

## 📊 技术实现

详细的技术实现文档请参考：[TECHNICAL.md](TECHNICAL.md)

主要技术特性：
- AES-256-GCM 加密算法
- 文件名随机化（SHA-256 哈希）
- 流式加密/解密
- WebDAV 协议完整实现
- Windows 文件锁定处理
- RaiDrive 客户端兼容性优化

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

## ⚠️ 免责声明

本项目仅供学习和研究使用。使用本软件时：
- 请确保遵守当地法律法规
- 请妥善保管主密钥，丢失后数据无法恢复
- 作者不对数据丢失或安全问题承担责任
- 建议定期备份重要数据

## 📮 联系方式

如有问题或建议，请通过 GitHub Issues 联系。
