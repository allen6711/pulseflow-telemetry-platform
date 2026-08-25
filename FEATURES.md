# PulseFlow Feature 拆分計畫（Spec Kit 開發指南）

本文件把 `README.md` 的 MVP 範圍拆成 **11 個可獨立 spec / plan / implement 的 feature**，
並附上可直接複製貼上的 `/speckit.specify` 提示詞、依賴關係與驗收標準。

- 拆分基準：README「Core MVP」§1–§6、「MVP acceptance criteria」、「Testing」、「Benchmark plan」、「Stretch goals」
- 目標工作流：`/speckit.constitution` → 每個 feature 各跑一輪 `/speckit.specify` → `/speckit.clarify` → `/speckit.plan` → `/speckit.tasks` → `/speckit.analyze` → `/speckit.implement`

---

## 0. 開始之前：先寫 Constitution

`.specify/memory/constitution.md` 目前仍是未填寫的樣板。它是所有 `plan.md` 的檢查依據，
建議在開第一個 feature 之前先執行 `/speckit.constitution`，並至少涵蓋以下原則：

1. **可觀測性優先** — 每個新增的處理路徑都必須同時輸出 metric 與結構化日誌（含 trace ID）。
2. **測試分層（非協商）** — 純邏輯用單元測試；跨越 Kafka / ClickHouse / Redis 邊界一律用 testcontainers 或 compose 的整合測試。
3. **可重現的量測** — 任何效能數字都必須來自 committed 的腳本與設定，並記錄硬體、指令、資料集、時長與原始輸出。README 的目標值在量測完成前不得寫成成果。
4. **至少一次交付 + 應用層冪等** — 先持久化再 commit offset；重複的 `event_id` 不得產生第二筆分析紀錄。
5. **簡潔優先** — 不自行實作 Kafka / ClickHouse / Redis、不做共識演算法、不做微服務拆分（見 README「Explicit non-goals」）。

---

## 1. 拆分原則

| 原則 | 說明 |
| --- | --- |
| 垂直切片 | 除了 F01 基礎建設外，每個 feature 完成後都能單獨被觀察／驗證（可以打 API、可以看到資料進 ClickHouse、可以看到 metric 變化）。 |
| 單一 spec 可寫完 | 每個 feature 的規格大致落在 1 份 spec + 3–8 個 user story，避免一個 feature 塞進整條 pipeline。 |
| 依賴單向 | 後面的 feature 只依賴前面的，不回頭改前面的契約；若必須改（例如新增 metric），在該 feature 的 spec 明確寫出「修改既有元件」的範圍。 |
| 跨切面能力集中定義 | 設定載入、日誌、metric registry 在 F01 建好共用套件，之後各 feature 只註冊自己的指標，不重造輪子。 |
| Benchmark 與 MVP 驗收獨立成 feature | README 把量測列為交付物，因此它需要自己的 spec 與 tasks，而不是散在各處的 TODO。 |

---

## 2. 依賴關係

```text
F01 平台基礎與本地環境
 ├── F02 事件擷取 API ─────────────┐
 ├── F03 ClickHouse 分析儲存層 ────┼── F04 分區消費 Worker
 │                                 │        │
 │                                 │        └── F05 重試 / DLQ / 冪等保證
 │                                 │
 │                                 └── F06 分析查詢 API
 │                                          │
 │                                          └── F07 Redis 查詢快取
 │
 └── F08 可觀測性（OTel + Prometheus 完整指標目錄）
              │
              ├── F09 負載產生器與 Benchmark 套件
              │        │
              │        └── F10 E2E 測試與 MVP 驗收
              │
              └── F11 Kubernetes 部署（MVP 之後）

F12+ Stretch goals（僅在 MVP 驗收通過後）
```

**關鍵路徑（最短可展示 demo）**：F01 → F02 → F03 → F04 → F06。
走完這五個就有「送事件 → 落地 → 查聚合」的完整鏈路，可以先做一次 demo，再補可靠性與效能。

---

## 3. Feature 清單

### F01 · 平台基礎與本地開發環境

- **建議分支/short-name**：`platform-foundation`
- **對應 README**：Repository layout、MVP acceptance 第 1 條、API summary 的 `/v1/health/live`、`/v1/health/ready`
- **依賴**：無

**範圍（In scope）**

