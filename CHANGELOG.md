# 更新日志

## 4.0.0-beta.1

### 新功能与变化

1. **新版邮箱界面**：React SPA 提供桌面三栏、移动端布局、暗色模式、虚拟列表、星标和服务器草稿。
2. **域名 DNS 向导**：逐条检查 MX、地址、SPF、DKIM、DMARC 等，提示如何合并 SPF，并导出 BIND 区域片段。
3. **首次注册密钥**：空库且未设置 `PMAIL_ADMIN_PASSWORD` 时，从 `docker compose logs pmail | grep 注册密钥` 获取一次性密钥并在 HTTPS 网页注册。
4. **中文与英文提示**：后端按用户语言或请求协商返回提示，退信也按收件用户偏好生成。
5. **移除插件**：不再加载可执行插件，插件设置、微信推送及外部垃圾邮件分类插件停用。内置 catch-all SPF/DKIM 过滤不变。
6. **代码邮件与附件预览**：保留常见代码结构、缩进和补丁；支持 Markdown 围栏、代码高亮及安全的文本附件预览。
7. **前后端独立构建、同镜像部署**：React 与 Go 以 OpenAPI 为契约，网页产物运行时从 `/work/web` 加载，不再嵌入 Go 二进制。

### 升级注意事项

- 先备份数据库与配置卷。已有账号直接登录，不重新注册。`initial_admin_password` 不再生成；已有文件不会自动删除。
- 不再读取插件配置，不做破坏性数据迁移。若依赖插件通知或分类，请在升级前安排替代方式。
- 附件迁至 `/api/attachments/{id}` 和 `/api/attachments/download/{id}`，兼容旧 `/attachments/*`；文本预览为 `/api/attachments/preview/{id}`。
