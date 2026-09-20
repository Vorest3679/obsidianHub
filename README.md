# ObsidianHub

一个用 Go 编写的轻量知识订阅归档器：RSSHub 负责把 B 站、知乎、X 等平台转换为 RSS/Atom，ObsidianHub 负责定时拉取、去重，并写入 Obsidian Vault。

## 数据流

```text
平台 / 博客 / RSS
        ↓
      RSSHub
        ↓ RSS / Atom / JSON
    ObsidianHub
     ↙       ↘
  SQLite    Obsidian Vault
```

当前版本的最小闭环：

- 支持 RSS、Atom、JSON Feed；
- 支持 RSSHub 相对路径和任意完整 Feed URL；
- 使用 `subscription_id + guid` 去重；没有 GUID 时回退到链接；
- 按订阅源、年份、月份写入 Markdown；
- frontmatter 保存来源、作者、发布时间、原文链接和标签；
- `--once` 适合 cron/systemd，默认模式适合常驻运行。

## 快速开始

本地开发模式需要 Go 1.22+；完整 Compose 模式只需要 Docker。

```bash
cp config.example.json config.json
# 修改 vault_path 和订阅 URL
# 如需知乎路由，把登录 Cookie 放入 .env 的 ZHIHU_COOKIES
# B 站被风控时，把对应账号 Cookie 放入 .env 的 BILIBILI_COOKIE_2267573
docker compose up -d rsshub
go run ./cmd/subhub -config ./config.json -once
go run ./cmd/subhub -config ./config.json
```

也可以完全使用 Docker Compose 运行：

```bash
cp config.example.json config.json
cp .env.example .env
# 编辑 .env，至少设置 OBSIDIAN_VAULT_PATH；需要的平台再填 Cookie
docker compose up -d --build
docker compose logs -f subhub
```

Compose 模式会自动使用容器内的 `http://rsshub:1200`，并把宿主机的 Obsidian Vault 挂载到容器的 `/vault`。数据库保存在 Docker volume `subhub-data` 中。

Compose 使用 `diygod/rsshub:chromium-bundled`，因为部分 B 站路由在接口被风控时会回退到 Playwright；普通 `latest` 镜像不包含浏览器，可能返回 503。

建议先把 `vault_path` 改成 Obsidian Vault 的绝对路径。`database_path` 是去重数据库，不要放进 Vault 也可以。

## 订阅配置

相对路径会拼接 `rsshub_base_url`，完整 URL 则直接访问：

```json
{
  "id": "bilibili-foo",
  "name": "某个 B 站 UP 主",
  "source": "bilibili",
  "url": "/bilibili/user/video/123456",
  "folder": "Bilibili/某个 UP 主",
  "tags": ["来源/Bilibili", "知识输入"],
  "enabled": true
}
```

示例路由：

- B 站投稿：`/bilibili/user/video/{uid}`；
- 知乎用户动态：`/zhihu/people/activities/{id}`；
- X 用户时间线：`/twitter/user/{username}`；
- 博客：直接填写 `https://example.com/feed.xml`。

平台路由和登录要求会变化，实际使用前应在 RSSHub 实例中先打开 Feed URL 验证。尤其是 X、知乎和部分 B 站路由可能需要自建 RSSHub、Cookie 或认证配置。

## 后续适合增加的能力

1. Web UI：添加、暂停、测试订阅源；
2. 失败重试、指数退避和同步状态表；
3. 下载图片/视频缩略图到 Vault 的 `assets` 目录；
4. 内容清洗为更适合 Obsidian 的 Markdown；
5. 按关键词、作者和平台做规则过滤；
6. 用 systemd/Docker 部署，并增加 Prometheus 指标。
