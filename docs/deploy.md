# 部署到 armbian 主机

目标：在 **armbian-1**（aarch64 / 4 核 / 3.8G 内存 / eMMC 剩约 14G）上跑起来，
通过 tailnet 访问 `http://100.70.0.29:8090`。

> 全部命令都在**宿主机**上执行。1Panel 的 MCP 通道没有 shell，所以本文件的命令
> 由你复制粘贴执行；每一步都给了「期望输出」，对不上就先别往下走。

---

## 0. 部署前必验三项

```bash
docker compose version
```
**期望**：打印 `Docker Compose version v2.x`。
若报 `docker: 'compose' is not a docker command`，说明只有独立的 `docker-compose`
（这台机器上确实是这种情况）——`install.sh` 已自动兼容，无需处理。

```bash
tailscale status --json | grep -o '"CertDomains":\[[^]]*\]'
```
**期望**：出现带域名的数组，例如 `["armbian-1.tailbae726.ts.net"]`。
空数组表示 tailnet 还没启用 HTTPS 证书 —— 不影响首次部署（我们用 http），
但以后想装 PWA 就需要它（见第 6 节）。

```bash
curl -sI --max-time 8 'https://t2.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=https://github.com&size=64' | head -1
```
**期望**：`HTTP/2 200`（或任何非 000 的状态码）。
不通的话服务能跑，但所有图标都会退化成纯色文字兜底。

顺手确认端口没被占：
```bash
ss -tlnp | grep -E ':(8090)\b' || echo "8090 空闲"
```

---

## 1. 一键部署

```bash
curl -fsSL https://raw.githubusercontent.com/ffdkj/my_nav/v0.1.0/deploy/install.sh -o /tmp/my_nav-install.sh
sudo bash /tmp/my_nav-install.sh
```

脚本是幂等的，做这几件事：

1. 检查 docker / compose，并给出"用哪个 compose 命令"的结论
2. 建目录 `/opt/1panel/docker/compose/my_nav`（与这台机器上其他 9 个 compose 项目一致，1Panel 面板能识别）
3. 下载 `compose.yaml`，写 `.env`（**已存在则不覆盖**）
4. `docker pull` 镜像并 `up -d`
5. 等健康检查，打印访问地址与后续确认清单

常用覆盖项：

```bash
sudo NAV_PORT=8091 bash /tmp/my_nav-install.sh          # 换端口
sudo IMAGE=ghcr.io/ffdkj/my_nav:edge bash /tmp/my_nav-install.sh   # 用最新开发版
```

### 手工方式（等价）

```bash
sudo mkdir -p /opt/1panel/docker/compose/my_nav/data
cd /opt/1panel/docker/compose/my_nav
sudo curl -fsSLO https://raw.githubusercontent.com/ffdkj/my_nav/v0.1.0/deploy/compose.yaml
sudo curl -fsSL  https://raw.githubusercontent.com/ffdkj/my_nav/v0.1.0/deploy/.env.example -o .env
sudo sed -i 's/^NAV_BIND_IP=.*/NAV_BIND_IP=100.70.0.29/' .env
sudo docker compose up -d    # 或 sudo docker-compose up -d
```

---

## 2. 验收清单

```bash
docker ps --filter name=my_nav --format '{{.Status}}\t{{.Ports}}'
# 期望：Up ... (healthy)   100.70.0.29:8090->8080/tcp
```

- [ ] `http://100.70.0.29:8090` 能打开，看到网格与「+ 添加」图块
- [ ] 添加 `https://github.com` → 几秒内出现**真实图标**（不是纯色字母；若是纯色字母请看第 6.5 节）
- [ ] 手机上打开同一地址，**按住 300ms** 能拖动图标，松手吸附
- [ ] 拖一个图标压到另一个上停住约 0.5 秒 → 合并成文件夹
- [ ] 底部圆点能加第二页，滚轮/横滑能翻页
- [ ] 设置 →「数据」能导出 JSON；导入同一份后数据不变
- [ ] `docker logs my_nav` 没有反复报错

---

## 3. 升级 / 回滚

```bash
cd /opt/1panel/docker/compose/my_nav
sudo sed -i 's|^    image: .*|    image: ghcr.io/ffdkj/my_nav:0.2.0|' compose.yaml   # 或改 .env 里的版本
sudo docker compose pull && sudo docker compose up -d
```

回滚就是把 `image:` 改回旧版本号再 `up -d`。
**数据库 schema 变更只在启动时向前迁移**，所以回滚到旧镜像前请先按第 4 节导出一份。

---

## 4. 备份与恢复

应用内：设置 →「数据」→ 下载 JSON 或 zip。
**导入前会自动在服务器留一份 `/opt/1panel/docker/compose/my_nav/data/backup/pre-import.json`，永久只保留最新一份。**

要备份"整个服务"，直接打包数据卷（SQLite 开了 WAL，**旁车文件要一起拿**）：

```bash
cd /opt/1panel/docker/compose/my_nav
sudo docker compose stop
sudo tar czf /root/my_nav-data-$(date +%F).tar.gz -C . data
sudo docker compose start
```

