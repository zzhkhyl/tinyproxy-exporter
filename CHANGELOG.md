# 更新日志

本文件记录所有值得注意的改动。格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### 新增

- 站点过滤（白名单）：`Filter` / `FilterType` / `FilterDefaultDeny`，**默认关闭**，按需启用，详见 [docs/filter.md](docs/filter.md)
- 单文件 Kubernetes 清单 `k8s/tinyproxy.yaml`：Namespace / ConfigMap / Deployment / Service / ServiceMonitor / PrometheusRule
- 开发测试（`docker-compose.dev.yml`）与生产（`docker-compose.yml`）两套独立 compose 配置
- 打 tag 自动构建并推送镜像到 GHCR 的 workflow（`.github/workflows/release.yml`）
- push / PR 的 CI 门禁（gofmt / go vet / go test / 清单校验，`.github/workflows/ci.yml`）
- `docs/` 主题文档：指标、接入方式、流量统计、过滤、运维、限制

### 修复

- **默认清单部署后代理不可用**：白名单里没有放行 StatHost，而容器探针（Dockerfile `HEALTHCHECK`、compose healthcheck、K8s liveness/readiness）请求的正是 `http://tinyproxy.stats/`，该请求同样经过过滤器，于是探针持续 403 失败——K8s 下 Pod 永不 Ready、Service 没有 Endpoints，exporter 统计通道同时失联。规则文件现在放行 `tinyproxy.stats`，并把过滤默认关闭
- [docs/filter.md](docs/filter.md) 中"StatHost 不必放行，tinyproxy 在过滤检查之前就应答"的结论与实际行为相反，已改正
- [docs/filter.md](docs/filter.md) 补充 fnmatch 通配语义：`*.example.com` 不匹配裸域名 `example.com`，示例中的裸域名已补齐

### 变更

- 站点过滤默认关闭（`Filter` / `FilterType` / `FilterDefaultDeny` 在 `tinyproxy.conf` 与 K8s ConfigMap 里均为注释状态）：一个代理镜像的开箱行为不应被硬编码域名清单限制
- 两个镜像的版本线解耦：`tinyproxy` 镜像的 tag 跟随上游 tinyproxy 版本（构建前从基础镜像解析，并校验镜像内实际装到的版本一致），`tinyproxy-exporter` 镜像的 tag 跟随手动打的 git tag
- 两个镜像都不再推 `{{major}}.{{minor}}` / `{{major}}` 浮动 tag：`tinyproxy` 只保留完整上游版本（如 `:1.11.3`）与 `:latest`，`tinyproxy-exporter` 只保留完整版本（如 `:0.1.4`）与 `:latest`。前者只是换个下载源的上游二进制，行为完全由上游决定，额外维护一层兼容边界承诺没有意义，反而容易被误读成受控的滚动版本；后者与镜像一一对应，精确 tag 已表达全部信息，`:0.1` 只是个升级补丁时要跟着同步却无信息增量的别名
- `tinyproxy/Dockerfile` 新增 `TINYPROXY_VERSION` 构建参数与 `/etc/tinyproxy/VERSION`，用于写入版本标签并校验镜像内容
- 新增 `make upstream-version`，用于查看基础镜像里 tinyproxy 的上游版本
- 生产镜像来源统一为 `ghcr.io/<owner>/tinyproxy:latest` 与 `ghcr.io/<owner>/tinyproxy-exporter:latest`
- 文档从 README 拆分到 `docs/`，README 只保留总览与索引
- Go module 路径改为 `github.com/zzhkhyl/tinyproxy-exporter`

[Unreleased]: https://github.com/zzhkhyl/tinyproxy-exporter/commits/main
