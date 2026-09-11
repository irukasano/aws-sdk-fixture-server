## HLD

### 2026-09-07 00:00 : Issue #1 初期実装

- 目的: GitHub Issue #1（`docs/HLD/HLD.md をもとに初期実装する`）に基づき、AWS SDK-compatible Fixture Server の初期実装を行う。
- 変更対象: Go 製サーバー本体、Scenario YAML、Fixture Control API、Docker 構成、および JavaScript / TypeScript と Python の AWS SDK 互換テスト。初期対象 Operation は Secrets Manager `GetSecretValue`、S3 `GetObject`/`PutObject`/`HeadObject`、SQS `SendMessage`、Bedrock Runtime `InvokeModel`/`Converse`。
- 非変更対象: HLD で後続対応とされる Cognito、SNS、EventBridge、Bedrock Streaming、および AWS サービス内部状態のエミュレーション。
- 入出力: 入力は AWS SDK 互換 HTTP リクエストと Scenario Fixture YAML、出力は AWS SDK が deserialize 可能な HTTP 応答およびテスト用 Control API 応答を想定する。
- 運用方法: Docker コンテナをポート 4566 で起動し、Control API でシナリオをロード・リセット・履歴取得する。Scenario Fixture はホスト側から `/scenarios` に read-only マウントし、Control API は同ディレクトリ配下の YAML のみをロードする。SDK 互換テストは Docker Compose 上の Node.js 24.20.0 LTS と Python 3.14.7 の各コンテナから実行する。
- 失敗時挙動: 未定義リクエストは対象プロトコルの AWS 形式の HTTP `500` / `UNEXPECTED_AWS_REQUEST` として fail-fast する。Scenario のロード失敗は Control API の `400` または `404` とし、現在の Scenario を破棄する。後続の AWS リクエストも `UNEXPECTED_AWS_REQUEST` として fail-fast する。
- 既存機能への影響: 現在のリポジトリには README と HLD のみで、実装済み機能はない。
- 未確定事項: Go 1.27.1、Node.js 24.20.0 LTS、Python 3.14.7 を使用する。Scenario の `match` は exact、文字列の `*` ワイルドカード、`contains`、正規表現、`optional` をサポートする。`optional` は「パラメータなしなら一致、あれば内側の matcher を評価」とする。`contains` と `regex` は各 matcher 名をキーとする YAML オブジェクトで表記する。複数一致時は `responses` の記述順で最初の定義を選ぶ。defaults はサーバー同梱の `defaults/*.yml` とし、Scenario Fixture が最優先で上書きする。成功 response は AWS API 出力 JSON のキー名で記述し、S3 の本文と追加ヘッダーは `body` / `headers` とする。Fixture error の `status` は任意で、未指定時は `400` とする。Scenario YAML は厳格に検証する。Control API の失敗応答と失敗時の Scenario 破棄を定義した。Reset は Scenario 定義と defaults を保持し、Request History と Sequence counter を初期化する。Sequence の終了後は最後の要素を継続して返す。Request History は request 全体と結果メタデータのみを公開する。Control API は認証なしで、専用クライアントライブラリは提供しない。`GET /__fixture/health` は scenario の有無にかかわらず起動完了時に `200` を返し、SDK テストはこれを待機する。HTTP は `net/http`、YAML は `go.yaml.in/yaml/v3` を使い、Web フレームワークは導入しない。Bedrock Runtime は `InvokeModel` と `Converse` の両方を対象とする。SDK テストは `@aws-sdk/client-*` と `boto3` を現行安定版で固定して使用する。待受ポートは `PORT` で変更可能、未指定時は 4566。`make test` は Go 単体テストと、Fixture Server・JavaScript / TypeScript テスト・Python テストの 3 サービスを定義するルートの `docker-compose.yml` による SDK 互換テストを実行する。各 Operation の protocol mapping は HLD に明記した。request ID は `crypto/rand` による UUID v4。HLD の未確定事項は解消した。
- ユーザー確認が必要な項目: HLD 全体の合意。