- Go module 與 `cmd/api`、`cmd/worker` 骨架（可啟動、可優雅關閉）
- `internal/config`：環境變數 + 預設值的設定載入與驗證（Kafka brokers、ClickHouse DSN、Redis addr、port、log level）
- 結構化 JSON 日誌套件（`log/slog`），欄位含 service、version、level、trace_id 佔位
- `docker-compose.yml`：Kafka、ClickHouse、Redis、Prometheus，含 healthcheck 與固定版本
- `/v1/health/live`（純存活）與 `/v1/health/ready`（檢查 Kafka / ClickHouse / Redis 連線）
- `Makefile` 或 `scripts/`：build、test、lint、compose up/down
- GitHub Actions CI：build + vet + unit test

**不在範圍**：任何業務邏輯、metric 內容（只建 registry 骨架）、K8s。

**驗收**

- `docker compose up` 後所有依賴 healthy，API 與 worker 容器啟動不 crash
- 停掉 ClickHouse 後 `/v1/health/ready` 回 503 且列出失敗的依賴，`/v1/health/live` 仍回 200
- CI 在 PR 上綠燈

```text
/speckit.specify --short-name platform-foundation
建立 PulseFlow 的專案骨架與本地開發環境。內容包含：Go module 與 cmd/api、cmd/worker 兩個可啟動且支援 graceful shutdown 的執行檔；internal/config 提供以環境變數為主、具預設值與啟動時驗證的設定載入；使用 log/slog 的結構化 JSON 日誌套件，保留 trace_id 欄位；docker-compose.yml 啟動 Kafka、ClickHouse、Redis、Prometheus 並附 healthcheck 與固定映像版本；API 提供 /v1/health/live 純存活探針與 /v1/health/ready 依賴感知探針（檢查 Kafka、ClickHouse、Redis，失敗時回 503 並列出失敗依賴）；Makefile 提供 build、test、lint、compose 生命週期指令；GitHub Actions 執行 build、go vet 與單元測試。此 feature 不包含任何業務邏輯與 Kubernetes 部署。
```

---

### F02 · 事件擷取 API（Ingestion）

- **建議分支/short-name**：`event-ingestion-api`
- **對應 README**：Core MVP §1、API summary `POST /v1/events`、`POST /v1/events/batch`
- **依賴**：F01

**範圍**

- 正式的事件 schema（`event_id`、`service`、`timestamp`、`metric`、`value`、`tags`、schema version）與驗證規則
- `POST /v1/events`：驗證 → 發布到 Kafka `telemetry.events` → Kafka ack 後回 `202 Accepted`
- `POST /v1/events/batch`：有上限的批次（例如 1000 筆），定義部分失敗的回應語意
- Kafka producer：以 `service`（或 `service+metric`）決定 partition key，確保同一服務事件有序
- 錯誤映射：400（schema 不合法）、413（批次超限）、503（Kafka 不可用）、統一錯誤 JSON 格式
- 單元測試：schema 驗證、錯誤映射；整合測試：producer 對真實 Kafka

**不在範圍**：消費端、ClickHouse、快取、rate limit（stretch）。

**驗收**

- 合法事件回 202，且能用 console consumer 在 `telemetry.events` 讀到該筆
- 缺欄位／型別錯誤／時間格式錯誤各自回 400 並指出欄位
- Kafka 停機時回 503，且不會回 202（避免假成功）
- 批次超過上限回 413

```text
/speckit.specify --short-name event-ingestion-api
實作 PulseFlow 的遙測事件擷取 API。定義正式的事件 schema（event_id、service、timestamp、metric、value、tags、schema 版本）與驗證規則。POST /v1/events 接收單筆事件，驗證後發布到 Kafka topic telemetry.events，並在 Kafka 確認寫入後才回應 202 Accepted。POST /v1/events/batch 接收有上限的批次（預設上限 1000 筆），需明確定義部分成功／部分失敗的回應語意。Kafka producer 以 service 作為 partition key，使同一服務的事件維持順序。錯誤映射需涵蓋 schema 不合法回 400 並指出欄位、批次超限回 413、Kafka 不可用回 503 且絕不回 202。需有 schema 驗證與錯誤映射的單元測試，以及對真實 Kafka 的 producer 整合測試。不包含消費端、ClickHouse 寫入與速率限制。
```

