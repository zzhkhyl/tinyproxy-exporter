# 更新日志

本文件记录所有值得注意的改动。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 新增

- 站点过滤（白名单）：`Filter` / `FilterType` / `FilterDefaultDeny Yes`，详见 [docs/filter.md](docs/filter.md)
- 单文件 Kubernetes 清单 `k8s/tinyproxy.yaml`：Namespace / ConfigMap / Deployment / Service / ServiceMonitor / PrometheusRule
- 开发测试（`docker-compose.dev.yml`）与生产（`docker-compose.yml`）两套独立 compose 配置
- 打 tag 自动构建并推送镜像到 GHCR 的 workflow（`.github/workflows/release.yml`）
- push / PR 的 CI 门禁（gofmt / go vet / go test / 清单校验，`.github/workflows/ci.yml`）
- `docs/` 主题文档：指标、接入方式、流量统计、过滤、运维、限制

### 变更

- 生产镜像来源统一为 `ghcr.io/<owner>/tinyproxy:latest` 与 `ghcr.io/<owner>/tinyproxy-exporter:latest`
- 文档从 README 拆分到 `docs/`，README 只保留总览与索引
- Go module 路径改为 `github.com/zzhkhyl/tinyproxy-exporter`

[Unreleased]: https://github.com/zzhkhyl/tinyproxy-exporter/commits/main
