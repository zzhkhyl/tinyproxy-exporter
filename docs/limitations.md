# 已验证内容与已知限制

已在本地实测通过：

- `gofmt` / `go vet` / `go test -race ./...` 全部通过；单元测试覆盖三种统计页形态、真实日志行分类（含日志轮转、截断、半行、主机基数上限）、`/proc/net/dev` 解析与配置校验。
- 两个镜像均可构建；`docker compose up` 后双容器正常启动，统计页返回自定义模板内容，`/healthz` 与 `/metrics` 输出符合预期。
- 日志卷跨容器可读（tinyproxy 侧 0644，Exporter 以不同 UID 只读挂载）。
- 端到端产生代理流量后，`tinyproxy_log_requests_total{method}`、`tinyproxy_log_requests_by_host_total{host}`、`established`/`closed` 计数与日志内容一致，`parse_errors` 为 0。

已知限制：

- **无原生流量字节指标**，原因与替代方案见[流量统计](traffic.md)。
- `tinyproxy_requests_total` 来自统计页，含义是「已接受连接数」且含 Exporter 自身的抓取；需要纯粹的请求数请使用 `tinyproxy_log_requests_total`。
- `TINYPROXY_LOG_MAX_HOSTS` 之外的 host 会归并到 `__other__`，会丢失长尾分布细节。
- 统计页计数器依赖「数值回退 = 进程重启」的推断；若 Exporter 自身重启，偏移量会丢失并导致这几个 counter 回退一次（Prometheus 会按 counter reset 处理）。