### 2026-09-07 00:00 : 並列テスト session 分離

- 目的: 同一 Fixture Server を複数テストケースが並列利用しても、Scenario、Sequence、Request History が競合しないようにする。
- 変更対象: Fixture Server の runtime state、Control API、TypeScript / JavaScript・Python helper、SDK 互換テスト、HLD。
- 非変更対象: AWS サービス状態のエミュレーション、Streaming API。
- 入出力: `POST /__fixture/sessions` がサーバー生成 session ID と `http://aws-fixture:4566/__fixture/sessions/{sessionId}/aws` 形式の endpoint を返し、`DELETE /__fixture/sessions/{sessionId}` が state を破棄する。AWS SDK request は endpoint URL から session ID を識別する。
- 運用方法: テストケースは開始時に session を作成し、helper が process の `AWS_ENDPOINT_URL` を session 固有 endpoint へ設定する。その際に `AWS_ENDPOINT_URL_` で始まるすべての環境変数を保存・一時削除する。Scenario をロードして AWS SDK を利用し、終了時に helper が session を破棄して環境変数を復元する。JavaScript / TypeScript は `node --test` の process isolation を使用し、同一 process 内の concurrent test / subtest は使用しない。Python は `pytest` を使用し、並列化時は `pytest-xdist` の worker process に限定する。
- 失敗時挙動: 存在しない session ID の Control API は `404`、AWS request の endpoint に session ID がない・不明 ID は protocol 別の `400` / `INVALID_FIXTURE_SESSION` を返す。有効 session で Scenario 未ロードまたは不一致の場合は `500` / `UNEXPECTED_AWS_REQUEST` を返す。
- 既存機能への影響: global な current scenario / history / sequence state を session ごとの state に置き換える。以前合意した「専用 helper を提供しない」方針は撤回する。client 作成時に endpoint を明示指定するアプリケーションは helper の対象外とする。
- 未確定事項: なし。
- ユーザー確認が必要な項目: session 分離 HLD 全体の合意。同一 process 内の session 並列実行は非対応。npm / PyPI 公開は行わない。

### 2026-09-07 00:00 : 実装・commit スキル作成

- 目的: Plan に基づく TDD 実装と、Issue 参照付きコミットのためのリポジトリ管理 Codex skill を作成する。
- 変更対象: `.codex/skills/implementation-workflow/SKILL.md` と `.codex/skills/commit-workflow/SKILL.md`。必要な UI metadata は skill initializer の標準出力に限定する。
- 非変更対象: アプリケーション実装、既存 HLD の設計内容、外部サービス、Git push。
- 入出力: 実装スキルは合意済み Plan / HLD を入力とし、独立したサブエージェントによる red テスト作成・HLD 整合レビュー・green 化・commit スキル実行を導く。commit スキルは Plan / HLD / 差分を入力とし、ブランチ末尾の Issue 番号を反映したコミットを出力する。
- 運用方法: リポジトリの `.codex/skills/` に配置し、通常の自動 skill discovery を有効にする。実装スキルは Plan 合意後の実装タスクで使い、commit スキルはユーザーがコミットを求めた場合、またはユーザーが明示的に起動した実装スキルから呼ばれた場合に使う。後者は commit の承認とみなす。
- 失敗時挙動: red テストを再現できない、HLD とテストの不整合がある、または実装後も green にならない場合はコミットせず停止する。commit 対象や Issue 番号を決定できない場合もコミットせず確認する。
- 既存機能への影響: 既存の Codex system skill は変更しない。リポジトリ内の将来の Codex 作業にのみ適用する。
- 未確定事項: なし。
- ユーザー確認が必要な項目: 上記の skill scope と停止条件。

### 2026-09-08 00:00 : PR テンプレートと作成 workflow