---

### F03 · ClickHouse 分析儲存層

- **建議分支/short-name**：`clickhouse-store`
- **對應 README**：Core MVP §2（persist）、§3（aggregates）、Testing「ClickHouse repository integration test」
- **依賴**：F01

**範圍**

- `migrations/`：表結構設計與可重複執行的 migration 機制
  - 明細表（依 `service`、`metric`、時間分區，排序鍵支援時間範圍掃描）
  - 冪等策略的資料層基礎（例如 `ReplacingMergeTree` 以 `event_id` 去重，或去重輔助表）
  - 視需要的物化視圖／預聚合表以支撐 p95 查詢
- `internal/storage`：repository 介面 + ClickHouse 實作（批次寫入、聚合查詢、服務清單查詢）
- 批次寫入策略：批次大小 / flush 間隔 / 逾時
- 整合測試：對真實 ClickHouse 驗證寫入與聚合結果正確性（使用已知 fixture）

**不在範圍**：Kafka 消費流程、HTTP 層。

**驗收**

- migration 可在乾淨的 ClickHouse 上執行成功，重跑不出錯
- 以固定 fixture 寫入後，count / avg / min / max / p50 / p95 / p99 查詢結果符合預期值
- 同一 `event_id` 寫入兩次，去重策略下查詢結果仍只算一次

```text
/speckit.specify --short-name clickhouse-store
建立 PulseFlow 的 ClickHouse 分析儲存層。內容包含 migrations 目錄與可重複執行的 migration 機制；設計遙測明細表，依時間分區、以 service 與 metric 及時間戳為排序鍵以支援時間範圍掃描；採用支援以 event_id 去重的表引擎策略作為冪等的資料層基礎；視查詢需求建立預聚合表或物化視圖以支撐百分位查詢。實作 internal/storage 的 repository 介面與 ClickHouse 實作，提供批次寫入、時間範圍聚合查詢（count、avg、min、max、p50、p95、p99）與已觀察服務清單查詢，並定義批次大小、flush 間隔與逾時策略。需有對真實 ClickHouse 的整合測試，使用已知 fixture 驗證聚合值正確，並驗證重複 event_id 不會被重複計算。不包含 Kafka 消費流程與 HTTP 層。
```

---

### F04 · 分區消費 Worker（Consumer Group + 持久化）

- **建議分支/short-name**：`partitioned-worker`
- **對應 README**：Core MVP §2 前半、MVP acceptance「至少三個 worker replica 共享處理」
- **依賴**：F02、F03

**範圍**

- Kafka consumer group 消費 `telemetry.events`，多 replica 平行處理不同 partition
- 事件 schema 與版本驗證；不合法事件直接拒絕（此階段先記錄與計數）
- 依 `event_id` 去重（應用層 + F03 的資料層策略）
- 批次寫入 ClickHouse
- **持久化成功後才 commit offset**（at-least-once）
- 消費者 rebalance 與優雅關閉（收到 SIGTERM 時先 flush 再退出）
- 整合測試：Kafka producer/consumer、重複 `event_id` 只產生一筆分析紀錄

**不在範圍**：重試退避與 DLQ（F05）、完整指標目錄（F08）。

**驗收**

- 3 個 worker replica 起來後，partition 被均分且處理總量等於送入量（扣除重複）
- 送入含重複 `event_id` 的資料集，ClickHouse 中該 ID 只有一筆有效分析紀錄
- worker 在 flush 之前被砍掉，重啟後該批事件會被重新消費而非遺失

```text
/speckit.specify --short-name partitioned-worker
實作 PulseFlow 的遙測處理 worker。worker 以 Kafka consumer group 消費 telemetry.events，支援多個 replica 平行處理不同 partition 並可水平擴充。處理流程為：驗證事件 schema 與版本、拒絕格式錯誤事件、以 event_id 去重、批次寫入 ClickHouse，並且只有在持久化成功之後才 commit Kafka offset，以達成至少一次交付語意。需處理 consumer group rebalance，並在收到 SIGTERM 時先 flush 未寫入的批次再退出。需有 Kafka producer/consumer 整合測試，以及驗證重複 event_id 只產生一筆分析紀錄的測試，並驗證三個 replica 可共享處理同一 topic。此 feature 不包含重試退避與 dead-letter 處理，也不包含完整的指標目錄。
```

