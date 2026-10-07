# Concordance 语料一致性裁决台

使用 PostgreSQL 启动完整服务：

```bash
docker compose up -d --build
```

浏览器访问 `http://localhost:18536`。后端地址为 `http://localhost:19536`，PostgreSQL 宿主机端口为 `57536`。

本项目是面向企业 NLP 数据团队的内部质量工作台。它负责语料集元数据和标注规范版本管理、接收多位标注员的独立结果、计算一致性、聚类结构化分歧，并执行需要独立复核的裁决流程。系统只保存脱敏样例和结构化标签证据，不是大众众包社区，也不展示语料原文。

## 主要功能

- 创建、修订、冻结和归档 `CorpusDataset` 版本。
- 草拟、校验、发布、废止和复制 `AnnotationSchema` 版本；规范中心只负责规范本身的发布与废止。
- 创建、编辑、提交、退回、锁定、比较和替代本人拥有的 `AnnotationSet`；提交时钉住生效规范版本（`schema_code` + `version`）与源校验和。
- 规范版本废止后，历史结果集原样保留但不能再算分，需在现行规范下重标（走 `supersede` 链）后重新计算。
- 计算 Cohen's Kappa 或名义尺度 Krippendorff's Alpha，同时展示观测一致率、机会一致率、样本量、标注员数量、缺失值数量和适用条件。
- 对 span 标注识别边界、标签、遗漏和重叠分歧，保存标签混淆矩阵和稳定聚类键。
- 认领、裁决、独立复核、接受或重开 `AdjudicationCase`，并对计算和裁决提交提供幂等保护。
- 按数据集编号（`dataset_code`）与条目键（`item_key`）对账；对不上的未结裁决单进入 `suspended` 挂起态，修复后可恢复，废止规范挂起单只能重标。
- 升级时按条目键回填旧结果集的规范版本；同一条目存在多个版本或版本已缺失的结果单列 `unresolved` 队列。
- 按操作者、request ID、实体和动作检索四实体的脱敏审计投影。

## 角色账号

| 用户名 | 密码 | 角色 | 用途 |
| --- | --- | --- | --- |
| `manager` | `Data#536` | `data_manager` | 数据集、规范、锁定和一致性计算 |
| `annotator_a` | `Annotate#536` | `annotator` | 第一位独立标注员 |
| `annotator_b` | `Compare#536` | `annotator` | 第二位独立标注员 |
| `adjudicator` | `Decide#536` | `adjudicator` | 认领并裁决分歧 |
| `reviewer` | `Review#536` | `adjudicator` | 独立复核裁决结果 |
| `auditor` | `Audit#536` | `auditor` | 只读审计检查 |
| `admin` | `Admin#536` | `admin` | 管理和流程恢复 |

以上账号仅用于本地验证。共享部署前必须更换密码和 `JWT_SECRET`。

## 技术栈

| 层级 | 技术 |
| --- | --- |
| 前端 | Angular 17、TypeScript 5.4、Angular Material、Signals、Lucide |
| Web 服务 | Nginx 1.27，SPA 回退与 `/api` 反向代理 |
| 后端 | Go 1.22、Gin、validator/v10、JWT |
| 数据访问 | GORM，构造器注入的 repository/service 分层 |
| 正式数据库 | PostgreSQL 16 |
| 轻量验证数据库 | SQLite（仅 runtime smoke） |
| 部署 | Docker Compose，三服务健康检查与健康依赖 |

## 架构与目录

```text
内置 Browser
  -> Angular 17 独立组件（Signals store、Material）
  -> Nginx /api 代理
  -> Gin router 与 middleware
  -> 实体 handler
  -> 领域 service 与状态机
  -> GORM repository
  -> PostgreSQL 16（Compose）或 SQLite（runtime smoke）
```

后端使用构造器注入，handler 不直接访问数据库；状态变化采用带前置状态的条件更新，多记录标注和一致性计算使用事务。前端四个实体分别拥有独立的 type、API client、Signals store 和页面模块。

```text
.
├── backend/
│   ├── cmd/server/                 # 进程生命周期与优雅停机
│   └── internal/
│       ├── algorithm/ matching/    # 一致性算法与 span 比较
│       ├── constants/ dto/ model/  # 共享契约
│       ├── repository/ service/    # 持久化与业务规则
│       ├── handler/ router/        # HTTP 边界
│       └── middleware/             # 认证、RBAC、审计、错误、恢复、request ID
├── frontend/src/
│   ├── types/ api/ stores/ hooks/
│   ├── components/common/          # 状态徽标、混淆矩阵、差异抽屉
│   ├── pages/ router/
│   └── styles.scss
├── database/init.sql
├── scripts/api_smoke.sh
├── docker-compose.yml
├── go.work
└── runtime_smoke.json
```