- 目的: PR の概要、変更範囲、検証内容、既知の制約を一貫した形式で記載し、変更範囲の書き漏れを防ぐ。
- 変更対象: `.github/PULL_REQUEST_TEMPLATE.md`。後続で合意された場合は、このテンプレートを入力に PR を作成するリポジトリ内 skill。
- 非変更対象: 既存の implementation / commit skill、GitHub Issue の内容、PR の自動マージ。
- 入出力: テンプレート先頭は Issue 番号を埋め込んだ `fixes #{number}`、2 行目は空行とする。続けて `サマリ`（目的・背景を約 200 文字）、`変更範囲`（変更対象を箇条書き）、`テスト`（観点と実行内容を箇条書き）、`暫定判断・懸念`（なければ「なし」）を置く。`#{number}` は PR workflow が `feature/*#number` または `bugs/*#number` のブランチ末尾から取得して置換する。PR title はサマリをさらに短くした変更内容ベースの文とする。workflow は `gh issue view #{number} --json assignees` で元 Issue の Assignees を取得し、確認表示と PR の Assignee に同じ GitHub login を使う。PR 作成に成功した場合、workflow は作成された PR の URL を最後に出力する。
- 運用方法: PR workflow は HLD / Plan / 差分 / 検証結果からテンプレートの各欄と title の下書きを作成し、title・本文・Assignees を確認用に表示する。ユーザーが承認した場合だけ、まず `git push -u origin <current-branch>`、続いて repository の既定 branch を base として `gh pr create` を実行する。Issue に Assignee がいない場合は、PR に Assignee を指定しない。サマリは概要に限定し、レビュー対象の列挙は `変更範囲` に必ず箇条書きで記載する。
- 失敗時挙動: 未コミット変更がある場合、Issue 番号、変更範囲、テスト結果、または暫定判断・懸念を決定できない場合、PR を作成しない。Issue Assignees の取得に失敗した場合、または表示した title / 本文 / Assignees が承認されない場合も push / PR 作成を行わない。push に失敗した場合は PR を作成しない。push 成功後に PR 作成が失敗した場合は、remote branch が残ったことを明示して停止する。
- 既存機能への影響: PR の記載形式のみを追加し、アプリケーションの動作には影響しない。
- 未確定事項: なし。
- ユーザー確認が必要な項目: HLD 全体の合意。

## Plan

### 2026-09-07 00:00 : Issue #1 初期実装

- [x] HLD の残る未確定事項を一つずつ合意する。
- [x] 合意済み HLD に基づき、実装・テスト・Docker 構成を計画する。
- [x] Plan の合意後に実装する。
- [ ] 関連テスト、Docker 起動、および main との差分を確認する。

- [ ] `go.mod` と Go のパッケージ構成を作成し、`net/http`、`go.yaml.in/yaml/v3`、`crypto/rand` のみでサーバー基盤を実装する。
- [ ] Docker SDK 互換テストの実行基盤として、Fixture Server・JavaScript / TypeScript・Python の 3 サービスを定義するルート `docker-compose.yml` と各テストコンテナの設定を先に作成する。
- [ ] Scenario YAML と defaults YAML の厳格な loader・validator・merge を実装する。
- [ ] matcher（exact、`*`、`contains`、`regex`、`optional`）、先頭一致優先、sequence、runtime reset を実装する。
- [ ] AWS request router / normalizer と protocol encoder を実装し、Secrets Manager、S3、SQS、Bedrock Runtime の指定 Operation を対応する。
- [ ] `POST /__fixture/sessions`、`DELETE /__fixture/sessions/{sessionId}`、session-scoped Scenario / reset / history API、`GET /__fixture/health`、失敗時の fail-fast を実装する。
- [ ] Request History、AWS 形式の fixture error / unexpected error、request ID と response defaults を実装する。
- [ ] 単体テストと Fixture を追加し、正常・error・sequence・reset・history・matcher・不正 YAML を検証する。Node.js 24.20.0 LTS と Python 3.14.7 の SDK 互換テストと固定依存ファイルを追加する。
- [ ] Fixture Server 用 `Dockerfile` と `Makefile` を追加する。
- [ ] `make test`、Docker Compose SDK 互換テスト、`go test ./...`、`git diff main...HEAD` を実行して結果を確認する。
- [ ] Review に実装内容、テスト結果、main との差分を記録する。