---

### F05 · 可靠性：重試分類、退避與 Dead-Letter

- **建議分支/short-name**：`retry-and-dlq`
- **對應 README**：Core MVP §2 後半、§5
- **依賴**：F04

**範圍**

- 失敗分類：暫時性（ClickHouse 逾時、網路）vs 永久性（schema 不合法、無法解析）
- 暫時性失敗以有上限的指數退避重試（可設定次數、基準、上限、jitter）
- 超過重試上限或永久性失敗 → 送往 dead-letter topic
- DLQ 紀錄格式：原始 payload + 失敗原因 + 錯誤分類 + 重試次數 + 首次/最後失敗時間 + trace ID + 來源 partition/offset
- DLQ 檢視方式（腳本或簡易 CLI）
- 測試：重試分類單元測試、注入失敗使訊息進入 DLQ 的整合測試
- Worker 重啟／rebalance 演練腳本（供 F09 的失敗實驗重複使用）

**不在範圍**：自動重放 DLQ（可列為 stretch）。

**驗收**

- 注入暫時性錯誤時，訊息在退避後成功處理且未進 DLQ
- 注入永久性錯誤（毒訊息）時，於重試上限內進入 DLQ，且 DLQ 紀錄可讀出原始 payload 與失敗中繼資料
- Worker 在處理中被終止 20 次的演練中，Kafka 已 ack 的事件無遺失

```text
/speckit.specify --short-name retry-and-dlq
為 PulseFlow worker 加上可靠性處理。需將處理失敗分類為暫時性失敗（ClickHouse 逾時、網路錯誤）與永久性失敗（schema 不合法、無法解析）。暫時性失敗以有上限的指數退避重試，重試次數、基準間隔、上限與 jitter 皆可設定。超過重試上限或屬永久性失敗的訊息送往 dead-letter topic，DLQ 紀錄需包含原始 payload、失敗原因、錯誤分類、重試次數、首次與最後失敗時間、trace ID 以及來源 partition 與 offset，並提供可檢視 DLQ 內容的腳本。需有重試分類的單元測試、以注入失敗驗證訊息進入 DLQ 的整合測試，以及可重複執行的 worker 終止與重啟演練腳本，用於驗證 Kafka 已確認的事件在重啟過程中不會遺失。不包含 DLQ 自動重放。
```

---

### F06 · 分析查詢 API

- **建議分支/short-name**：`analytics-query-api`
- **對應 README**：Core MVP §3、API summary `GET /v1/metrics/{service}/{metric}`、`GET /v1/services`
- **依賴**：F03（可與 F04 並行）

**範圍**

- `GET /v1/metrics/{service}/{metric}?from=&to=&percentiles=p50,p95,p99`
- 回應含 count、avg、min、max 與請求的百分位
- 查詢參數驗證：時間格式、`from < to`、最大時間範圍、允許的百分位清單、未知參數處理
- `GET /v1/services`：列出已觀察到的服務（可含 metric 清單）
- 錯誤映射：400（參數不合法）、404 或空結果語意（需明確定義）、504（查詢逾時）
- 單元測試：查詢驗證、API 錯誤映射；整合測試：對已知 fixture 的正確聚合值

**不在範圍**：快取（F07）。

**驗收**

- 對已知 fixture 的查詢回傳正確聚合值（與 F03 整合測試的期望值一致）
- 不合法的時間範圍與百分位回 400 並說明原因
- 超過最大時間範圍的查詢被拒絕而不是拖垮 ClickHouse

```text
/speckit.specify --short-name analytics-query-api
實作 PulseFlow 的分析查詢 API。GET /v1/metrics/{service}/{metric} 支援 from、to 時間範圍與 percentiles 參數（如 p50,p95,p99），回應包含 count、平均、最小、最大值與請求的百分位。需驗證查詢參數：時間格式、from 必須早於 to、限制最大查詢時間範圍、限制允許的百分位清單，並定義未知參數的處理方式。GET /v1/services 列出已觀察到的服務與其 metric 清單。錯誤映射需涵蓋參數不合法回 400 並說明原因、無資料時的回應語意需明確定義、查詢逾時回 504。需有查詢參數驗證與 API 錯誤映射的單元測試，以及以已知 fixture 驗證聚合值正確的整合測試。此 feature 不包含 Redis 快取。
```

