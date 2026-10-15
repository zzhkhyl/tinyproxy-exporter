# Exporter 如何接入 tinyproxy

- **统计页**：`StatHost` 匹配的是请求的 `Host` 头（`src/reqs.c:328-337` 的 `is_stathost`，允许 `:port` 后缀），所以 Exporter 必须**经代理**发起 `GET http://tinyproxy.stats/`——URL 的 host 部分就是触发条件。stat host 请求还可绕过 BasicAuth（`src/reqs.c:1733-1742`）。
- **自定义统计页**：`StatFile` 指向的模板支持 `{[a-z]{1,32}}` 变量替换（`src/html-error.c:126`），本项目随镜像提供 [`../tinyproxy/statpage.txt`](../tinyproxy/statpage.txt)，输出 `key value` 形式的机器可读文本。Exporter 同时兼容三种形态：自定义模板、Alpine 包自带的表格模板（`data/templates/stats.html`）、以及未配置 `StatFile` 时 `src/stats.c:84-107` 硬编码的 `Number of : N<br />` 页面。
- **日志级别选 `Info` 而不是 `Connect`**：`src/log.c:123-130` 显示，`LogLevel Connect` 会丢弃**所有** `LOG_INFO` 行，包括 `Closed connection between local client ...` 和 `Failed auth attempt ...`；`LogLevel Info` 只丢 `DEBUG`，信息最全。
- **日志文件权限**：tinyproxy 在降权之后才创建日志文件（`src/main.c:398`），走 `create_file_safely()` 以 `0600` 创建（`src/utils.c:92`）。因此不同 UID 的 Exporter 容器默认读不到——本项目由 [`../tinyproxy/entrypoint.sh`](../tinyproxy/entrypoint.sh) 预先以 `0644` 创建日志文件解决。该文件以 `O_APPEND` 打开（`src/utils.c:102-103`），重启不会清空。
- **自抓取隔离**：Exporter 自己的统计页请求会被 tinyproxy 记进 `num_reqs`，也会在日志里产生 `Request ... http://tinyproxy.stats/`。日志解析器会识别出 `TINYPROXY_STATS_HOST` 并从业务请求中剔除，单列 `tinyproxy_log_stathost_requests_total`。
