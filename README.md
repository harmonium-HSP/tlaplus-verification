# Redlock Fencing Demo

[![TLA+ Verification](https://github.com/你的用户名/distributed-labs/actions/workflows/tlaplus-verify.yml/badge.svg)](https://github.com/你的用户名/distributed-labs/actions/workflows/tlaplus-verify.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/你的用户名/distributed-labs)](https://goreportcard.com/report/github.com/你的用户名/distributed-labs)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

一个完整的分布式锁验证项目，包含多种分布式锁算法的形式化验证、Go 实现和混沌测试。

## 🚀 项目亮点

- **形式化验证**：使用 TLA+ 验证 Redlock、Raft、Paxos、Lease Lock 的安全性
- **生产级实现**：经过验证的 Go 代码实现
- **混沌测试**：验证在故障场景下的健壮性
- **Kubernetes Operator**：支持在 Kubernetes 上部署分布式锁集群
- **CI/CD 集成**：自动运行模型检查和测试

## 📁 项目结构

```
redlock-fencing-demo/
├── models/                    # TLA+ 形式化模型
│   ├── redlock/              # Redlock 算法模型
│   ├── raft/                 # Raft 选举协议模型
│   ├── paxos/                # Paxos 共识协议模型
│   └── lease-lock/           # 租约锁模型
├── pkg/                      # Go 实现
│   ├── redlock/              # Redlock 实现
│   ├── raft/                 # Raft 实现
│   ├── paxos/                # Paxos 实现
│   ├── leaselock/            # Lease Lock 实现
│   ├── fencing/              # Fencing Token 实现
│   └── storage/              # Redis 存储层
├── operators/                # Kubernetes Operators
│   ├── redlock-operator/     # Redlock 集群 Operator
│   └── raft-operator/        # Raft 集群 Operator
├── deployments/              # 部署配置
│   ├── kubernetes/           # Kubernetes 部署文件
│   └── docker-compose.yml    # Docker Compose 配置
├── scripts/                  # 自动化脚本
├── docs/                     # 文档
│   ├── architecture.md       # 架构文档
│   ├── verification.md       # 验证文档
│   ├── chaos-testing.md      # 混沌测试文档
│   └── anti-examples/        # 反例分析文档
├── configs/                  # 配置文件
└── cmd/                      # 命令行工具
```

## 🔧 快速开始

### 环境要求

- Go 1.21+
- Java 11+ (用于 TLA+ 模型检查)
- Docker/Docker Compose
- Kubernetes (用于 Operator 部署)

### 安装依赖

```bash
go mod tidy
```

### 运行测试

```bash
# 运行所有单元测试
make test

# 运行混沌测试
make chaos

# 运行特定协议测试
make test-redlock
make test-paxos
make test-leaselock
```

### 运行 TLA+ 验证

```bash
# 验证所有协议
make verify-all

# 验证特定协议
make verify-redlock
make verify-paxos
```

### 使用 Docker Compose 启动测试环境

```bash
make docker-up
make docker-down
```

## 📊 支持的协议

| 协议 | 状态 | 验证类型 |
|------|------|---------|
| **Redlock** | ✅ 已验证 | TLA+ + 混沌测试 |
| **Raft 选举** | ✅ 已验证 | TLA+ + 混沌测试 |
| **Paxos** | ✅ 已验证 | TLA+ + 混沌测试 |
| **Lease Lock** | ✅ 已验证 | TLA+ + 混沌测试 |

## 🔐 Fencing Token 机制

项目实现了完整的 Fencing Token 机制，防止分布式锁场景下的陈旧写入：

```go
// 获取锁时同时获取 fencing token
token, err := redlock.Lock(ctx, "resource")
if err != nil {
    return err
}

// 写入前验证 token
err := writer.Write(ctx, "key", data, token)
if err == fencing.ErrStaleToken {
    // token 已过期，需要重新获取锁
}
```

## 🧪 混沌测试

项目包含全面的混沌测试套件：

| 故障类型 | 测试场景 |
|---------|---------|
| **网络分区** | 模拟节点间网络隔离 |
| **延迟注入** | 模拟网络延迟 |
| **随机故障** | 模拟随机节点故障 |
| **高并发** | 大量并发请求 |
| **租约过期竞态** | 租约过期时的竞争条件 |

## 🚢 Kubernetes 部署

### 部署 Redlock Operator

```bash
make deploy-redlock-operator
kubectl apply -f examples/redlock-cluster.yaml
```

### 部署 Raft Operator

```bash
make deploy-raft-operator
kubectl apply -f examples/raft-cluster.yaml
```

### 查看状态

```bash
make status
```

## 📈 CI/CD 集成

项目集成了 GitHub Actions 和 GitLab CI：

- **PR 检查**：自动运行模型检查和测试
- **性能回归检测**：监控模型检查状态数变化
- **Slack/Confluence 通知**：验证失败时自动通知团队

## 📝 文档

- [架构文档](docs/architecture.md)
- [验证文档](docs/verification.md)
- [混沌测试文档](docs/chaos-testing.md)
- [反例分析](docs/anti-examples/)

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License

---

## 命令参考

```bash
# 测试命令
make test                    # 运行所有单元测试
make test-all                # 运行所有测试
make test-redlock            # Redlock 测试
make test-paxos              # Paxos 测试
make test-leaselock          # Lease Lock 测试
make chaos                   # 运行所有混沌测试
make chaos-redlock           # Redlock 混沌测试

# 验证命令
make verify-all              # 运行所有 TLA+ 验证
make verify-redlock          # Redlock 验证
make verify-raft             # Raft 验证
make verify-paxos            # Paxos 验证

# 构建命令
make build                   # 构建二进制文件
make docker-build-redlock-operator  # 构建 Redlock Operator 镜像
make docker-build-raft-operator     # 构建 Raft Operator 镜像

# 部署命令
make deploy-redlock-operator # 部署 Redlock Operator
make deploy-raft-operator    # 部署 Raft Operator
make create-redlock-cluster  # 创建 Redlock 集群
make create-raft-cluster     # 创建 Raft 集群

# 状态命令
make status                  # 查看集群状态

# 清理命令
make clean                   # 清理构建产物
make docker-down             # 停止 Docker 容器
```
