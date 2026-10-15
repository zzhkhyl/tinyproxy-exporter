# 指标

Exporter 暴露的全部 Prometheus 指标。命名空间统一为 `tinyproxy`；此外还有 client_golang 自带的 `go_*` / `process_*` 运行时指标（不需要时可用 ServiceMonitor 的 `metricRelabelings` 丢弃）。

## 来自 StatHost 统计页

Tinyproxy 的统计页只有 5 个计数（见 `src/stats.c` 的 `struct stat_s`），Exporter 把它们规范化后导出：

| 指标 | 类型 | 含义 | 统计页字段 |
| --- | --- | --- | --- |
| `tinyproxy_connections_open` | gauge | 当前打开的连接数 | `num_open` |
| `tinyproxy_requests_total` | counter | 已接受的连接/请求总数 | `num_reqs` |
| `tinyproxy_bad_connections_total` | counter | 坏连接数 | `num_badcons` |
| `tinyproxy_denied_connections_total` | counter | 被拒绝的连接数 | `num_denied` |
| `tinyproxy_refused_connections_total` | counter | 因超出 `MaxClients` 被拒绝的连接数 | `num_refused` |
| `tinyproxy_stathost_info{version,package}` | gauge | 统计页报告的版本信息 | `{version}`/`{package}` |
| `tinyproxy_stats_value{metric="…"}` | gauge | **通用透传**：统计页上任何数值字段都会原样镜像一份 | 任意 |

说明：

- `num_reqs` 在 tinyproxy 内部是**每接受一个连接自增一次**（`src/stats.c:146-149` 的 `STAT_OPEN`），因此它约等于「已接受连接数」，而不是 HTTP 事务数。
- Exporter 自己的统计页抓取也会被 tinyproxy 计入 `num_reqs`/`num_open`，所以这两个数值天然包含抓取开销。
- `num_*` 是**进程内累计值**，tinyproxy 重启会归零。Exporter 检测到回退时会累加偏移量，保证这几个 `_total` 指标单调递增（代价：如果 Exporter 自身重启，偏移量丢失，指标会回退一次，Prometheus 会把那次回退识别为 counter reset）。
- `tinyproxy_stats_value` 的存在意味着**将来 tinyproxy 新增字段也不会瞎掉**，无需改代码即可看到。

## 来自日志

Tinyproxy 日志格式固定为 `级别(左对齐9) Mon DD HH:MM:SS.mmm [pid]: 消息`（`src/log.c:176-182`），Exporter 增量 tail 并分类：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `tinyproxy_log_lines_total` | counter | 读取到的日志行数 |
| `tinyproxy_log_read_bytes_total` | counter | 从日志文件读取的字节数 |
| `tinyproxy_log_parse_errors_total` | counter | 不符合日志行格式的行数 |
| `tinyproxy_log_unrecognized_lines_total` | counter | 格式正确但暂无分类规则的行数（**格式变化的预警信号**） |
| `tinyproxy_log_client_connections_total` | counter | 客户端连接数（`Connect (file descriptor N): IP`，等价于 `STAT_OPEN`）**含**统计页请求 |
| `tinyproxy_log_established_connections_total` | counter | 建立的出站连接数 |
| `tinyproxy_log_closed_connections_total` | counter | 关闭的连接数 |
| `tinyproxy_log_requests_total{method}` | counter | 按 HTTP 方法统计的代理请求数 |
| `tinyproxy_log_requests_by_host_total{host}` | counter | 按目标主机统计的请求数 |
| `tinyproxy_log_denied_total{reason}` | counter | 按原因统计的拒绝数：`acl`、`filter`、`auth`、`connect_port`、`max_clients` |
| `tinyproxy_log_bad_requests_total` | counter | 被拒绝的坏请求（400 / 未知方法） |
| `tinyproxy_log_errors_total{type}` | counter | 内部错误：`client_closed`、`client_headers`、`server_headers`、`upstream_connect`、`upstream_missing`、`send_connect_response` |
| `tinyproxy_log_stathost_requests_total` | counter | 统计页请求数（**不计入**上面的业务请求指标） |
| `tinyproxy_log_request_entity_bytes_total` | counter | 请求体字节数（来自 `Read request entity of N bytes`） |

日志派生的计数在 Exporter 生命周期内**只增不减**，即使日志轮转或被截断也不会归零，因此是合法的 Prometheus counter。

两个容易混淆的点：

- `tinyproxy_log_client_connections_total` 统计**所有**被接受的客户端连接，包含统计页请求（与 tinyproxy 自身的 `STAT_OPEN` 口径一致）；要只看业务请求请用 `tinyproxy_log_requests_total`。
- 日志里 `Request (file descriptor N): …` 只有在请求行被解析出来时才计数；`CONNECT` 方法的请求行形如 `CONNECT host:443 HTTP/1.1`，其 host 会被正确归类到 `requests_by_host`。

## Exporter 自身

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `tinyproxy_up` | gauge | 至少一个启用的数据源健康时为 1 |
| `tinyproxy_exporter_all_sources_up` | gauge | 所有启用的数据源都健康时为 1 |
| `tinyproxy_exporter_source_up{source}` | gauge | 单个数据源健康状态，`source` ∈ `stats`/`log`/`traffic` |
| `tinyproxy_exporter_source_last_success_timestamp_seconds{source}` | gauge | 最近一次成功读取的时间戳 |
| `tinyproxy_exporter_source_scrape_duration_seconds{source}` | gauge | 最近一次读取耗时 |
| `tinyproxy_exporter_source_scrape_failures_total{source}` | counter | 失败次数 |
| `tinyproxy_exporter_build_info{version,revision,goversion}` | gauge | 构建信息 |

## 可选的接口流量

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `tinyproxy_traffic_received_bytes_total{interface}` | counter | 接口接收字节 |
| `tinyproxy_traffic_sent_bytes_total{interface}` | counter | 接口发送字节 |

只有设置了 `TINYPROXY_TRAFFIC_INTERFACE` 才会出现。详见[流量统计](traffic.md)。