> 更稳的做法是先让 SQLite 自己产出一致快照：
> `docker exec my_nav /nav -healthcheck` 只探活，不能导库；
> 因此这里的 stop → tar → start 是最省心且一致性有保证的方式。

恢复就是把 `data/` 解回去再 `up -d`。

---

## 5. 数据卷为什么必须放本地盘

SQLite 的 WAL 依赖共享内存与 POSIX 文件锁。放在 NFS/CIFS/SMB 上会出现
"随机损坏 / SQLITE_BUSY"。这台机器的 `/` 是 eMMC 上的 ext4，属于本地盘，没问题。

另外容器的 `/` 是只读的（`read_only: true`），只有 `/data` 与 `/tmp` 可写 ——
这是刻意的，能挡住大部分"莫名其妙写坏镜像层"的情况。

---

## 6. 可选：启用 PWA（需要 HTTPS）

Service Worker 只在**安全上下文**注册，`http://ip:port` 不是。
这台机器的 80/443 已经被既有 Caddy（vaultwarden）占用，所以不要动它，
走一个高位端口：

```bash
cd /opt/1panel/docker/compose/my_nav
sudo sed -i 's/^NAV_BIND_IP=.*/NAV_BIND_IP=127.0.0.1/' .env
sudo docker compose up -d
sudo tailscale serve --bg --https=8443 http://127.0.0.1:8090
sudo tailscale serve status          # 确认 443→8443 的映射
```

之后用 `https://armbian-1.tailbae726.ts.net:8443` 访问，手机上即可"添加到主屏幕"，
离线也能打开（在线看过的数据会缓存）。

想还原成 IP 直连：`sudo tailscale serve --https=8443 off`，再把 `.env` 改回 `100.70.0.29`。

---

## 6.5 容器出网：图标能不能显示真实图标的前提

**症状**：添加任何网址，图标都是纯色字母；连 `https://www.baidu.com` 也一样。

**根因**（这台机器上实测确认）：宿主用**透明代理**（clash/shellcrash 的 TUN/TPROXY 模式）
接管出网，而 docker bridge 网络的流量**不在其内**。于是容器：

- 直连出网：被拦（连国内站都不通）
- 指向代理端口：**不可能** —— TUN 模式下代理根本没有监听端口
  （实测该机上唯一像代理的 `100.70.0.29:8790` 其实是个返回 JSON 的 API，HTTP/SOCKS 都连不上）

**修法**：让容器**共用宿主网络栈**，它的流量就会走进宿主已有的代理链路。

```bash
sudo NETWORK=host bash /tmp/my_nav-install.sh
```

或者手工把 compose 里的 `ports:` 换成：

```yaml
    network_mode: host
    environment:
      NAV_ADDR: "100.70.0.29:8090"   # host 网络下端口映射不生效，直接指定绑定地址
```

完整文件见 [`deploy/compose.host-network.yaml`](../deploy/compose.host-network.yaml)。

**代价**：没有网络隔离、也没有端口映射（直接绑到指定地址）。对个人 tailnet 服务可以接受。

**验证**：在编辑面板点「重新抓取」，图标状态从 `miss` 变成 `ok` 即说明出网通了。

> 另一种环境（有可用的 HTTP 代理端口，比如 clash 的 mixed-port 7890）不需要 host 网络：
> 给容器加 `HTTPS_PROXY=http://<宿主可达地址>:7890` 即可 —— 抓取客户端会读这个变量
> （`NO_PROXY` 也支持）。注意：**配了代理后，拨号层的 SSRF 私网拦截会自动让位** ——
> 因为此时拨号对象是代理本身，由代理负责出网策略。

---

## 7. 故障排查

| 症状 | 原因 / 处理 |
|---|---|
| `manifest unknown` | 镜像标签写错。可用标签：`0.1.0`、`0.1`、`latest`、`edge`（`edge` 跟 main 分支） |
| 容器一直 `starting` | 看 `docker logs my_nav`；多半是 `/data` 权限（镜像以 uid 65532 运行，宿主目录需要可写） |
| 页面能开但没有图标 | **先看第 6.5 节**：多半是容器没有出网，用 `NETWORK=host` 重装即可。临时也可上传本地图标 |
| 端口占用 | `sudo NAV_PORT=8091 bash install.sh` |
| 局域网其他设备打不开 | 正常：只绑定了 Tailscale 虚拟 IP。要不就改 `.env` 的 `NAV_BIND_IP` |
| 想重置一切 | 删掉 `data/` 再 `up -d`（会重新初始化空库） |

---

## 8. 已知限制

- **无应用内认证**：任何能连上 tailnet 的设备都有完整读写权限（这是当初的决策）。
- 数据库文件与图标/壁纸都在同一个 `data/` 卷里，一起备份即可，但也意味着**单点**。
- 自动备份只有"导入前快照"这一份，没有定时快照（当时明确只保留一份）。
- 缩略图是 JPEG 而不是 WebP（纯 Go 没有 WebP 编码器，理由见 `docs/spec.md` 第 8 节）。