### 2026-09-07 00:00 : 実装・commit スキル作成

- [x] `skill-creator` の定義を全文確認する。
- [x] 合意済み HLD に基づき `.codex/skills/` に両スキルの雛形を作成する。
- [x] `SKILL.md` と UI metadata に、合意済みの適用範囲・手順・停止条件を記載する。
- [x] 独立レビューでスキル間の矛盾を確認し、検出事項を修正する。
- [x] validator または代替の静的検査で frontmatter・未完了プレースホルダー・差分を検証する。
- [x] Review に作成内容と検証結果を記録する。

### 2026-09-08 00:00 : session 分離を反映した Issue #1 再計画

- [x] session 分離 HLD を合意し、同一 process 内の並列実行を非対応とする。
- [x] 先行 red テストと Compose 設定を独立レビューし、session 分離・依存固定・readiness・カバレッジの不足を確認する。
- [x] Fixture Server 用 `Dockerfile`、healthcheck、3 サービスを定義する `docker-compose.yml`、`Makefile` を整備する。`make test` は Fixture Server を起動後、JavaScript / TypeScript と Python の SDK テストを直列実行する。
- [x] `packages/sdk/javascript`・`packages/sdk/python` に TypeScript / JavaScript・Python のローカル `FixtureSession` helper パッケージと unit test を置き、`tests/sdk` は helper を利用する SDK 互換テスト専用とする。固定済み依存ファイル（npm lockfile、完全固定 `requirements.txt`）も用意する。
- [x] サブエージェントに、session lifecycle / endpoint 環境変数の保存・復元 / Control API / matcher / validation / history / 全対象 protocol・Operation を対象とする最小 red テストを作成させ、Docker 上で red を確認する。本体実装は依頼しない。
- [x] 別のサブエージェントに red テストを HLD・本 Plan と照合させる。不足または不整合を解消し、承認済みの red を確定する。
- [x] Go サーバーを実装する。session resource API、session endpoint routing、Scenario engine、4 protocol・7 Operation、defaults、error、history、healthcheck を HLD の範囲で実装し、反復して green にする。
- [x] session helper を実装し、start / destroy の環境変数復元、session-scoped Control API、process-isolated SDK client 利用を検証する。
- [x] JavaScript / TypeScript では Node.js `node --test` の process isolation、Python では pytest（必要時 pytest-xdist process worker）で SDK 互換テストを実行する。同一 process concurrent test は追加しない。
- [x] `make test`、Docker Compose SDK 互換テスト、Go 単体テスト、`git diff main...HEAD` を実行してログを確認する（基準ブランチは存在しない `main` ではなく `master`）。
- [x] `commit-workflow` を実行し、Plan / HLD / 差分を要約した `refs #1` 付きコミットを作成する。
- [x] Review に変更内容、red / green 証跡、独立レビュー、検証結果、main 差分を記録する。

### 2026-09-08 00:00 : PR テンプレートと作成 workflow

- [x] `skill-creator` の定義を全文再確認し、HLD と skill の必須手順・制約・検証方法を Plan に反映する。
- [ ] `.github/PULL_REQUEST_TEMPLATE.md` に、`fixes #{number}`、空行、サマリ、変更範囲、テスト、暫定判断・懸念の順でテンプレートを作成する。
- [ ] skill initializer を使って `.codex/skills/pull-request-workflow/` を作成し、通常の自動 discovery を有効にした UI metadata を生成する。
- [ ] `SKILL.md` に、Issue 番号の取得、Issue Assignees の取得、HLD / Plan / 差分 / 検証結果に基づく title・本文の下書き、ユーザーへの確認表示、承認後の `git push -u origin <current-branch>`、repository の既定 branch を base とする `gh pr create`、PR URL 出力を記載する。未解決情報・Assignee 取得・push・PR 作成の各失敗時の停止条件も記載する。
- [ ] 別担当のサブエージェントに、HLD とテンプレート・skill の整合、外部変更前の承認境界、push 後の PR 作成失敗時の報告、PR URL 出力をレビューさせる。必要なら skill / template のみを修正する。
- [ ] `quick_validate.py`、`git diff --check`、frontmatter / placeholder / 必須見出し / skill 名の静的確認を実行する。PR 作成・push は実行しない。
- [ ] Review に変更内容、独立レビュー、検証結果を記録する。今回の依頼では commit / push / PR 作成は行わない。