---

### F07 · Redis 查詢快取

- **建議分支/short-name**：`redis-query-cache`
- **對應 README**：Core MVP §4、Testing「cache-key generation」「Redis integration test」
- **依賴**：F06

**範圍**

- 決定性 cache key：由 service、metric、時間範圍（正規化／對齊）、聚合選項推導
- 時間範圍正規化策略（例如對齊到 bucket 邊界以提高命中率），以及「查詢包含現在時間」時的處理
- TTL + 隨機 jitter，避免同時大量過期
- cache hit / miss 指標與 hit ratio
- 快取失效／繞過機制（例如 `?no_cache=1` 或 header，供 benchmark 用）
- 單元測試：cache key 生成的決定性與碰撞安全；整合測試：對真實 Redis
- Redis 不可用時降級為直接查 ClickHouse（不可讓查詢整體失敗）

**不在範圍**：分散式鎖／singleflight（可列為 stretch，但若能簡潔實作可納入）。

**驗收**

- 相同查詢的第二次請求命中快取，且回應內容與未命中時一致
- 參數順序不同但語意相同的查詢產生相同 key；語意不同的查詢絕不共用 key
- TTL jitter 可觀察到過期時間分散
- Redis 停機時查詢仍成功，並記錄降級事件

```text
/speckit.specify --short-name redis-query-cache
為 PulseFlow 分析查詢加上 Redis 快取層。需以 service、metric、正規化後的時間範圍與聚合選項推導出決定性的 cache key，語意相同但參數順序不同的查詢須產生相同 key，語意不同的查詢絕不可共用 key，並定義時間範圍的對齊策略與查詢包含當前時間時的處理方式。快取需設定 TTL 並加上隨機 jitter，避免大量 key 同時過期。需暴露 cache hit、cache miss 指標與命中率，並提供繞過快取的機制供 benchmark 使用。Redis 不可用時需降級為直接查詢 ClickHouse 並記錄降級事件，不可讓查詢整體失敗。需有 cache key 生成的決定性單元測試與對真實 Redis 的整合測試。
```

---

### F08 · 可觀測性：OpenTelemetry 追蹤與 Prometheus 指標目錄

- **建議分支/short-name**：`observability`
- **對應 README**：Core MVP §6、API summary `/metrics`
- **依賴**：F02、F04、F06（前述元件存在後才能完整插樁）

**範圍**

- 完整指標目錄（README §6 全部十項）：ingestion RPS、ingestion p50/p95/p99、Kafka publish 失敗數、consumer lag、processed events/sec、處理失敗數、DLQ 計數、ClickHouse 寫入延遲、analytics query p95、Redis 命中率
- `/metrics` Prometheus endpoint（API 與 worker 各自暴露）
- 指標命名規範與 label 基數控制（避免以 `event_id` 或高基數 tag 當 label）
- OpenTelemetry trace：HTTP ingestion → Kafka publish 的 context 傳遞（透過 Kafka header），worker 端接續 span
- 結構化日誌帶入 trace ID
- Prometheus scrape 設定納入 compose
- 測試：指標存在性與名稱穩定性的測試

**跨切面規則**：F02–F07 只註冊自己必要的指標；F08 負責統一命名、補齊缺漏、加上追蹤，並在 spec 中明確列出會修改哪些既有檔案。

**驗收**

- `curl :PORT/metrics` 可看到十項指標且有實際數值變動
- 一次 ingestion 請求可在 trace 中看到 HTTP span 與 Kafka publish span 相連，worker 端 span 具相同 trace ID
- 日誌中的 trace ID 可對應到 trace

```text
/speckit.specify --short-name observability
為 PulseFlow 建立完整的可觀測性。需暴露以下 Prometheus 指標：ingestion 請求速率、ingestion 的 p50/p95/p99 延遲、Kafka publish 失敗數、consumer lag、每秒處理事件數、處理失敗數、dead-letter 計數、ClickHouse 寫入延遲、分析查詢 p95 延遲、Redis 快取命中率。API 與 worker 各自提供 /metrics endpoint。需定義指標命名規範與 label 基數控制原則，禁止使用 event_id 或高基數 tag 作為 label。使用 OpenTelemetry 建立追蹤，將 HTTP ingestion 的 trace context 透過 Kafka header 傳遞到 worker，使 worker 的 span 屬於同一條 trace，並在結構化日誌中輸出 trace ID。Prometheus 的 scrape 設定需納入 docker compose。需有驗證指標存在且名稱穩定的測試。此 feature 會修改既有的 API、worker、storage 與 cache 元件以加入插樁。
```

