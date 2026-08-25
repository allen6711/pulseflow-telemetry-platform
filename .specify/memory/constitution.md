<!--
Sync Impact Report
==================
Version change: (未填寫樣板) → 1.0.0
Bump rationale: 首次批准，將樣板佔位符全部替換為具體治理內容（MINOR/PATCH 不適用於初次批准）。

Modified principles:
  [PRINCIPLE_1_NAME] → I. 可觀測性優先 (Observability First)
  [PRINCIPLE_2_NAME] → II. 測試分層 (Layered Testing) — NON-NEGOTIABLE
  [PRINCIPLE_3_NAME] → III. 可重現的量測 (Reproducible Measurement)
  [PRINCIPLE_4_NAME] → IV. 至少一次交付與應用層冪等 (At-Least-Once + Idempotency)
  [PRINCIPLE_5_NAME] → V. 簡潔優先與範圍紀律 (Simplicity & Scope Discipline)

Added sections:
  [SECTION_2_NAME]  → 技術與範圍限制 (Technology & Scope Constraints)
  [SECTION_3_NAME]  → 開發流程與品質關卡 (Development Workflow & Quality Gates)
  Governance        → 具體修訂程序、版本政策與合規審查

Removed sections: 無

Templates requiring updates:
  .specify/templates/plan-template.md — ✅ 無需修改；其 "Constitution Check" 區塊於執行時讀取本檔，
    可直接引用各原則的「關卡」條列作為 gate。
  .specify/templates/spec-template.md — ✅ 無需修改。
  .specify/templates/tasks-template.md — ✅ 無需修改。
  .specify/templates/checklist-template.md — ✅ 無需修改。

Follow-up TODOs: 無（所有佔位符皆已填入具體值）
-->

# PulseFlow Constitution

PulseFlow 是一套分散式遙測擷取與分析平台。本憲章定義所有 feature 的規格、計畫與實作
都必須遵守的不可協商規則。憲章優先於個人偏好與既有習慣。

## Core Principles

### I. 可觀測性優先 (Observability First)

任何新增或修改的處理路徑（HTTP handler、Kafka producer/consumer、儲存寫入、快取存取）
MUST 在同一份變更中同時具備可觀測性，不得延後補上：

- MUST 輸出至少一項 Prometheus 指標，涵蓋該路徑的流量、延遲與失敗三者中的相關項目。
- MUST 輸出結構化 JSON 日誌，並帶有 `trace_id`；錯誤日誌 MUST 包含錯誤分類與足以定位的上下文。
- 指標 label MUST 為有界基數。禁止以 `event_id`、原始 `tags` 值或任何使用者可控的無界字串作為 label。
- 跨越行程邊界時（HTTP → Kafka → worker），trace context MUST 透過訊息 header 傳遞。
- 失敗路徑（重試、降級、dead-letter）MUST 與成功路徑一樣具備指標與日誌。

**關卡**：plan 中任何新增的處理路徑，都能對應到具體的指標名稱與日誌事件；無法對應者視為違反。

**理由**：本專案的核心價值主張是「能量測、能除錯的分散式系統」。事後補插樁必然遺漏失敗路徑，
而失敗路徑正是需要被觀測的部分。

### II. 測試分層 (Layered Testing) — NON-NEGOTIABLE

測試策略依「是否跨越行程邊界」分層，且各層都有強制要求：

- **純邏輯 MUST 用單元測試**：schema 驗證、cache key 生成、重試分類、查詢參數驗證、
  API 錯誤映射。這些邏輯 MUST 可在不啟動任何外部依賴的情況下測試。
- **跨越 Kafka / ClickHouse / Redis 邊界的行為 MUST 用整合測試**，對真實服務執行
  （testcontainers 或 docker compose）。禁止以 mock 取代這些邊界的正確性驗證。
- **端對端行為 MUST 對完整 container stack 驗證**，涵蓋 ingest → 處理 → 查詢的完整鏈路，
  以及 dead-letter、重複事件、快取命中三條分支路徑。
- 修正缺陷時 MUST 先新增能重現該缺陷的測試。
- 涉及正確性宣稱的行為（聚合值、去重、無遺失）MUST 以已知 fixture 驗證期望值，
  不得僅斷言「沒有錯誤」。

