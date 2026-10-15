# 运维注意事项

- **日志轮转**：遵循日志文件的大小回退（`copytruncate`）与 inode 变化（`create`）两种轮转方式，也不会因日志缺失而退出（Exporter 可以先于 tinyproxy 启动）。
- **开放的代理**：`tinyproxy.conf` 里 `Allow 0.0.0.0/0` 是为了让 Docker 客户端可用；一旦出现任何 `Allow`/`Deny`，tinyproxy 的默认策略就变成拒绝。生产环境请收紧，例如 `Allow 172.16.0.0/12` + `Allow 127.0.0.1`，并避免把 `8888` 暴露到公网。
- **站点过滤**：限制能访问哪些站点要用 `Filter`（与 `Allow` 是两回事），配置与坑见 [过滤配置](filter.md)。
- **权限**：Exporter 容器以非 root（uid 10001）运行，日志卷以只读方式挂载。
- **不是 root**：tinyproxy 以 root 启动只为绑定端口，随后按 `User`/`Group` 指令降权到 `tinyproxy`。

## 升级镜像

生产 compose 与 Kubernetes 都用 GHCR 的 `latest`。`latest` 会随每次发布移动：

```bash
# 生产 compose
docker compose pull && docker compose up -d

# Kubernetes（Deployment 里已是 imagePullPolicy: Always）
kubectl rollout restart deployment/tinyproxy -n tinyproxy
```