---

### F09 · 負載產生器與 Benchmark 套件

- **建議分支/short-name**：`benchmark-suite`
- **對應 README**：Benchmark plan（四項實驗）、MVP acceptance「一百萬事件」「committed 的 benchmark 腳本與結果」
- **依賴**：F07、F08

**範圍**

- 合成事件產生器：可設定服務數、metric 數、事件總數、速率、重複 `event_id` 比例、毒訊息比例
- k6 腳本：ingestion 負載與 analytics query 負載
- 四項實驗的可重複執行腳本：
  1. 吞吐量擴展（1 / 2 / 4 workers）
  2. 查詢快取（未快取 vs 快取的 p95 對比）
  3. Worker 失敗（處理中終止 worker 並驗證恢復）
  4. 重複交付（重送 event ID 並驗證每個 ID 僅一筆分析紀錄）
- 結果收集：從 Prometheus 抓取指標，輸出原始資料與摘要
- `benchmarks/README.md`：硬體、指令、資料集、設定、執行時長、原始與摘要結果的紀錄格式（數值先留空，量測後填入）

**不在範圍**：把目標值寫成成果（README 明文禁止）。

**驗收**

- 一百萬事件可由單一指令產生並送出，無需人工介入
- 四項實驗各自有一支可重複執行的腳本並產出 committed 的結果檔
- 快取實驗能產出可量測的 p95 差異數字

```text
/speckit.specify --short-name benchmark-suite
建立 PulseFlow 的負載產生器與 benchmark 套件。需提供合成遙測事件產生器，可設定服務數量、metric 數量、事件總數、送出速率、重複 event_id 比例與毒訊息比例，並能以單一指令產生並送出至少一百萬筆事件而無需人工介入。使用 k6 撰寫 ingestion 負載與 analytics 查詢負載腳本。需提供四項可重複執行的實驗腳本：以 1、2、4 個 worker 執行相同工作負載的吞吐量擴展實驗；比較未快取與已快取分析查詢 p95 延遲的快取實驗；在 ingestion 進行中終止 worker 並驗證恢復的失敗實驗；刻意重送 event ID 並驗證每個 ID 僅產生一筆分析紀錄的重複交付實驗。實驗結果需自 Prometheus 收集並輸出原始資料與摘要檔案。benchmarks 目錄需有說明文件，記錄硬體、執行指令、資料集、設定、執行時長與結果格式；在實際量測完成前，不得把目標值當成已達成的結果寫入。
```

---

### F10 · E2E 測試與 MVP 驗收

- **建議分支/short-name**：`e2e-acceptance`
- **對應 README**：Testing「End-to-end tests must exercise the local container stack」、MVP acceptance criteria 全部十條
- **依賴**：F09

**範圍**

- E2E 測試：對 compose 起的完整 stack 執行 ingest → 處理 → 查詢的完整鏈路
- 涵蓋 DLQ 路徑、重複事件路徑、快取路徑
- 一支「MVP 驗收」腳本，逐條檢查 README 的十項驗收條件並輸出通過／失敗報告
- CI 中執行 E2E（可用較小的資料量）

**驗收**

- 驗收腳本在乾淨環境上從零跑完，十項條件全數通過並產出報告

```text
/speckit.specify --short-name e2e-acceptance
建立 PulseFlow 的端對端測試與 MVP 驗收流程。E2E 測試需針對 docker compose 啟動的完整 stack，執行從事件擷取、worker 處理到分析查詢的完整鏈路，並涵蓋 dead-letter 路徑、重複事件路徑與快取命中路徑。需提供一支 MVP 驗收腳本，逐條檢查 README 中列出的十項驗收條件：compose 可啟動全部依賴、可無人工介入送出一百萬事件、至少三個 worker replica 共享消費、重複 event ID 不產生重複分析紀錄、失敗訊息可在有限重試後進入 dead-letter topic、分析查詢對已知 fixture 回傳正確聚合值、Redis 產生可量測的查詢延遲差異、Prometheus 暴露吞吐量與 lag 與失敗與延遲指標、worker 終止測試不遺失已確認事件、benchmark 腳本與結果已提交，並輸出逐條通過或失敗的報告。E2E 測試需能以較小資料量在 CI 中執行。
```

