## 做了什么

<!-- 一句话说明动机与改动范围 -->

## 关联 Issue

<!-- Fixes #123 -->

## 验证方式

<!-- 贴出实际跑过的命令与结果，不要只写"应该没问题" -->

- [ ] `make fmt vet test` 通过
- [ ] `docker compose -f docker-compose.yml config -q` 通过
- [ ] `docker compose -f docker-compose.dev.yml config -q` 通过
- [ ] 手动验证：（说明怎么验的）

## 影响范围

- [ ] 涉及指标新增 / 改名 —— 已同步 `docs/metrics.md` 与 `grafana/dashboard.json`
- [ ] 涉及配置契约（`StatHost` / `StatFile` / `LogFile` / `LogLevel`）—— 已同步 `k8s/tinyproxy.yaml` 里的 ConfigMap
- [ ] 涉及用户可见行为 —— 已在 `CHANGELOG.md` 的 `Unreleased` 补条目
- [ ] 以上都不是
