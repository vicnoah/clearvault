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

## 📋 系统要求

- Go 1.21 或更高版本（编译）
- 支持的操作系统：Windows、Linux、macOS
- 远端 WebDAV 存储服务（如 Nextcloud、坚果云等）

## 🚀 快速开始

### 方式一：直接运行（推荐用于开发测试）

1. **克隆仓库**
```bash
git clone https://github.com/yourusername/clearvault.git
cd clearvault
```

2. **编译项目**
```bash
go build -o clearvault ./cmd/clearvault
```

3. **创建配置文件**

创建 `config.yaml`：
```yaml
# 监听地址和端口
listen: "127.0.0.1:8080"

# WebDAV 路径前缀
webdav_prefix: "/dav"

# 认证信息
auth:
  username: "admin"
  password: "your-secure-password"

# 主加密密钥（32字节）
# 如果留空或保持默认值，首次启动时将自动生成安全密钥并回写入此文件
master_key: "CHANGE-THIS-TO-A-SECURE-32BYTE-KEY"

# 元数据存储配置
metadata:
  type: "local"  # 可选: local, sqlite
  path: "storage/metadata"  # local 类型使用此路径
  # db_path: "storage/metadata.db"  # sqlite 类型使用此路径

# 远端 WebDAV 存储配置
remote:
  url: "https://your-webdav-server.com/remote.php/dav/files/username/"
  username: "your-webdav-username"
  password: "your-webdav-password"
  base_path: "clearvault"  # 远端存储的基础路径
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
# 构建镜像
docker build -t clearvault:latest .

# 运行容器
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -v $(pwd)/storage:/app/storage \
  clearvault:latest
```

### Docker 环境变量配置

除了使用配置文件，也可以通过环境变量配置：

```bash
docker run -d \
  --name clearvault \
  -p 8080:8080 \
  -e LISTEN="0.0.0.0:8080" \
  -e WEBDAV_PREFIX="/dav" \
  -e AUTH_USERNAME="admin" \
  -e AUTH_PASSWORD="your-password" \
  -e MASTER_KEY="your-32-byte-master-key" \
  -e REMOTE_URL="https://your-webdav.com/dav/" \
  -e REMOTE_USERNAME="user" \
  -e REMOTE_PASSWORD="pass" \
  -v $(pwd)/storage:/app/storage \
  clearvault:latest
```

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
