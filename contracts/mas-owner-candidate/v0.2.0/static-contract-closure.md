# MAS v1alpha5 静态契约评审稿

适用范围为 ca-skills PR #74 的 13 个操作和固定 IPv4 / SSC1 / LOCAL_POOL /
无二次鉴权配置。源行为固定在 env-openamsm 594dec78。以下语义是拟冻结的
Owner 契约输入。状态为待评审，不可作为可信 DependencySnapshot 导入。

## 公开结果与写入边界

所有操作按 MUTATION 设计。这里的 Effect 包含 Owner 的本地会话状态与外部
写入。成功输出的字段保持候选 Schema 类型，表中的含义约束可声称的事实。

| 步骤 | 写入边界 | 成功证据允许表达的事实 |
| --- | --- | --- |
| s1 | 可建立 SMF UE/Session shell 并更新订阅状态 | sub_summary=ok 只表达订阅校验完成 |
| s2 | 写入 SAA 阶段、未决标记和 EAP 轮次 | phase=SKIPPED 只表达已确认适用的配置跳过 DN-AAA |
| s3 | 写入 SM Context、本地生命周期及资源关联 | session_keys 和 OPERATIONAL 表达本地 SC 上下文创建。sm_context_ref 只进私有上下文。没有外部响应流也可达到 OPERATIONAL |
| s3r | 写入 UECM 注册及本地注册状态 | 核实该 SUPI/PSI 与业务代次的 UECM 注册。无 UE 的本地成功分支不能出具该证据 |
| s3p | 建立 PCF 策略关联并写入 PA 状态 | 非空且非 bound 的 policy_assoc_id，结合本次 PCF 创建确认。已绑定分支的 ID 不能证明新建 |
| s4 | 选择并写入 UP 锚点状态 | up_anchor=1，锚点已选定 |
| s5 | 分配地址并写入 IAM 状态 | 验证本会话的 LOCAL_POOL IPv4 租约。ue_addr=1 为分配标记，IPv6-only 状态不满足 |
| s6u | 更新已有 PCF 关联及 PA 状态 | 同一真实关联上的本次更新，不能只凭既有 policy_assoc_id |
| s7 | 写入本地 CA skip 状态 | charging_ref=1，跳过占位收费步骤；没有收费预留 |
| s8 | 建立 N4/PFCP 会话并写入 UPN 状态 | 核对该次 PFCP 建立确认与 UPF SEID；已有 SEID 不能证明本次建立 |
| s10 | 发送 N1/N2 并记录匹配的 AN 隧道 | 由原操作匹配的 UpdateSMContext 确认非零 TEID 和有效地址。旧隧道或挂起状态不能证明本次完成 |
| s11 | 更新 N4 会话的 AN 隧道并写入 N3 同步状态 | 验证该次 PFCP 修改确认、原 SEID 及请求的同一 F-TEID。既有 n3_synced 不能证明本次同步 |
| s13 | 写入本地事件状态 | last_event=1，本地事件已记录；没有 3GPP Step 18 通知含义 |

初始 CreateSMContext 已由上游准入。Host 只做原始请求认证、去重、SUPI/PSI
关联和竞争写入裁决。入口没有额外的 AMF 注册、CM CONNECTED 或 SMF Session
ABSENT 门槛。s1 的 shell 不构成 PDU Session 建立证据。

无二次鉴权是固定配置的适用条件。派发 s2 前，部署应以受信策略绑定
已准入的 SUPI/PSI、DNN、S-NSSAI 和当前业务实例，并由 Owner 确认无冲突的
订阅或策略要求。缺失、过期或冲突时停止，禁止派发固定 required="0" 的 s2。
s2 的输入和 SKIPPED 均不能反向证明不需要鉴权。该校验只涉及 MAS 配置适用性。

## 提交与证据契约

- SUCCEEDED 是最小 Owner Outcome。只有能证明原调用 NOT_COMMITTED 且
  无遗留副作用时，才可声明 FAILED 类别及对应证明路径。FAILED 不可重试。
  当前 Skill 有 FAILED 路由，fixture 为该路由提供明确标记的合成失败类。
  正式快照必须逐项给出真实类别，或同步移除相应 Skill 的 FAILED 路由。
  每个 MUTATION 的
  receipt.commit 都引用根封闭的 proof Schema，必填 originalInvocationId 和
  commitState，后者枚举 COMMITTED 与 NOT_COMMITTED。ToolCatalog 的
  mutation.commitProof.bind 固定为 originalInvocationId。
- 对外声明 SUCCEEDED 时，Owner 必须能验证该次原始调用的 COMMITTED
  凭证，且公开结果必须满足该操作的 Schema。业务 Predicate 单独判断
  本次结果是否满足目标。无法证明提交状态的错误走 OUTCOME_UNKNOWN，
  不能靠 FAILED 判定原调用安全结束。
- 同一原始调用的已验证提交或结果可重放。对不同调用，不可用同一会话的
  已有状态代替本次写入证明。若未来需要“已满足”语义，应单独定义允许的
  效果与证据，并复核 Skill 的 Predicate。未知结果不自动重试。