### 2026-09-08 17:18 : PR テンプレートと作成 workflow の実装

- [x] `skill-creator` の定義と `agents/openai.yaml` の UI metadata 規約を全文確認した。
- [x] `.github/PULL_REQUEST_TEMPLATE.md` を、`fixes #{number}`、空行、`サマリ`、`変更範囲`、`テスト`、`暫定判断・懸念` の順で作成する。本文の各セクションは埋める対象を示すコメントにし、具体的な PR 内容は固定しない。
- [x] initializer で `.codex/skills/pull-request-workflow/` と `SKILL.md` / `agents/openai.yaml` を作成する。UI metadata は通常の自動 discovery を維持し、default prompt は `$pull-request-workflow` を明示する。
- [x] skill に、現在ブランチから Issue 番号を取得し、`gh issue view` で Issue title / Assignees を取得する手順を記載する。番号・Assignees・HLD / Plan・差分・検証結果のいずれかを取得・確定できない場合は、push / PR 作成を行わず停止する。
- [x] skill に、変更内容ベースの短い title と、テンプレートの `fixes #{number}` / サマリ / 変更範囲 / テスト / 暫定判断・懸念を HLD / Plan / 差分 / 検証結果から下書きする手順を記載する。Issue Assignees が空なら PR Assignee を指定しない。
- [x] skill に、title・本文・Assignees を表示してユーザーの明示承認を待つ境界を記載する。承認後のみ `git push -u origin <current-branch>` と、repository 既定 branch を base にする `gh pr create` を実行する。push 失敗時は PR を作成せず、PR 作成失敗時は remote branch が残ることを報告し、成功時は PR URL を最後に出力する。
- [x] 独立したサブエージェントに template / skill を HLD・Plan と照合させ、確認前の外部変更、Assignee 引き継ぎ、push / PR 作成失敗、URL 出力の漏れをレビューさせる。検出事項は template / skill のみに反映する。
- [x] `quick_validate.py`、`git diff --check`、frontmatter、skill 名、必須見出し、未完了 placeholder の静的検査を実行する。実装中は push、PR 作成を行わない。commit はユーザーから明示依頼を受けた場合だけ実行する。
- [x] Review に作成内容、独立レビュー、検証結果を記録する。

## Review

### 2026-09-07 00:00 : Issue #1 初期実装

- red テスト: `NewHandler` / `Config` を対象に Secrets Manager と SQS sequence / reset / unexpected request の初期 red テストを作成した。`go test ./...` はローカルに Go がないため実行できなかった。Docker Compose による red は、未作成のルート `Dockerfile` で失敗することを確認した。
- 独立レビュー: HLD / Plan との整合レビューで、Dockerfile、JavaScript lockfile、Python のトランジティブ依存固定、Compose healthcheck、SDK テストの必須 Operation カバレッジ、各種 matcher / validation / error / history のテストが不足していることを確認した。さらに JS / Python テストの並列実行は Scenario・履歴状態を競合させるため、テスト実行方式の再計画が必要となった。

### 2026-09-07 00:00 : 実装・commit スキル作成