## API 清单

业务接口统一使用 `/api/v1`。成功响应包含 `data` 和 `request_id`，列表响应另含 `meta`；错误响应包含稳定的 `error.code`、消息和 request ID。

| 方法与路径 | 功能 | 写权限 |
| --- | --- | --- |
| `GET /healthz`、`GET /readyz` | 存活与数据库就绪检查 | 公开 |
| `POST /api/v1/auth/login` | 签发 JWT | 公开，限流 |
| `GET/POST /api/v1/datasets` | 查询或创建数据集 | 创建：manager/admin |
| `GET/PUT /api/v1/datasets/:id` | 详情或修订草稿元数据 | 更新：manager/admin |
| `POST /api/v1/datasets/:id/transition` | 冻结或归档 | manager/admin |
| `GET/POST /api/v1/schemas` | 查询或创建规范 | 创建：manager/admin |
| `GET/PUT /api/v1/schemas/:id` | 详情或编辑草稿 | 更新：manager/admin |
| `POST /api/v1/schemas/:id/copy` | 复制新版本 | manager/admin |
| `POST /api/v1/schemas/:id/transition` | 校验、发布或废止 | manager/admin |
| `GET/POST /api/v1/annotations` | 查询或创建结果集 | 创建：annotator/admin |
| `GET/PUT /api/v1/annotations/:id` | 详情或编辑本人草稿 | 更新：owner/admin |
| `POST /api/v1/annotations/:id/transition` | 提交、退回、锁定、比较、替代 | 按状态和角色控制 |
| `GET/POST /api/v1/adjudications` | 查询裁决或计算一致性 | 计算：manager/admin，限流 |
| `GET /api/v1/adjudications/:id` | 查询冻结的裁决证据 | 已认证用户 |
| `POST /api/v1/adjudications/:id/assign` | 认领开放或重开案件 | adjudicator/admin |
| `POST /api/v1/adjudications/:id/decide` | 提交最终结构化标签 | 已指派 adjudicator/admin |
| `POST /api/v1/adjudications/:id/review` | 独立复核 | adjudicator/admin |
| `POST /api/v1/adjudications/:id/accept` | 接受已复核裁决 | 仅记录在案的独立 reviewer |
| `POST /api/v1/adjudications/:id/reopen` | 重开已复核裁决 | 仅记录在案的独立 reviewer |
| `POST /api/v1/reconciliation/run` | 按数据集编号和条目键对账并挂起/恢复案件 | manager/admin |
| `GET /api/v1/reconciliation/runs` | 查询对账运行记录 | manager/admin |
| `POST /api/v1/adjudications/:id/resume` | 修复后释放挂起案件（废止规范除外） | manager/admin |
| `POST /api/v1/annotations/backfill` | 手动触发旧数据规范版本回填 | manager/admin |
| `GET /api/v1/annotations/unresolved` | 回填补不出的未决结果队列 | manager/admin |
| `GET /api/v1/audit` | 筛选脱敏变更投影 | auditor/adjudicator/admin |

一致性计算和裁决提交必须带 `Idempotency-Key` 请求头。非法状态迁移返回 HTTP 409，规范或标注不兼容返回 HTTP 422，未认证和权限不足分别返回 401、403。

## 共享枚举位置

`AnnotationState = draft | submitted | returned | locked | compared | superseded`

| 层级 | 位置 |
| --- | --- |
| 后端枚举与状态机 | `backend/internal/constants/annotation_state.go` |
| model 持久化 | `backend/internal/model/annotation_set.go` |
| dto 请求与响应 | `backend/internal/dto/annotation_set.go` |
| service 迁移与权限 | `backend/internal/service/annotation_set.go` |
| repository 条件更新 | `backend/internal/repository/annotation_set.go` |
| 前端枚举 | `frontend/src/types/enums/annotation-state.ts` |
| store 与 API | `frontend/src/stores/annotation-set.store.ts`、`frontend/src/api/annotation-set.ts` |
| 共享组件 | `frontend/src/components/common/annotation-state-badge.component.ts` |
| 页面 | `frontend/src/pages/annotations.page.ts`、`frontend/src/pages/adjudication.page.ts` |
| 测试 | `backend/internal/constants/annotation_state_test.go` |

