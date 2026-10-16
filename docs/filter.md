# 过滤配置（Filter）

`Filter` 决定**允许访问哪些站点**。它和 `Allow` 是两件不同的事：

| 指令 | 作用 | 管的是 |
| --- | --- | --- |
| `Allow` / `Deny` | 谁能使用这个代理 | 客户端来源地址 |
| `Filter` | 能访问哪些站点 | 目标域名 / URL |

**本仓库默认不启用过滤。** `tinyproxy/tinyproxy.conf` 与 `k8s/tinyproxy.yaml` 里那三行都是注释状态，此时任何能连上代理的客户端都可以访问任意站点，唯一的约束是 `Allow` / `Deny`。要启用见第 1 节，要确认当前状态见第 3 节。

---

## 1. 启用白名单过滤

取消 `tinyproxy/tinyproxy.conf` 里这三行的注释（K8s 改 ConfigMap 里的同名那段）：

```conf
Filter "/etc/tinyproxy/filter"
FilterType fnmatch
FilterDefaultDeny Yes
```

| 指令 | 说明 |
| --- | --- |
| `Filter` | 规则文件的路径，注意是**容器内**路径。文件必须存在，否则 tinyproxy 启动即失败。 |
| `FilterType` | 规则语法：`fnmatch`（shell 通配，最好懂也最快）、`bre`（POSIX 基本正则，默认）、`ere`（POSIX 扩展正则）。 |
| `FilterDefaultDeny` | `Yes` = 白名单（只有命中的放行）；不写或 `No` = 黑名单（命中的拒绝）。 |

> **StatHost 必须放行。** 容器健康检查（Dockerfile 的 `HEALTHCHECK`、compose 的 healthcheck、K8s 的 liveness/readiness 探针）与 exporter 请求的都是 `http://tinyproxy.stats/`，而这个请求**同样会经过过滤器**。
>
> 白名单里漏掉 StatHost（默认是 `tinyproxy.stats`）的后果不是"少一个功能"，而是整体不可用：探针持续失败，K8s 下 Pod 永不 Ready、Service 没有 Endpoints，代理完全无法访问；exporter 的统计通道也会 403，只剩日志通道有数据。仓库自带的规则文件已经把这一条写在最前面，**请保留**。

可选指令：

| 指令 | 说明 |
| --- | --- |
| `FilterCaseSensitive On` | 只影响 `bre` / `ere`；`fnmatch` 恒为大小写敏感。 |
| `FilterURLs On` | 按 URL 而不是域名过滤。只在明文 HTTP 下有效，HTTPS 普及后基本没用，上游也建议不要开。 |
| `FilterExtended On` | 已废弃，等价于 `FilterType ere`。 |

---

## 2. 规则文件

路径由 `Filter` 指定。本仓库的规则文件是 `tinyproxy/filter`，在容器里挂到 `/etc/tinyproxy/filter`。

格式：

- 每行一条规则；`#` 开头是注释（`\#` 转义可当字面量）；空行忽略
- **规则里不能出现空白字符**：解析器遇到空格就把该行截断
- 按 `FilterType` 指定的语法匹配

### fnmatch 的通配不吃裸域名

`fnmatch` 下 `*` 至少匹配一个字符，所以 `*.example.com` **不匹配** `example.com` 本身。要同时覆盖主站和子域，得写两行：

```
example.com
*.example.com
```

这一条在启用白名单时特别容易踩：写了 `*.github.com` 却发现 `github.com` 打不开。`FilterType bre` / `ere` 下同理，`.*\.example\.com$` 也不匹配 `example.com`。

白名单（`FilterDefaultDeny Yes`）示例：

```
# StatHost，必须保留，否则探针与 exporter 全部失败
tinyproxy.stats

# 主站与子域都要写
github.com
*.github.com
pypi.org
*.pypi.org
files.pythonhosted.org
```

黑名单（`FilterDefaultDeny No`）示例：

```
^ads\.example\.com$
.*\.doubleclick\.net$
```

规则写错（正则非法）会让 tinyproxy 启动失败，改完先在本地验证再上线。

---

## 3. 不启用过滤（默认状态）