- 作成内容: `.codex/skills/implementation-workflow` に、red テスト担当と独立 HLD/Plan レビュー担当のサブエージェントを分ける実装 workflow を追加した。`.codex/skills/commit-workflow` に、Plan/HLD/差分を要約し、`feature/*#number` または `bugs/*#number` の最終番号を `refs #number` として subject の先頭に付与するコミット workflow を追加した。
- レビュー: 独立レビューで、実装 workflow による commit 呼び出しと commit workflow の承認条件が矛盾することを検出した。ユーザーが実装 workflow を明示的に起動した場合は、その commit workflow 呼び出しも承認済みと定義して修正した。
- 検証: `quick_validate.py` は PyYAML が未インストールで実行できず、環境には `pip` も存在しなかった。代替として `git diff --check`、`[TODO]` プレースホルダー不在の確認、各 `SKILL.md` の YAML frontmatter（`name` / `description`）確認、必須ファイル存在確認を実行し成功した。

### 2026-09-08 00:00 : session 分離を反映した Issue #1 再計画

- red テスト: `src/server/http_server_test.go` に session lifecycle・隔離・URL routing・session-scoped Control API・reset/sequence/history・matcher・厳格 YAML validation・全 7 Operation/4 protocol の red テストを追加した。`git diff --check` は成功した。ローカル環境には Go がないため `gofmt` / `go test ./...` は実行不可で、実装前の Docker Compose はルート `Dockerfile` 不在により red となった。
- 独立レビュー: サーバー境界テストは HLD と整合することを確認し、不足していた無効 JSON/YAML、`/scenarios` 内の欠損ファイルの 404、history parameters、fixture error の既定 400 をテストのみで補完した。一方、JS/Python `FixtureSession` helper の環境変数保存・復元、失敗時復元、session-scoped Control API、SDK endpoint 利用を検証する red テストが未作成のため承認保留となった。helper の置き場所はユーザー合意により `packages/sdk/javascript`・`packages/sdk/python` とし、`tests/sdk` は SDK 互換テスト専用とする。red テスト追加後に再レビューする。
- red テスト再レビュー: JS/Python helper の lifecycle、全 `AWS_ENDPOINT_URL_*` の保存・削除・復元、start / destroy の失敗時挙動、および helper 経由の SDK endpoint 利用を追加し、独立レビューで red テスト仕様の承認を得た。
- 実装: Go `net/http` サーバーに session-scoped Control API、Scenario YAML の厳格検証、matcher、sequence/history、7 Operation/4 protocol、AWS 形式 error、UUID v4 request ID、同梱 defaults loader を実装した。JS/Python helper と unit test、固定依存、Compose healthcheck、直列 SDK test 用 Makefile を追加した。
- green 検証: `make go-test`、`npm --prefix packages/sdk/javascript test`、`python3 -m unittest discover -s packages/sdk/python -p 'test_*.py'`、`make sdk-test`、`git diff --check` が成功した。`make test` も defaults loader 追加前の同一構成で成功し、追加後は Go unit test と SDK integration test を再実行して成功した。基準ブランチは `main` ではなく `master` のため、差分確認は `master` を用いた。

### 2026-09-08 17:24 : PR テンプレートと作成 workflow

- 作成内容: `.github/PULL_REQUEST_TEMPLATE.md` に Issue close 行、サマリ、変更範囲、テスト、暫定判断・懸念のテンプレートを追加した。`.codex/skills/pull-request-workflow` に、dirty worktree の停止、Issue / Assignees /既定 base branch の取得、確認表示後の push / PR 作成、失敗時の停止、成功時 URL 出力を定義した。
- 独立レビュー: 必須の変更範囲・テスト結果・暫定判断・懸念を確定できない場合と、既定 base branch を取得できない場合の停止条件が不足していることを検出し、skill に補完した。承認境界、Assignee 引き継ぎ、失敗時挙動、URL 出力は HLD / Plan と整合することを確認した。
- 検証: `quick_validate.py` と UI metadata generator は PyYAML 未導入により `ModuleNotFoundError` となった。代替として frontmatter、metadata の説明長、`$pull-request-workflow` を含む default prompt、テンプレートの必須見出し、未完了 placeholder 不在、`git diff --check` を静的に検査する。commit、push、PR 作成は実行していない。
