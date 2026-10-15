# 流量统计：tinyproxy 能给什么，不能给什么

这一点必须说清楚，因为它影响你能得到什么指标。

**结论：tinyproxy 本身不统计、也不记录任何字节数。**

证据（均在提供的上游源码中）：

- `src/stats.c:38-44` 的 `struct stat_s` 只有 `num_reqs`、`num_badcons`、`num_open`、`num_refused`、`num_denied` 五个字段，没有任何字节累计。
- 全 `src/` 目录没有对收发字节的计数或打点（`buffer.c` 里的 `bytesin`/`bytessent` 只是当次读写的局部变量，用完即弃）。
- 统计页模板可用的变量只有 `opens`、`reqs`、`badconns`、`deniedconns`、`refusedconns`（`src/stats.c:119-127`），因此**也无法通过自定义模板把字节数变出来**。

所以本项目提供两条诚实的替代路径：

1. **请求体字节（部分真实）**：日志里的 `Read request entity of N bytes` 会被累计到 `tinyproxy_log_request_entity_bytes_total`。它只覆盖**带请求体的请求**，不是全部流量。
2. **接口字节（可选）**：设置 `TINYPROXY_TRAFFIC_INTERFACE` 后读取 `/proc/net/dev`。这组数字只有在 **Exporter 与 tinyproxy 处于同一网络命名空间**时才代表代理流量。做法是把 exporter 服务的 `ports` / `networks` 换成：

   ```yaml
   services:
     tinyproxy-exporter:
       # 删除 ports 与 networks，改为：
       network_mode: "service:tinyproxy"
       environment:
         TINYPROXY_TRAFFIC_INTERFACE: "eth0"
   ```

   此时 Exporter 复用 tinyproxy 的 netns，`/proc/net/dev` 的计数即该代理容器的真实收发量，端口 `9119` 也由 tinyproxy 容器的 IP 暴露（`9119:9119` 映射需要挪到 tinyproxy 服务上）。

未启用时这些指标**直接缺席**，而不是伪造 0。
