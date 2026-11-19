# MAS v1alpha5 静态契约收口

适用范围为 ca-skills PR #74 的 13 个操作和固定 IPv4 / SSC1 / LOCAL_POOL /
无二次鉴权配置。源行为固定在 env-openamsm 594dec78。以下语义是拟冻结的
Owner 契约输入，候选目录不具备发布权限。

## 公开结果与写入边界

所有操作按 MUTATION 设计。这里的 Effect 包含 Owner 的本地会话状态与外部
写入。成功输出的字段保持候选 Schema 类型，表中的含义约束可声称的事实。

| 步骤 | 写入边界 | 成功证据允许表达的事实 |
| --- | --- | --- |
| s1 | 可建立 SMF UE/Session shell 并更新订阅状态 | sub_summary=ok 只表达订阅校验完成 |
| s2 | 写入 SAA 阶段、未决标记和 EAP 轮次 | phase=SKIPPED 只表达固定配置跳过 DN-AAA |
| s3 | 写入 SM Context、本地生命周期及资源关联 | session_keys 和 OPERATIONAL 表达本地 SC 上下文创建。sm_context_ref 只进私有上下文。没有外部响应流也可达到 OPERATIONAL |
| s3r | 写入 UECM 注册及本地注册状态 | uecm_registered=1，须绑定该 SUPI/PSI 的注册事实 |
| s3p | 建立 PCF 策略关联并写入 PA 状态 | 非空且非 bound 的 policy_assoc_id，证明该次创建的关联 |
| s4 | 选择并写入 UP 锚点状态 | up_anchor=1，锚点已选定 |
| s5 | 分配地址并写入 IAM 状态 | ue_addr=1，地址已分配；字段为标记 |
| s6u | 更新已有 PCF 关联及 PA 状态 | 同一真实关联上的本次更新，不能只凭既有 policy_assoc_id |
| s7 | 写入本地 CA skip 状态 | charging_ref=1，跳过占位收费步骤；没有收费预留 |
| s8 | 建立 N4/PFCP 会话并写入 UPN 状态 | est_done=1 加有效非零 SEID，证明 N4 会话建立 |
| s10 | 发送 N1/N2 并记录匹配的 AN 隧道 | 原始操作对应的非零 TEID 和有效地址；挂起本身不满足 |
| s11 | 更新 N4 会话的 AN 隧道并写入 N3 同步状态 | n3_synced=1，限于该次同步 |
| s13 | 写入本地事件状态 | last_event=1，本地事件已记录；没有 3GPP Step 18 通知含义 |

初始 CreateSMContext 已由上游准入。Host 只做原始请求认证、去重、SUPI/PSI
关联和竞争写入裁决。入口没有额外的 AMF 注册、CM CONNECTED 或 SMF Session
ABSENT 门槛。s1 的 shell 不构成 PDU Session 建立证据。

## 提交与证据契约

- 最小 ToolCatalog Owner Outcomes 为 SUCCEEDED 和 FAILED，与当前 Skill 的
  FAILED 路由一致。FAILED 不授予无副作用或重试权限。每个 MUTATION 的
  receipt.commit 都引用根封闭的 proof Schema，必填 originalInvocationId 和
  commitState，后者枚举 COMMITTED 与 NOT_COMMITTED。ToolCatalog 的
  mutation.commitProof.bind 固定为 originalInvocationId。
- 当前契约没有经证明的可重试失败类。HTTP 200/OK 是线缆观测。派发后缺少
  精确证明保持 OUTCOME_UNKNOWN。DEFERRED 要求可恢复 Owner 句柄，原始 HTTP
  waiter 不满足。DENIED 属于运行侧拒绝。最小 Skill 编译配置不声明 Reconcile
  callable，当前 Procedure 也没有 Reconcile 节点。将来声明时使用同一 Owner
  的独立 READ 与只接收 originalInvocationId 的输入，按原始操作逐项指定证明
  和可恢复输出。补偿与重发不属于 Reconcile。
- 每条 Evidence Guarantee 绑定本次 SUCCEEDED 的准确 callable 和公开
  output；选择 exact，排序字段为 observedAt，关联 invocationId 和
  producerInvocationId。Owner 必须
  在私有证据记录中验证并记录准确 SUPI、PSI、部署和业务实例代次，并给出
  实际 observedAt、最大可接受时效及来源。只有同一次精确派发的证据参与
  Predicate 判定。s2 的 PSI 来自已验证的私有会话范围。
- 发布时以不可变快照提供 mas ToolCatalog 和数据 Schema、Evidence Guarantee
  描述符及 ca-context 的 subject Schema。部署另行提供逐 callable 的
  CapabilityContracts 与可信范围、EvidenceTrustBindings 和发布授权。
  不为没有独立证据来源的事实声明独立性或共同事实关系。

测试身份与时效只能存在于 fixture。用
`python contracts/mas-owner-candidate/fixtures/build_fixture.py <new-dir>`
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
Check 均失败。诊断版本的结果只验证候选契约形状；发布版仍有阻塞。