---

### F11 · Kubernetes 部署（MVP 之後）

- **建議分支/short-name**：`k8s-deployment`
- **對應 README**：Planned technology stack（Kubernetes）、Repository layout `deployments/k8s`
- **依賴**：F10

**範圍**

- API 與 worker 的 Deployment、Service、ConfigMap、Secret
- liveness / readiness probe 對應 F01 的 endpoint
- resource requests/limits、`terminationGracePeriodSeconds`（配合 worker flush）
- worker 副本數可調整；PodDisruptionBudget
- ServiceMonitor 或 Prometheus scrape annotation
- 在 kind / minikube 上的部署驗證步驟文件

**驗收**

- 在本機 kind 叢集上可完成部署，ready probe 通過，調整 worker replica 數會觸發 consumer group rebalance 且不遺失事件

```text
/speckit.specify --short-name k8s-deployment
為 PulseFlow 建立 Kubernetes 部署設定。需提供 API 與 worker 的 Deployment、Service、ConfigMap 與 Secret 定義；liveness 與 readiness probe 對應既有的 /v1/health/live 與 /v1/health/ready；設定 resource requests 與 limits，並依 worker 的 flush 行為設定合適的 terminationGracePeriodSeconds；worker 副本數可調整並設定 PodDisruptionBudget；提供讓 Prometheus 抓取指標的設定。需附上在本機 kind 或 minikube 叢集上的部署與驗證步驟文件，並驗證調整 worker 副本數會觸發 consumer group rebalance 且不遺失事件。
```

---

## 4. Stretch Goals（僅在 F10 驗收通過後開）

README 明確要求 MVP 驗收之後才實作。建議各自獨立成 feature：

| 編號 | Feature | short-name | 依賴 |
| --- | --- | --- | --- |
| F12 | gRPC 擷取端點 | `grpc-ingestion` | F02 |
| F13 | 以 consumer lag 驅動的 Kubernetes HPA | `lag-based-hpa` | F11 |
| F14 | Grafana 儀表板 | `grafana-dashboard` | F08 |
| F15 | Schema 版本相容性測試 | `schema-compat` | F02、F04 |
| F16 | 可設定的保留期／TTL 政策 | `retention-policy` | F03 |
| F17 | 每租戶速率限制 | `tenant-rate-limits` | F02 |

---

## 5. 建議里程碑

| 里程碑 | 內容 | 完成後可展示 |
| --- | --- | --- |
| M1 骨架 | F01 | compose 起得來、健康檢查有依賴感知 |
| M2 端到端最小鏈路 | F02 + F03 + F04 + F06 | 送事件 → 落 ClickHouse → 查到正確聚合值 |
| M3 生產級可靠性與效能 | F05 + F07 + F08 | 重試、DLQ、快取、完整指標與追蹤 |
| M4 可量測與可驗收 | F09 + F10 | 四項 benchmark 有真實數字、十項驗收全過 |
| M5 部署 | F11 | kind 上跑起來、可擴縮 worker |
| M6+ | F12–F17 | 選做 |

---

## 6. 每個 Feature 的建議執行流程

```bash
# 1) 只做一次
/speckit.constitution

# 2) 每個 feature 重複
/speckit.specify --short-name <short-name>  <上方對應的提示詞>
/speckit.clarify      # 回答未明確的設計取捨（批次上限、TTL、重試次數等）
/speckit.plan         # 產生技術方案，會對照 constitution 檢查
/speckit.tasks        # 產生可執行的任務清單
/speckit.analyze      # 檢查 spec / plan / tasks 一致性
/speckit.implement    # 執行
/speckit.checklist    # 需要額外品質檢查時（例如 F09、F10）
```

**特別建議跑 `/speckit.clarify` 的 feature**：F02（批次上限與部分失敗語意）、F05（重試次數與退避參數）、F07（時間範圍正規化與 TTL 策略）、F09（benchmark 的硬體與資料集定義）。