`DisagreementType = boundary | label | omission | overlap`

`CaseState` 另含对账挂起态 `suspended`（不计入正常状态机迁移，由对账服务挂起/恢复并记录 `state_before_suspend`）；结果集钉版状态 `SchemaPinStatus = legacy | pinned | unresolved`。

| 层级 | 位置 |
| --- | --- |
| 后端枚举 | `backend/internal/constants/disagreement_type.go` |
| model 与 dto | `backend/internal/model/adjudication_case.go`、`backend/internal/dto/adjudication_case.go` |
| span 算法与聚类 | `backend/internal/matching/span.go` |
| 裁决 service | `backend/internal/service/adjudication_case.go` |
| 前端枚举与类型 | `frontend/src/types/enums/disagreement-type.ts`、`frontend/src/types/adjudication-case.ts` |
| store 与 API | `frontend/src/stores/adjudication-case.store.ts`、`frontend/src/api/adjudication-case.ts` |
| 共享差异组件 | `frontend/src/components/common/diff-evidence-drawer.component.ts` |
| 页面 | `frontend/src/pages/annotations.page.ts`、`frontend/src/pages/adjudication.page.ts`、`frontend/src/pages/audit.page.ts` |
| 测试 | `backend/internal/matching/span_test.go` |

数据集页和裁决页共用 `AgreementMatrix`；标注页和裁决页共用 `AnnotationStateBadge`；标注、裁决和审计页共用 `DiffEvidenceDrawer`。`useAuth` 与 `useAdjudicationQueue` 位于 `frontend/src/hooks/`。

## 规范版本钉版与两侧对账

规范中心（`AnnotationSchema` 维护端）与标注运营台（`AnnotationSet` / `AdjudicationCase` 侧）职责分离：

- **提交钉版**：结果集 `draft -> submitted` 时在运营台侧快照 `pinned_schema_code`、`pinned_schema_version`、`pinned_source_checksum` 与 `pinned_at`；提交可携带 `source_checksum`，与建档校验和不一致返回 409 `source_checksum_mismatch`。
- **发布/废止隔离**：规范发布与废止只写规范表及对应审计；发布失败只需重试规范侧，运营台已提交结果不受影响。
- **废止处理**：规范废止后，禁止基于该版创建结果（422 `incompatible_schema`）与重新算分（422 `schema_deprecated`）。历史结果不删除，标注员在现行规范下新建结果并经 `supersedes` 链替代旧版，再锁分计算。
- **对账挂起**：`POST /api/v1/reconciliation/run` 按数据集编号和条目键核对裁决单与结果集；未钉版、键不一致或结果集缺失的未结案件进入 `suspended`（记录 `suspend_reason_code` 与 `state_before_suspend`），修复后同次或下次对账自动恢复，也可由经理 `POST /api/v1/adjudications/:id/resume` 手动恢复。废止规范挂起单拒绝恢复，必须重标。
- **升级回填**：服务启动时对 `schema_pin_status=legacy` 且已提交的旧数据，按 `dataset_id + item_key` 分组回填；同条目引用唯一规范版本即钉版，多版本或版本缺失标为 `unresolved` 单列。也可手动触发 `POST /api/v1/annotations/backfill`，未决队列见 `GET /api/v1/annotations/unresolved`。
- **裁决状态**：`suspended` 不参与认领/裁决/复核状态机；已接受案件是历史记录，不重新挂起。

## 算法适用条件

- 当且仅当两位标注员拥有完整的共同单元集合时，自动选择 Cohen's Kappa。显式请求 Kappa 时，缺失单元会计数并从计算中排除。
- 标注员超过两位或存在缺失评分时，自动选择名义尺度 Krippendorff's Alpha。少于两份评分的单元不参与观测分歧计算。
- span 比较使用左闭右开的字符区间，区分精确匹配、部分重叠、边界差异、标签差异和单边遗漏。
- 混淆矩阵统计成对标签，待裁决聚类键格式为 `schema_code:disagreement_type:label_pair`。
- 每次计算保存输入哈希和 `ALGORITHM_VERSION`；相同输入可复用既有结果。

本系统只提供数据质量辅助证据。一致性分数不是自动授权，也不是模型质量保证。

## 脱敏与审计边界