不过滤是 tinyproxy 的默认行为：**只要配置里没有生效的 `Filter` 指令，就完全不做站点过滤**，任何能连上代理的客户端都可以访问任意站点——此时唯一的约束是 `Allow` / `Deny`。

本仓库发出来的配置就是这个状态，`Filter` / `FilterType` / `FilterDefaultDeny` 三行都是注释。要确认容器里跑的是哪种，看一眼生效的配置：

```bash
# 有输出 = 过滤已启用
grep -E '^\s*Filter' /etc/tinyproxy/tinyproxy.conf
```

另外注意：`FilterDefaultDeny Yes` 只在存在 `Filter` 指令时才有意义；没有 `Filter` 就没有过滤，这个开关不会生效。

此时 `tinyproxy/filter` 文件可以留着不用（仓库默认就是这么发的），也可以从挂载和镜像里去掉。

---

## 4. 把规则文件放进容器

`Filter` 用的是容器内路径，四种部署方式都要把文件投进去，否则 tinyproxy 启动就会失败（`fopen` 失败即 `exit`）。

| 部署方式 | 做法 |
| --- | --- |
| 镜像 | `tinyproxy/Dockerfile` 里 `COPY filter /etc/tinyproxy/filter`（已内置） |
| 开发 compose | `docker-compose.dev.yml` 挂载 `./tinyproxy/filter:/etc/tinyproxy/filter:ro` |
| 生产 compose | `docker-compose.yml` 挂载 `${TINYPROXY_FILTER:-./tinyproxy/filter}:/etc/tinyproxy/filter:ro` |
| Kubernetes | ConfigMap 增加 `filter` 键；Deployment 的 `config` 卷 `items` 增加一项，并挂到 `/etc/tinyproxy/filter` |

K8s 那份 ConfigMap 是逐文件挂载（`items` + `subPath`），所以要像 `tinyproxy.conf` 一样单独列出来，不能只挂目录。

---

## 5. 生效方式

- **只改规则文件**：不必重启容器。发 `SIGHUP`，tinyproxy 会重新读取：

  ```bash
  docker compose exec tinyproxy kill -HUP 1

  # Kubernetes
  kubectl exec deploy/tinyproxy -c tinyproxy -n tinyproxy -- kill -HUP 1
  ```

- **改 `tinyproxy.conf`**：需要重启容器。K8s 里它是以 `subPath` 挂载的，不会热更新，得 `kubectl rollout restart deployment/tinyproxy -n tinyproxy`。

---

## 6. 与监控的关系

被过滤拒绝的请求会被 exporter 统计为：

```
tinyproxy_log_denied_total{reason="filter"}
```

对应日志行是 `Proxying refused on filtered domain "..."`。启用过滤后拒绝率上升、`TinyproxyDenialSpike` 这类告警开始触发，是预期行为。

反过来，Stathost 被误过滤时不会体现在这个计数器上（请求根本没到代理的主流程），而是表现为：

- `tinyproxy_up 0`、`tinyproxy_exporter_source_up{source="stats"} 0`
- 容器探针失败、Pod 反复重启
- exporter 日志里的 `statistics page scrape failed ... unexpected status 403 Filtered`

见到这个组合基本可以直接怀疑白名单漏了 `tinyproxy.stats`。

---

## 7. 常见坑

- **StatHost 漏放行 = 整体不可用**。上面反复强调的那条：探针、K8s Service Endpoints、exporter 统计通道会一起挂掉，而且报错不会指向"过滤配置"。
- **`*.example.com` 不匹配 `example.com`**，见第 2 节。
- **文件不存在 = 启动失败**。`src/filter.c` 里 `fopen` 失败直接 `exit(EX_DATAERR)`：容器起不来，K8s 表现为 `CrashLoopBackOff`。
- **正则非法 = 启动失败**。规则文件里的坏规则同样会让进程退出（`Bad regex in ...: line N`）。
- **规则里不能有空格**，否则会被静默截断成半截规则。
- **白名单与 `Allow 0.0.0.0/0` 不矛盾**：一个管目标站点，一个管客户端来源，彼此独立。