- 当前契约没有经证明的可重试失败类。HTTP 200/OK 是线缆观测。派发后缺少
  精确证明保持 OUTCOME_UNKNOWN。DEFERRED 要求可恢复 Owner 句柄，原始 HTTP
  waiter 不满足。DENIED 属于运行侧拒绝。最小 Skill 编译配置不声明 Reconcile
  callable，当前 Procedure 也没有 Reconcile 节点。将来声明时使用同一 Owner
  的独立 READ 与只接收 originalInvocationId 的输入，按原始操作逐项指定证明
  和可恢复输出。补偿与重发不属于 Reconcile。
- 每条 Evidence Guarantee 绑定本次 SUCCEEDED 的准确 callable 和公开
  output；选择 exact，排序字段为 observedAt，关联 invocationId 和
  producerInvocationId。单次调用的事实还必须把原生产调用纳入 Scope。
  factValue 比较完整的规范化公开结果：同一次调用出现不同策略关联，
  或同一 TEID 配上不同 IP，都应判为冲突。另一调用的记录不得参与选择。
  Owner 必须
  在私有证据记录中验证并记录准确 SUPI、PSI、部署和业务实例代次，并给出
  实际 observedAt、最大可接受时效及来源。只有同一次精确派发的证据参与
  Predicate 判定。s2 的 PSI 来自已验证的私有会话范围。
- 发布时以不可变快照提供 mas ToolCatalog 和数据 Schema、Evidence Guarantee
  描述符及 ca-context 的 subject Schema。部署另行提供逐 callable 的
  CapabilityContracts 与可信范围、EvidenceTrustBindings 和发布授权。
  不为没有独立证据来源的事实声明独立性或共同事实关系。

## 隧道字段验收

s8 的 n4_seid 和 s10/s11 的 n4Seid 使用规范十进制字符串，值域为
1 至 2^64-1。s10 的 an_tunnel_teid 和 s11 的 anTunnelTeid 使用相同编码，
值域为 1 至 2^32-1。拒绝零、前导零、符号、空白、非数字和溢出。
an_tunnel_ip/anTunnelIp 必须是可解析的 IPv4 或 IPv6 字面值，不能携带
端口、前缀长度或 zone。PDU 地址族与 RAN 传输地址族分别判定。

当前 CA-SL G.4 编译器限制跨节点结果的可用 Schema 关键字。Provider/Owner
必须在出具结果、派发消费方和证据物化前执行上述值域校验；公开 JSON
Schema 保持可链接的字符串形状。校验失败不可生成 SUCCEEDED 或证据。
测试向量见 `../fixtures/test_contract_values.py`。

测试身份与时效只能存在于 fixture。用
`python contracts/mas-owner-candidate/fixtures/build_fixture.py --version v0.2.0 <new-dir>`
生成测试输入，然后交给 ca-skills 的 `compile_mas_v1alpha5.py --mode fixture`。
正式发布仍需要真实 Owner 凭证、证据来源和部署授权。

## 编译实测

测试基线为 ca-skills `9701011761ed64c838e346ceccbe427ec14562d8`。
校验过 SHA-256 的 v2.0.1 发布二进制在 Check 阶段报 30 条
`SOURCE_NOT_AVAILABLE`，没有其他错误。该版本的 Producer 可达性实现
把 `postcondition.PASS` 当成非 SUCCEEDED 出边，导致沿正常成功链消费
前序操作输出时被拒绝。

从同一发布标签本地构建的诊断版本仅为定位该问题排除了 postcondition
出边。在同一 Skill 与 fixture 上，Check、Inspect、Build 均通过。分别
删除 Evidence 描述符、提交证明 Schema 或首个 Capability 授权后，
Check 均失败。包含回归测试的 [Compiler PR #47](https://github.com/odango-chan/ca-skill-compiler/pull/47)
修复分支也通过了同一组测试。正式发布版仍需复验。

## 定稿评审

| 验收场景 | 应有结果 |
| --- | --- |
| Owner 核实未提交且无遗留副作用 | 可按已发布失败类别处理为 FAILED；不可重试 |
| 只有错误响应、超时或缺失凭证 | OUTCOME_UNKNOWN，保留原调用和未决责任 |
| 已有 UECM/PCF/UPN/SSM 状态，缺少本次调用的证明 | 不得产生对应的本次调用 Evidence |
| 同一原调用重复查询，且核验到原有凭证和完全相同的结果 | 安全返回原结果，无第二次派发 |
| 同 SUPI/PSI 的不同原调用，或同一调用出现不同 PCF ID、相同 TEID 配不同 IP | 分别判为范围不符和同调用证据冲突 |
| 受信配置明确允许跳过鉴权，且 Owner 无冲突要求 | 可执行固定 s2 |
| 只提供 required="0"，或 Owner 报告需要鉴权 | s2 不派发，按适用性拒绝或保留未决 |

- Owner 确认每个操作的写入边界和可证明事实，重点核对 s3r 的实际 UECM
  注册、s6u 的本次策略更新和 s10 的 AN 隧道归属。共享字段或本地标记
  不足以证明另一条操作或外部效果。
- Skill 审核者确认公开输出足以支撑现有 Predicate 和下游参数，并确认
  测试契约与待发布规范中对失败和未知结果的处理一致。
- Deployment 审核者确认每个授权的 callable、Effect 和 Scope，以及
  哪些 Evidence 来源确实有权限出具。发布基线不填写尚未签发的身份或授权。

评审意见解决后冻结版本化契约基线，供实现方使用。可信发布另有验收条件。