- 原始文档只以 item key 和 SHA-256 checksum 表示，不保存语料原文。
- 规范样例必须显式包含 `[MASK]` 或 `***`，未脱敏样例会被拒绝。
- 审计记录仅包含操作者、角色、request ID、动作、实体和结构化摘要。
- 审计写入前递归脱敏 `labels`、examples、masked/raw/original content，以及 password、token、authorization、secret 和 credential 类字段。
- 业务变更与对应 `AuditEvent` 在同一数据库事务中提交；审计持久化失败时业务变更会回滚。
- HTTP 日志只记录路由模板与响应元数据，不记录请求体。

## 环境变量与端口

需要本地调整时，以 `.env.example` 为模板创建 `.env`；`.env` 已被 Git 忽略。

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `FRONTEND_PORT` | `18536` | Angular/Nginx 宿主机端口 |
| `BACKEND_PORT` | `19536` | Gin 宿主机端口 |
| `DB_PORT` | `57536` | PostgreSQL 宿主机端口 |
| `DB_DRIVER` | `postgres` | `postgres` 或 `sqlite` |
| `DB_DSN` | Compose PostgreSQL DSN | GORM 连接串 |
| `DB_AUTO_MIGRATE` | `true` | 启动时执行模型迁移 |
| `JWT_SECRET` | 本地默认值 | HMAC 签名密钥，至少 24 字节 |
| `JWT_TTL_MINUTES` | `480` | token 有效期 |
| `CORS_ORIGIN` | `http://localhost:18536` | 允许的浏览器来源 |
| `RATE_LIMIT_PER_MINUTE` | `300` | 各限流域的内存计数上限 |
| `ALGORITHM_VERSION` | `agreement-nominal-span-v1.0` | 持久化的算法版本 |

## 本地开发

依赖 Go 1.22、Node.js 20、npm；正式模式使用 PostgreSQL 16，runtime smoke 使用项目已引入的 SQLite GORM driver。

使用 SQLite 配置启动后端：

```bash
cd backend
PORT=20536 \
DB_DRIVER=sqlite \
DB_DSN='file:local-dev?mode=memory&cache=shared' \
JWT_SECRET='local-development-secret-change-me' \
go run ./cmd/server
```

Angular 开发服务需要将 `/api` 反向代理到后端，完整联调建议使用 Compose。前端独立检查命令：

```bash
npm --prefix frontend ci
npm --prefix frontend run typecheck
npm --prefix frontend run build
```

## 构建与测试

在项目根目录执行：

```bash
go work sync
go build ./backend/...
go vet ./backend/...
go test ./backend/...
go test -race ./backend/...
npm --prefix frontend ci
npm --prefix frontend run typecheck
npm --prefix frontend run build
docker compose config --quiet
```

Compose 服务健康后运行真实 API 工作流（依赖 `curl` 与 `jq`）：

```bash
./scripts/api_smoke.sh
```

## Runtime Smoke

`runtime_smoke.json` 是可解析的服务启动 manifest。它从 `backend/` 启动服务，使用 `20536` 端口和内存 SQLite，并等待 `http://127.0.0.1:20536/healthz`。

```bash

```

## Docker 部署

```bash
docker compose up -d --build
docker compose ps
docker compose logs --tail=100 backend frontend db
```

## 常见问题

- **登录返回 401：** 使用上表中的完整账号密码，并确认后端健康。
- **写操作返回 403：** 当前 JWT 角色或资源所有权不允许该动作。数据集/规范属于 `data_manager`，结果创建属于 `annotator`，裁决属于 `adjudicator`。
- **状态迁移返回 409：** 刷新记录并按状态机顺序操作；数据集更新还必须携带当前版本。
- **标注返回 422：** 检查数据集已冻结、规范已发布且属于该数据集、标签存在于规范中，并确保 span 的 `end > start`。规范已废止时需改用现行规范重标；未钉版或钉版与规范中心不一致的结果集不能算分。
- **提交返回 409 `source_checksum_mismatch`：** 提交携带的 `source_checksum` 必须与建档时一致；提交成功即钉住该校验和与规范版本。
- **裁决单显示 `suspended`：** 对账发现数据集编号/条目键对不上、结果集缺失或规范版本未钉版；修复后重新运行对账即可恢复。`schema_deprecated` 挂起单不能手动恢复，必须按现行规范重标并重算。
- **裁决复核返回 403：** 做出裁决的人不能复核同一案件，请改用 `reviewer`。
- **Compose 端口冲突：** 只修改 `.env` 中宿主机侧的 `FRONTEND_PORT`、`BACKEND_PORT` 或 `DB_PORT`。

## 停止服务

停止三服务并移除命名数据卷：

```bash
docker compose down -v --remove-orphans
```

## License

本项目仅用于内部评审与训练，未授予外部分发许可。
