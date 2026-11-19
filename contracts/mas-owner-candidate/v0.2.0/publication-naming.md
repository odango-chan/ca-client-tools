# MAS 发布命名与依赖边界

本页是命名提案。这里的路径均从发布方实际控制的 HTTPS 基址开始，未分配正式
Package ID、Schema ID 或发布者身份。

| 发布物 | 建议展示名 | Skill 依赖别名 | Package ID 路径示例 |
| --- | --- | --- | --- |
| 3GPP 来源的通用业务 Schema | 3GPP SBI Common Data | `std-3gpp` | `/packages/3gpp/sbi-common-data` |
| OpenAMSM MAS 业务 Schema | OpenAMSM MAS Schemas | 随 `openamsm` 引用 | `/packages/openamsm/mas-schemas` |
| 可供 Compiler 链接的 MAS Owner 契约 | OpenAMSM MAS Compiler Contract | `openamsm` | `/packages/openamsm/mas-compiler-contract` |
| Host 主体上下文 | CA Host Subject Context | `ca-context` | 沿用该包实际发布的 Stable Package ID |
| 授权及证据信任 | MAS Deployment CapabilityContracts / EvidenceTrustBindings | 不作为 Skill Schema 别名 | 由实际 Deployment 独立发布 |
| 实现说明与验收 | OpenAMSM MAS Provider Runtime Contract | 无 | 独立版本化，不作为静态编译依赖 |

## 上游定义与本次实际发布范围

**3GPP 和 OpenAMSM 可以是原始 Schema 的发布者。** 原样消费已有正式
`$id` 时，保留上游 ID。ToolCatalog 引用 Schema 或编译快照随附其原始
字节，都不重新分配 ID，也不改变该 Schema 的来源和权威。

按固定源码核对后，本次有两类上游定义：

| 固定来源 | 上游可引用定义 | 本次处理 |
| --- | --- | --- |
| TS 29.571 19.6.0，Common Data OpenAPI 1.6.1 | `components/schemas` 下的 `Supi`、`PduSessionId`、`Dnn`、`Snssai`，这些组件没有独立 `$id` | 保留规范和组件路径。原样导入且需要独立 ID 时，由导入发布方分配 ID。固定 MAS 配置额外收窄的字段继续作为配置投影 |
| OpenAMSM MAS Orchestration OpenAPI 1.0.0 | `MasInvokeRequest` 和 `MasInvokeResponse`，以开放的 `inputs` / `outputs` 对象承载数据，组件没有独立 `$id` | 保留线上原定义作为来源。逐操作的关闭对象由 MAS 操作源码和 OES 映射导出，单独标为 Skill 投影 |

例如 3GPP 的 `PduSessionId` 为 0–255，当前 OpenAMSM MAS 初始配置只接受
1–15。这个收窄属于 MAS 配置，不能以 3GPP 原始类型名重新发布。s3 移除
私有引用也属于投影。当前 13 个输入和 12 个结果都是按操作的 Skill 投影，
没有一个可以直接替换为上游通用组件的原始 ID。

机器可读的 [来源登记](schema-provenance.json) 记录上游组件的 JSON Pointer、
固定版本和本次投影范围。Compiler 契约可以消费这些投影，也可以消费实际
发布的上游 Schema。运行时契约和部署授权仍按各自的发布节奏维护。

## ID 和版本

- Skill 别名是局部引用名。`openamsm::SC.CREATE_SM_CONTEXT` 中的
  `openamsm` 由 Skill 选择；Compiler v2.0.2 要求别名以 ASCII 字母开始，
  所以 3GPP 用 `std-3gpp`。产品展示名保留 `3GPP`。
- Stable Package ID 标识包，路径不带发布版本。发布版本、快照摘要和实际
  受信发布绑定锁定某次出版。Schema `$id` 精确到版本和类型定义；版本化
  Schema ID 发布后不得换成另一份内容。
- 如果来源已经为该定义分配了合适的正式 `$id`，原样引用。只有独立导入
  定义或派生投影需要新 ID 时，才使用给新定义分配 ID 的发布方所控制的基址。
  路径保留来源，例如
  `/schemas/3gpp/29.571/19.6.0/Snssai` 或
  `/schemas/openamsm/mas/v1/sc-create-sm-context-result`。
  `3gpp` 和 `openamsm` 在这些路径里说明定义来源。可信绑定负责确认发布者。
- 导入时记录规范版本和具体文件。3GPP Common Data 的示例来源是 TS 29.571
  19.6.0，OpenAMSM 固定源码中的标准版 YAML 标为 OpenAPI 文档版本 1.6.1。
  两个版本字段分别记录。MAS 来源锁定到 env-openamsm 594dec78 和
  `Nsmf_MasOrchestration.yaml` 的 API 版本 1.0.0。
- 投影或适配用独立 ID。例如 s3 的 Skill 公开结果移除了私有
  `sm_context_ref`，可命名为
  `/schemas/openamsm/mas/skill/v1/sc-create-sm-context-result`，
  并记录对原结果的投影。输入类型优先按操作命名，例如
  `sc-create-sm-context-input`。`s3` 保留在 OES 映射中。
  PA CREATE 和 UPDATE 可继续共用同一个 PA 结果 Schema ID。

## 独立出版和当前 Compiler 的组装方式

源 Schema 库和运行时材料可以分别出版。Compiler v2.0.2 在 ToolCatalog
所属依赖中查找 callable 直接引用的输入和输出 Schema。因此
`openamsm` 的编译快照应包含这些精确 Schema 字节，
连同 ToolCatalog、凭证 Schema 和 Predicate 实际依赖的 Evidence 声明。
可从独立 Schema 出版物按 ID 和摘要组装该快照。同一 Build 闭包内避免重复
放入相同的 `$id`。CapabilityContracts 和适用的 EvidenceTrustBindings
由 Deployment 单独提供。

当前 MAS Skill 尚未直接引用一个独立的 3GPP Schema 包。保留
`std-3gpp` 作为将来真实引用时的建议别名；当前 Build 只声明实际使用的依赖。
跨包 Schema 引用应先以真实依赖闭包做链接验证。

## 迁移验收

v0.2.0 已将候选投影改为 `urn:example:openamsm:mas:skill:v0.2.0:<类型>`，
并按操作命名实际文件。完整映射见 [schema-id-migration.json](schema-id-migration.json)。
v0.1.0 保留用于已有测试引用。canonical Skill 的正式迁移仍使用真实发布 ID。

现有 `urn:example:` 和 `mas::` 保留在可复现的测试材料里。正式发布时，
以获批的 Stable Package ID 和 Schema `$id` 同步更新 Skill 的依赖声明、
13 个 callable 引用所在的别名、Predicate 引用、ToolCatalog、快照和
受信绑定。业务 callable ID 和 Capability ID 无需随命名调整重命名。

已用正式 Compiler v2.0.2 验证别名迁移：在临时副本中把 `mas` 改为
`openamsm`，同步更新依赖路径和测试绑定，保留测试 Schema ID；
Check、Inspect、Build 均通过。