**關卡**：每個 feature 的 tasks 都必須同時包含其所屬層級的測試任務；只有單元測試而
觸及外部依賴的 feature 視為違反。

**理由**：分散式系統的缺陷幾乎都出現在行程邊界上。以 mock 通過的測試對這類缺陷零覆蓋。

### III. 可重現的量測 (Reproducible Measurement)

所有效能與可靠性數字 MUST 可由他人重跑得出：

- 效能數字 MUST 來自已提交進版控的腳本與設定，不得來自手動執行的一次性指令。
- 發布任何量測結果時 MUST 一併記錄：硬體規格、執行指令、資料集與其產生方式、
  完整設定、執行時長、原始輸出。
- 目標值與量測結果 MUST 在文件中明確區分。README 中的建議目標在實際量測完成前
  MUST NOT 被寫成已達成的成果。
- 比較型實驗（例如快取前後、worker 數量擴展）MUST 除受測變因外保持其他條件一致，
  並記錄該變因為何。
- 可靠性宣稱（例如「重啟不遺失事件」）MUST 附上試驗次數與成功次數，而非定性描述。

**關卡**：任何在 spec、plan 或文件中出現的數字，都能追溯到一支已提交的腳本與一份原始輸出。

**理由**：無法重現的效能數字在技術審視下沒有價值，且容易在不知情的情況下變成不實陳述。

### IV. 至少一次交付與應用層冪等 (At-Least-Once + Idempotency)

事件處理的正確性語意為「至少一次交付 + 應用層冪等」，實作 MUST 遵守：

- Kafka offset MUST 在對應事件成功持久化之後才 commit。禁止先 commit 再處理。
- 相同 `event_id` 重複進入系統 MUST NOT 產生第二筆有效分析紀錄，且此性質 MUST 有測試覆蓋。
- Ingestion API MUST 在 Kafka 確認寫入之後才回應成功（`202`）。Kafka 不可用時
  MUST 回錯誤，禁止回成功。
- 處理失敗 MUST 明確分類為暫時性或永久性；暫時性失敗 MUST 以有上限的退避重試，
  永久性失敗或超過重試上限者 MUST 進入 dead-letter topic。
- Dead-letter 紀錄 MUST 保留原始 payload 與足以判斷失敗原因的中繼資料，且 MUST 可被檢視。
- Worker 收到終止訊號時 MUST 先完成或放棄當前批次到一個安全狀態再退出，
  且已被 Kafka 確認的事件在此過程中 MUST NOT 遺失。

**關卡**：plan 中每一處寫入外部系統的操作，都能說明其失敗時的重試分類、offset 行為與冪等依據。

**理由**：這是本專案最核心的技術宣稱。任何一處違反都會使「無遺失、無重複」的整體宣稱失效。

### V. 簡潔優先與範圍紀律 (Simplicity & Scope Discipline)

架構複雜度 MUST 有明確理由，且 MUST NOT 超出專案宣告的範圍：

- MUST NOT 自行實作 Kafka、ClickHouse、Redis 或任何共識演算法（Raft/Paxos）。
- MUST NOT 引入 service mesh、多區域 active-active 部署，或將系統拆成數十個微服務。
- MUST NOT 建置完整的商用監控 UI，MUST NOT 加入 AI/LLM 功能。
- 專案 MUST 維持在審視者能快速理解架構的規模；元件數量的增加 MUST 在 plan 中說明理由。
- 新增抽象層、介面或間接層 MUST 對應到一個當下已存在的具體需求，不得為了假想的未來彈性。
- Stretch goals MUST 在 MVP 驗收條件全數通過之後才開始實作。

**關卡**：plan 中每個新增的元件、套件或抽象層，都能指出它解決哪一個當下的具體問題；
無法指出者列入 Complexity Tracking 並說明為何較簡單的替代方案不可行。

**理由**：本專案的目的是清楚展示分散式處理能力。過度設計會同時稀釋展示效果與可維護性。

## 技術與範圍限制 (Technology & Scope Constraints)

- **語言與執行環境**：後端服務一律使用 Go。共用邏輯放在 `internal/`，
  可執行檔放在 `cmd/api` 與 `cmd/worker`。
