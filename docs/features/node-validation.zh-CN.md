[English](node-validation.md) | 简体中文

# 节点导入、结构验证与订阅策略

结构验证检查能否提取节点身份并转换为规范代理表示。**它不是测速、延迟、连通性、IP 质量或解锁检测。** `success` 不证明服务器或凭据实际可用。网络测量请使用[测速系统](speedtest.zh-CN.md)。

## 导入与兼容性

节点页面通过表单调用 `POST /api/v1/nodes/add-async`，返回 `data.taskId`、`data.task` 和 `data.total`；离开页面后 `node_add` 任务仍继续。输入支持代理 URI、WireGuard 配置文本、含 `proxies:` 的 Clash YAML 和远程订阅 URL。普通 HTTP/HTTPS **代理**链接仍按代理节点处理，不作为订阅下载。

同步接口 `POST /api/v1/nodes/add` 仍受支持，并使用相同导入引擎。两者均接受 `nameMode` 及旧别名 `NameMode`；`custom` 仍是 `remark` 的别名。同步接口在全部重复时保留 `data.skipped: true`、`data.duplicateInfo`，并附加 `data.result`；成功导入时直接在 `data` 返回结构化结果。不要假定所有响应都具有相同嵌套。异步接口先按换行/逗号拆分输入，再调用共享引擎；不要假定旧同步接口具有相同的批量拆分语义。

结果区分 `added`、`skipped`、`invalid`、`failed` 和可选的 `warnings`。节点保存成功但标签分配等辅助操作失败时，结果为部分成功，而非虚假的全量失败。导入可能从远程订阅创建节点；**重验证绝不会这样做**。

## 状态、失效与安全显示

节点提供独立于测速字段的 `ValidationStatus`、`ValidationReason` 和 `ValidationCheckAt`：

| 状态 | 含义 |
|:---|:---|
| `untested` | 没有当前结构验证结果；历史空状态显示为未验证 |
| `success` | 结构通过；远程记录表示抓取的条目通过验证且未报告问题 |
| `partial` | 远程订阅包含可用条目，同时存在解析/验证问题 |
| `failed` | 结构无效、订阅抓取失败或没有有效订阅条目 |
| `disabled` | 已保存的禁用验证状态；不代表连通性结果 |

`Link`、`ClashExtra`、`SkipTLSVerify` 或 `DialerProxyName` 实际变化时，状态重置为 `untested`，原因和验证时间清空。普通编辑、原始字段编辑及批量更新路径均适用。仅元数据修改或无实际变化的修改保留结果。表格和移动端卡片中的原因有长度限制并做脱敏；不要据其措辞推断网络可达性。

## 验证选中节点或当前筛选全部节点

在节点页面使用选中节点验证，或检查确认数量后验证**符合当前筛选的全部节点**。后者不限于当前可见分页。

`POST /api/v1/nodes/revalidate` 需要认证，在演示模式下禁止；JSON 必须使用以下一种形式：

```json
{"ids":[12,18]}
```

```json
{"all":true,"filters":{"validationStatus":"untested","group":"US","countries":["US"]}}
```

- `ids` 不能与 `all` 或 `filters` 共存，即使是 `all:false`。ID 必须为正且存在；重复 ID 去重。空选择报错。
- `all:true` 必须附带 `filters` 对象（`{}` 表示全部节点）。未知选择字段、未知筛选字段和无效状态值报错，不会静默扩大选择范围。
- 筛选复用节点列表解码，包括国家/标签/排除 ID 数组与解锁规则语义。分页不是重验证筛选条件。节点列表、选择器和 ID 列表查询也支持 `validationStatus`：接受 `untested`、`success`、`partial`、`failed`、`disabled`，空值或 `all` 表示不限制；未知验证状态被拒绝。
- 响应包含 `data.taskId`、`data.task` 和 `data.total`。`node_revalidate` 任务启动前固定目标快照；之后新增的节点不会加入。

验证只写现有记录的验证元数据。远程记录会安全抓取并解析，但不会展开为新存储节点。抓取远程订阅属于网络 I/O；不会拨号测试代理节点、测速或探测可用性。抓取失败/无有效条目记为 `failed`；有效和无效条目混合记为 `partial`。

写回要求 ID、验证输入、`UpdatedAt` 及上一次 `ValidationCheckAt` 均与快照一致。编辑（包括编辑后改回）、删除或已写回的重叠验证结果使旧快照失效。旧结果被**跳过**，不会覆盖新节点状态或复活已删除节点。缓存从当前持久化数据刷新。

结果包含 `mode: "revalidate"`、`total`、`processed`、`validated`、`partial`、`skipped` 和 `failed`。`validated` 是成功写入 success/partial 结果的现有记录数；`partial` 是其子集，不能额外相加。不会新增节点。失败记录单独计数。请查看计数及结果 `status`，不能只看任务为 `completed`。

