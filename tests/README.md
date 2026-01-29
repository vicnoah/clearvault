# ClearVault 测试指南

## 测试结构

```
tests/
├── testutil/           # 测试工具包
│   ├── server_manager.go   # 测试服务器管理
│   └── doc.go              # 包文档
├── configs/            # 测试配置文件
│   ├── test-zs3.yaml
│   └── test-sweb.yaml
└── scripts/            # 测试辅助脚本
    ├── start_test_servers.sh
    └── stop_test_servers.sh
```

## 快速开始

### 1. 安装测试服务器工具

```bash
# 安装 zs3 (S3 服务器)
./scripts/install_zs3.sh

# 安装 sweb (WebDAV 服务器)
./scripts/install_sweb.sh
```

### 2. 运行集成测试

```bash
# 运行所有集成测试（自动启动/停止服务器）
./scripts/run_integration_tests.sh

# 带详细输出
./scripts/run_integration_tests.sh -v

# 运行指定测试
./scripts/run_integration_tests.sh -p TestUpload

# 生成覆盖率报告
./scripts/run_integration_tests.sh -cover
```

### 3. 单独运行测试包

```bash
# S3 集成测试
# 服务器会自动启动和停止
go test ./internal/remote/s3 -v

# WebDAV 集成测试
go test ./internal/remote/webdav -v

# 单元测试（不需要服务器）
go test ./internal/crypto -v
go test ./internal/key -v
go test ./internal/api -v
```

## 服务器管理工具

`tests/testutil` 包提供了自动化的测试服务器管理功能。

### 在测试中使用

#### 方式一：使用 TestMain（推荐，用于多个测试）

```go
package mypackage

import (
    "os"
    "testing"
    "clearvault/tests/testutil"
)

var testServerManager *testutil.TestServerManager

func TestMain(m *testing.M) {
    manager, err := testutil.NewTestServerManager()
    if err != nil {
        panic(err)
    }
    
    // 自动启动 zs3 和 sweb
    if err := manager.StartAll(); err != nil {
        panic(err)
    }
    
    testServerManager = manager
    
    // 运行所有测试
    code := m.Run()
    
    // 测试结束后自动停止服务器
    manager.StopAll()
    
    os.Exit(code)
}
```

#### 方式二：使用 EnsureServersForTest（单个测试）

```go
func TestMyFeature(t *testing.T) {
    // 自动启动服务器，测试结束后自动清理
    manager := testutil.EnsureServersForTest(t)
    
    // 获取连接信息
    s3Endpoint := manager.GetZS3Endpoint()
    webdavURL := manager.GetWebDAVURL()
    accessKey, secretKey := manager.GetS3Credentials()
    user, pass := manager.GetWebDAVCredentials()
    
    // 你的测试代码...
}
```

#### 方式三：检查服务器是否运行

```go
func TestWithExistingServers(t *testing.T) {
    // 如果服务器未运行则跳过测试
    testutil.SkipIfServersNotRunning(t)
    
    // 测试代码...
}
```

### 服务器配置

默认配置：

| 服务 | 地址 | 认证信息 |
|-----|------|---------|
| ZS3 (S3) | localhost:9000 | access-key: minioadmin, secret-key: minioadmin |
| SWEB (WebDAV) | localhost:8081/webdav | user: admin, pass: admin123 |

### API 参考

```go
// 创建管理器
manager, err := testutil.NewTestServerManager()

// 检查服务器状态
isZS3Running := manager.IsZS3Running()
isSWEBRunning := manager.IsSWEBRunning()

// 启动服务器
err := manager.StartAll()      // 启动所有
err := manager.StartZS3()      // 仅启动 ZS3
err := manager.StartSWEB()     // 仅启动 SWEB

// 停止服务器
err := manager.StopAll()       // 停止所有
err := manager.StopZS3()       // 仅停止 ZS3
err := manager.StopSWEB()      // 仅停止 SWEB

// 获取连接信息
endpoint := manager.GetZS3Endpoint()           // "localhost:9000"
url := manager.GetWebDAVURL()                  // "http://localhost:8081/webdav"
accessKey, secretKey := manager.GetS3Credentials()
user, pass := manager.GetWebDAVCredentials()

// 创建 S3 bucket
err := manager.CreateS3Bucket("my-bucket")

// 查看日志
logs, err := manager.CheckServerLogs("zs3")    // 或 "sweb"

// 清理测试数据
err := manager.Cleanup()
```

## 测试覆盖情况

### 已完成的测试

| 模块 | 测试文件 | 覆盖率 | 说明 |
|-----|---------|-------|------|
| internal/crypto | asymmetric_test.go | ~90% | RSA 非对称加密 |
| internal/key | manager_test.go | ~95% | 密钥管理 |
| internal/api | api_test.go | ~36% | HTTP API |
| internal/remote/s3 | client_test.go | - | S3 客户端集成测试 |
| internal/remote/webdav | client_test.go | ~92% | WebDAV 客户端集成测试 |
| pkg/gowebdav | *_test.go | ~74% | WebDAV 库测试 |

### 运行测试

```bash
# 所有单元测试
go test ./internal/crypto ./internal/key ./internal/api -v

# 集成测试（需要服务器）
go test ./internal/remote/s3 ./internal/remote/webdav -v

# 带覆盖率
go test ./internal/crypto ./internal/key ./internal/api -cover

# 完整测试套件
./scripts/run_integration_tests.sh
```

## 故障排除

### 服务器启动失败

```bash
# 检查端口占用
sudo lsof -i :9000  # ZS3
sudo lsof -i :8081  # SWEB

# 手动停止残留进程
pkill -f zs3
pkill -f sweb

# 检查日志
cat tests/testdata/zs3.log
cat tests/testdata/sweb.log
```

### S3 测试失败

zs3 服务器对时间格式的支持有限，某些测试可能会失败。这是服务器限制，不影响实际功能。

### WebDAV 测试失败

确保 sweb 正确启动：
```bash
curl -X PROPFIND http://localhost:8081/webdav/
```

## 贡献指南

添加新的集成测试时，请使用 server_manager 工具来管理测试服务器：

1. 导入 `clearvault/tests/testutil`
2. 在 `TestMain` 中调用 `manager.StartAll()`
3. 测试结束后调用 `manager.StopAll()`
4. 使用 `checkS3Available()` 或 `checkWebDAVAvailable()` 检查服务器可用性
