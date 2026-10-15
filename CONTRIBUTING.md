# 贡献指南

感谢你愿意改进 tinyproxy-exporter。下面是上手所需的最少信息。

## 环境要求

- Go，版本以 [`exporter/go.mod`](exporter/go.mod) 的 `go` 指令为准
- Docker 与 Docker Compose（端到端验证需要）
- 可选：`golangci-lint`，配置见 [`.golangci.yml`](.golangci.yml)

## 本地开发

```bash
make help          # 列出所有目标
make test          # go test -race -count=1 ./...
make vet           # go vet ./...
make fmt           # gofmt -w

# 起一套本地环境（本地构建镜像 + 绑定挂载配置与日志）
make up-dev
tail -f data/logs/tinyproxy.log
curl -s http://127.0.0.1:9119/metrics | grep '^tinyproxy'
curl -s http://127.0.0.1:9119/healthz
```

## 提交前自检

```bash
make fmt vet test
docker compose -f docker-compose.yml config -q
docker compose -f docker-compose.dev.yml config -q
```

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) 会跑同样的检查，另外用 kubeconform 校验 `k8s/tinyproxy.yaml`。

## 改动的约定

- **指标**：新增或改名指标时，同步更新 [`docs/metrics.md`](docs/metrics.md) 与 [`grafana/dashboard.json`](grafana/dashboard.json) 里的查询。CI 不会因此报错，但面板会静默失效。
- **日志分类**：tinyproxy 日志格式变化时要改 `exporter/source/log_parse.go` 的分类规则，并在 `source` 包补测试用例。
- **配置契约**：`StatHost` / `StatFile` / `LogFile` / `LogLevel` 与 exporter 的环境变量是一组契约，见 [`docs/integration.md`](docs/integration.md)。改动时**必须**同步 `k8s/tinyproxy.yaml` 里的 ConfigMap —— 它是 `tinyproxy/tinyproxy.conf` 的手工副本。
- **部署清单**：`docker-compose.yml`、`docker-compose.dev.yml`、`k8s/tinyproxy.yaml` 三者的端口与环境变量名要一起改。
- 提交信息用祈使句，一句话说明做了什么。

## 提交 Pull Request

1. Fork 仓库并新建分支
2. 确认 `make fmt vet test` 通过
3. 涉及用户可见行为时，在 [`CHANGELOG.md`](CHANGELOG.md) 的 `Unreleased` 段落补一条
4. PR 描述里写清动机、验证方式与影响范围（模板会提示）

## 报告问题

请用 [`.github/ISSUE_TEMPLATE/`](.github/ISSUE_TEMPLATE) 下的模板提交，尽量附上 `/healthz` 输出与 `tinyproxy_exporter_build_info` 的取值。

安全问题不要开公开 Issue，走 [`SECURITY.md`](SECURITY.md) 里的私密渠道。
