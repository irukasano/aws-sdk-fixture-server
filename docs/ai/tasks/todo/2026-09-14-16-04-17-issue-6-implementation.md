# Session task record

## HLD

### 2026-09-14 16:04 : Issue #6 implementation

- 目的: GitHub Issue #6 にある、Amazon SES を AWS SDK から決定的にテストできる Fixture 対応。
- Issue に明記された範囲: Scenario routing、AWS SDK 互換の成功・エラー応答、request history、default Scenario、JavaScript/Python SDK 互換テスト。
- Issue に明記された対象外: 実メール送受信、DNS・identity verification・sandbox 等の SES 状態、Cognito からの SES 呼び出し、OTP の生成・状態・期限・照合、既存サービスの挙動変更。
- 合意済み: 対象は SES API v2 のみとし、SES API v1 は対象外とする。
- 確認結果: 現在の `docs/HLD/HLD.md`、`docs/ai/tasks/todo/`、Issue コメント、既存 PR に、SES の対象 Operation、wire protocol、入力/出力、失敗時挙動、default response の仕様、検証コマンドを定めた承認済み HLD/Plan は見当たらない。
- 合意済み: `Content.Simple`、`Content.Raw`、`Content.Template` を業務的に検証せず、正規化した入力を Scenario の照合と Request History に渡す。Scenario が成功またはエラーを決める。
- 合意済み: server 同梱の default Scenario は入力値を問わず `MessageId: fixture-message` を返す。
- 合意済み: 通常の Scenario Fixture は `MessageRejected`・HTTP 400 を定義でき、Fixture Server は SES API v2 形式で返す。Go 単体テストで検証する。
- 合意済み: SES の正規化済みリクエスト全体を session 内 Request History に記録する。これはテスト検証用であり、実メール送信履歴ではない。
- 合意済み: JavaScript/Python の両 AWS SDK で `SendEmail` の default 成功応答と Scenario 定義の `MessageRejected` エラー応答を検証する。
- 合意済み: 対象外の SES API v2 Operation と通常 Scenario に一致しない request は、既存サービスと同じ AWS 形式の HTTP `500` / `UNEXPECTED_AWS_REQUEST` を返す。
- HLD draft: SES API v2 の `SendEmail` だけを REST/JSON で追加する。Scenario routing・error・request history は既存の session 単位の共通機構を使う。新規 default config / Scenario、Go 単体テスト、JavaScript/Python SDK 互換テストを追加する。既存 service、実メール処理、SES 内部状態、v1 API は変更しない。default Scenario は全入力に固定 `MessageId: fixture-message` を返し、通常 Scenario は全本文形式を業務検証せずに照合でき、`MessageRejected` HTTP 400 を返せる。対象外・不一致は `500 UNEXPECTED_AWS_REQUEST` とする。
- 2026-09-14 16:20:02 JST: ユーザーが HLD draft を承認した。
- 2026-09-14 16:09:46 JST: ユーザー指定により、合意済み SES HLD はこの session 記録だけでなく `docs/HLD/HLD.md` にも反映する。

## Plan

### 2026-09-14 16:04 : Issue #6 implementation

- [x] HLD と `implementation-workflow` の必須手順に照らし、SES v2 `SendEmail` の最小 RED test を別サブエージェントが追加する。production code は変更させない。`make go-test` を実行し、default 成功、Scenario `MessageRejected` HTTP 400、全本文形式の正規化・Request History、対象外/不一致の `500 UNEXPECTED_AWS_REQUEST` が未実装のため失敗する出力を保存する。RED の実行結果は 2026-09-14 16:25 JST、`src/server/sesv2_test.go` の 3 test が HTTP 500/XML `UNEXPECTED_AWS_REQUEST` で失敗。
- [x] 別のサブエージェントが RED test を HLD/Plan に独立照合し、成功・エラー・履歴・非対象・意図した未実装箇所で失敗することを確認する。指摘を解消して、承認済みかつ RED の状態を再実行で証明する。独立レビューは承認。unknown operation と unmatched は同一の外部応答を確認するもので、HLD の外部要件を満たす。
- [x] `src/server/http_server.go` の既存 normalize / response / error 共通経路を、SES API v2 の `POST /v2/email/outbound-emails`、service ID `sesv2`、operation `SendEmail` に最小拡張する。JSON payload をそのまま Scenario matcher と history に渡し、SES v2 の JSON 成功 / error をエンコードする。ほかの API、状態管理、本文の業務検証は追加しない。
- [x] `defaults/config/sesv2.yml` と `defaults/scenario/sesv2.yml` を既存の bundled default 構成に追加する。default response は `{ MessageId: fixture-message }` のみとし、既存の default fallback 規則を利用する。
- [x] Go テストを実装とともに GREEN にする。新しいテストと既存テストにより routing、SDK 互換 response/error 形状、default、Scenario error、history、対象外/不一致を検証する。`make go-test` は 2026-09-14 16:30 JST に成功。
- [x] JavaScript の `@aws-sdk/client-sesv2` 固定依存と lockfile、JavaScript/Python の SDK 互換テスト、および Scenario error fixture を追加する。両 SDK で default success の `MessageId` と `MessageRejected` を検証する。JavaScript は `node --test test.mjs`、Python は `pytest` がいずれも成功。
- [x] 完全検証として `make go-test`、`make sdk-test`、`git diff --check`、承認済み HLD/Plan と `main` の差分レビューを実行して結果を確認する。必須検証が失敗または未実施なら停止する。repository に `main` はないため `master` を基準に確認した。`make go-test` は成功。Compose の SDK 検証は同一 Make recipe の `docker compose build`、healthy な `aws-fixture`、JavaScript / Python の `docker compose run --rm --no-deps`、`docker compose down` を実行し、両方成功。`git diff --check` は成功。
- [x] `commit-workflow` を適用する。承認済みスコープだけを stage し、staged diff と `git diff --cached --check` を確認後、`refs #6` を先頭にした commit を作成する。push はしない。2026-09-15 08:23:38 JST に `20e23aa refs #6 Add SES v2 SendEmail fixture` を作成。
- 2026-09-14 16:20:54 JST: ユーザーが Plan を承認した。

## Review

### 2026-09-14 16:04 : Issue #6 implementation

- Work started; no implementation changes yet.
- 2026-09-14 16:05:37 JST: `implementation-workflow` と `commit-workflow` の手順、ローカル規約、既存レッスン、GitHub Issue #6、既存 HLD/作業記録を確認した。承認済み HLD/Plan が未発見のため、テスト追加・実装には進んでいない。
- 2026-09-14 16:13:28 JST: HLD 策定中。合意済み内容を session 記録と `docs/HLD/HLD.md` に反映した。実装・テスト追加は未実施。
- 2026-09-14 16:35:33 JST: RED test と独立レビューを完了し、SES v2 routing/default/error/history と SDK test を実装した。`make go-test` は成功。SDK verification は Docker Compose の `aws-fixture` が host port `4566` の使用中により network 起動できず未完了。外部の port 利用プロセスは変更せず、作成した Compose resource は `docker compose down` で削除した。
- 2026-09-15 08:22:17 JST: ユーザー許可後に port `4566` の Docker container を確認したところ停止済みだった。Compose の fixture server を healthy に起動し、SDK test を実行。Scenario `use_defaults` の既存規則により error definition が default fallback を抑止することを発見したため、default success と error を別 Scenario に分離して修正。JavaScript `node --test test.mjs` と Python `pytest` は成功、`make go-test` と `git diff --check` も成功。独立実装レビューは承認。
