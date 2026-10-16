# 运维注意事项

- **日志轮转**：遵循日志文件的大小回退（`copytruncate`）与 inode 变化（`create`）两种轮转方式，也不会因日志缺失而退出（Exporter 可以先于 tinyproxy 启动）。
- **开放的代理**：`tinyproxy.conf` 里 `Allow 0.0.0.0/0` 是为了让 Docker 客户端可用；一旦出现任何 `Allow`/`Deny`，tinyproxy 的默认策略就变成拒绝。生产环境请收紧，例如 `Allow 172.16.0.0/12` + `Allow 127.0.0.1`，并避免把 `8888` 暴露到公网。
- **站点过滤**：默认不启用。要限制能访问哪些站点需手动启用 `Filter`（与 `Allow` 是两回事）；启用时白名单必须放行 StatHost，否则健康检查与 exporter 会一起失效，详见 [过滤配置](filter.md)。
- **权限**：Exporter 容器以非 root（uid 10001）运行，日志卷以只读方式挂载。
- **不是 root**：tinyproxy 以 root 启动只为绑定端口，随后按 `User`/`Group` 指令降权到 `tinyproxy`。

## 升级镜像

生产 compose 与 Kubernetes 都用 GHCR 的 `latest`。两个镜像的版本线是独立的，升级时要分别看：

- `tinyproxy-exporter:latest` 随本仓库的每次发布移动（手动打的 `v*` tag）。
- `tinyproxy:latest` 随**上游 tinyproxy 版本**移动，也就是基础镜像的 Alpine 仓库里的包版本。上游升版本时（比如 `1.11.2-r0` 到 `1.11.3-r0`），需要重新触发一次 release workflow 才会同步。

```bash
# 生产 compose
docker compose pull && docker compose up -d

# Kubernetes（Deployment 里已是 imagePullPolicy: Always）
kubectl rollout restart deployment/tinyproxy -n tinyproxy
```

想知道某个 `tinyproxy` 镜像里装的是哪个上游版本（tag 与内容已由构建强制一致，这里只是核对）：

```bash
docker run --rm ghcr.io/<owner>/tinyproxy:latest cat /etc/tinyproxy/VERSION
```

要固定版本而不是跟 `latest`，生产上直接写 tag 即可：exporter 用本仓库的 tag（如 `:1.0.0`），tinyproxy 用上游版本（如 `:1.11.2`）。