- **既定技術選型**：事件串流使用 Kafka，分析儲存使用 ClickHouse，快取使用 Redis，
  可觀測性使用 OpenTelemetry 與 Prometheus，容器化使用 Docker 與 Docker Compose，
  編排使用 Kubernetes，負載測試使用 k6，CI 使用 GitHub Actions。
  更換上述任一項 MUST 修訂本憲章。
- **設定管理**：所有可變設定 MUST 由環境變數提供，MUST 有預設值，
  且 MUST 在服務啟動時驗證；設定錯誤 MUST 導致啟動失敗而非執行期才發現。
- **API 契約**：對外 HTTP 端點以 `/v1` 為前綴。已發布端點的破壞性變更 MUST 提升憲章 MAJOR 版本。
  錯誤回應 MUST 使用統一的 JSON 結構。
- **資料 schema**：遙測事件 schema MUST 帶版本欄位。Worker MUST 驗證版本並拒絕無法處理的版本。
- **健康檢查**：liveness 探針 MUST 僅反映行程存活；readiness 探針 MUST 反映依賴可用性
  並在失敗時指出是哪一個依賴。
- **本地環境**：`docker compose up` MUST 能啟動 demo 所需的全部依賴，且各依賴 MUST 有 healthcheck。

## 開發流程與品質關卡 (Development Workflow & Quality Gates)

- **Feature 工作流**：每個 feature 依序執行 `/speckit.specify` → `/speckit.clarify`（設計取捨未定時）
  → `/speckit.plan` → `/speckit.tasks` → `/speckit.analyze` → `/speckit.implement`。
  Feature 的拆分與依賴順序以 `FEATURES.md` 為準。
- **憲章關卡**：`/speckit.plan` 產生的 Constitution Check MUST 逐條對照本檔的五項原則。
  任何違反 MUST 記入該 plan 的 Complexity Tracking 表並說明較簡單方案為何不可行；
  無法說明者 MUST 修改設計而非放行。
- **依賴方向**：後續 feature MUST NOT 回頭破壞前序 feature 的對外契約。
  若必須修改既有元件（例如集中式插樁），該 feature 的 spec MUST 明確列出受影響的既有檔案與範圍。
- **CI 關卡**：每個 PR MUST 通過 build、`go vet`、lint 與單元測試。
  觸及外部依賴的 feature MUST 在 CI 中執行其整合測試（可用縮小的資料量）。
- **合併條件**：feature 的驗收條件全數可驗證通過，且測試分層要求已滿足，方可合併。
- **文件同步**：變更對外 API、指標名稱或部署方式時，MUST 在同一份變更中更新對應文件。

## Governance

- **地位**：本憲章優先於所有其他開發實務與個人偏好。與本檔衝突的既有做法 MUST 被修正。
- **修訂程序**：修訂 MUST 以變更本檔的方式提出，並在同一份變更中包含：
  (a) 修訂內容，(b) 更新後的 Sync Impact Report，(c) 版本號調整，(d) 受影響既有程式碼的處理計畫
  （立即修正或明確記錄的技術債）。
- **版本政策**（語意化版本）：
  - **MAJOR**：移除原則、重新定義既有原則使其不再向後相容，或變更既定技術選型與已發布 API 契約。
  - **MINOR**：新增原則或章節，或對既有指引作實質性擴充。
  - **PATCH**：措辭釐清、錯字修正、不改變語意的調整。
- **合規審查**：所有 PR 與程式碼審查 MUST 驗證本憲章的遵循情況。
  每完成一個里程碑（見 `FEATURES.md` 的 M1–M6）MUST 檢視一次憲章是否仍反映專案實況。
- **例外處理**：違反原則的例外 MUST 記錄在對應 feature 的 plan 中，
  包含具體理由、影響範圍與移除該例外的條件。未記錄的例外一律視為缺陷。
- **執行期指引**：專案範圍與需求以 `README.md` 為準，feature 拆分與依賴順序以 `FEATURES.md` 為準；
  兩者皆 MUST NOT 與本憲章衝突，衝突時以本憲章為準。

**Version**: 1.0.0 | **Ratified**: 2026-08-24 | **Last Amended**: 2026-08-24