## 任务恢复、取消与有界详情

- 浏览器刷新或离开页面不会取消服务端任务。认证后 UI 从 `/api/v1/tasks/running` hydrate 活动任务，再串行对账活动任务详情，每轮结束五秒后调度下一轮。SSE 提供即时进度，对账补偿漏失事件。登出/卸载会清理轮询。终态通知通过有界近期 ID 集合去重。
- 服务端重启将持久化的 `pending`、`running` 任务标记为 `error`，记录完成时间并刷新任务缓存。**没有自动续跑、自动重试或持久化 worker 队列。** 重启恢复与浏览器刷新恢复不同。
- 可在全局任务 UI 或 `POST /api/v1/tasks/{id}/stop` 停止任务。取消是协作式的，已应用的修改不会回滚。取消后保留最后已处理摘要供回看，但不承诺完整逐项取消报告。远程抓取中途取消不会写入伪造的验证失败。
- 进度更新只在内存保留摘要，不逐项写数据库；nil 更新不会擦除摘要。终态迁移将其持久化。终态持久化失败会显式显示为 `error`，不冒充成功完成。数据库不可用时无法保证持久化成功。
- 导入与重验证的内联结果最多保留 **200 项和 32 条诊断**，汇总计数保持完整。请检查 `truncated`、`omittedItems`、`omittedDiagnostics`（零值时可省略）。进度 SSE 只发送摘要，不发送累计明细数组。临时进度卡片消失后，可通过任务历史/详情回看结果。截断详情不是完整审计导出。

## 订阅验证策略

订阅表单字段 `ValidationPolicy` 会持久化、随订阅复制，并用于已保存/表单预览及分享、原生和转换输出。缺失或空输入默认为 `all`，历史记录亦然；新增/更新/表单预览的未知策略返回 HTTP 400。更新时省略字段也会选择 `all`：需要保留严格策略的客户端必须显式发送它。

| 策略 | 保留的已存储状态 |
|:---|:---|
| `all`（默认） | 全部状态，包括历史空状态 |
| `exclude_failed` | 除 `failed` 外全部状态，包括 `untested`、`partial`、`disabled` |
| `only_success` | 仅 `success` |

这是**已存储记录的结构状态策略**，不是实时可用性检查，也不是对远程展开后每个子节点的认证。策略在节点过滤脚本前随普通筛选执行，脚本后再执行一次；链式依赖在脚本后、渲染前检查。现有输出脚本与 Sub-Store sidecar 仍是可信转换边界，并非沙箱，也不保证最终输出已再次验证。

预览返回 `ValidationExcludedCount`、`ValidationDependencyExcludedCount`，对话框显示两者。已保存预览（`SubscriptionID`）使用已保存策略；未保存预览使用传入的 `ValidationPolicy`。普通渲染响应（包括 HEAD）提供：

- `X-Subscription-Validation-Excluded`：策略排除数量。
- `X-Subscription-Validation-Dependency-Excluded`：额外的链式依赖排除数量。
- `X-Subscription-Diagnostics`、`X-Subscription-Diagnostics-Count`：合并后的安全诊断，最多八条（不是完整失败计数）。

非 `all` 策略排除链式前置节点时，安全省略依赖目标，包括传递性的已存储 dialer 依赖。显式/动态/自定义组前置缺失不会降级为直连或较低优先级规则，也不会重新纳入已排除节点。严格策略的 Clash bridge 不输出未解析/空自定义链式组。`all` 保持原有链式行为。参见[链式代理](chain-proxy.zh-CN.md)。

全部节点被排除时，输出保持无存储节点，**不会回退到全量节点**。模板仍可能输出外壳。现有无效/过期分享的合成通知节点在之后加入，有意不受该策略限制。分享 token 与过期行为不变，参见[订阅分享](subscription-share.zh-CN.md)。

## 远程抓取安全与代理行为

手动远程导入、远程输出、机场根订阅和 provider 抓取共用受控订阅抓取边界。仅接受 HTTP/HTTPS，拒绝 URL userinfo 与私网/回环/保留地址；拨号与每次重定向重新校验 DNS/IP，并固定使用已检查的数字目标拨号。跨主机重定向清除请求头。响应必须为 2xx；解码后的正文（含 gzip）限制为 **4 MiB**，请求预算 **30 秒**，重定向次数有上限。

除非现有 `skipTLSVerify` 明确为相关抓取关闭校验，否则校验证书。该选项不会绕过目标地址限制。机场原有请求头、User-Agent、流量元数据、provider 展开及配置的代理下载仍受支持。配置的代理初始化失败会令请求失败，不会静默直连；代理拨号器接收已检查的数字目标，而非未经检查的主机名。此抓取边界没有生产私网放行开关。

准确请求字段及 curl 示例参见 [API 参考](../../skill-sublinkpro/reference/api.md)。
