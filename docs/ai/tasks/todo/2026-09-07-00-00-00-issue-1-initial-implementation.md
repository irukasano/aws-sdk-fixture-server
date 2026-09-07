## HLD

### 2026-09-07 00:00 : Issue #1 初期実装

- 目的: GitHub Issue #1（`docs/HLD/HLD.md をもとに初期実装する`）に基づき、AWS SDK-compatible Fixture Server の初期実装を行う。
- 変更対象: Go 製サーバー本体、Scenario YAML、Fixture Control API、Docker 構成、および JavaScript / TypeScript と Python の AWS SDK 互換テスト。初期対象 Operation は Secrets Manager `GetSecretValue`、S3 `GetObject`/`PutObject`/`HeadObject`、SQS `SendMessage`、Bedrock Runtime `InvokeModel`/`Converse`。
- 非変更対象: HLD で後続対応とされる Cognito、SNS、EventBridge、Bedrock Streaming、および AWS サービス内部状態のエミュレーション。
- 入出力: 入力は AWS SDK 互換 HTTP リクエストと Scenario Fixture YAML、出力は AWS SDK が deserialize 可能な HTTP 応答およびテスト用 Control API 応答を想定する。
- 運用方法: Docker コンテナをポート 4566 で起動し、Control API でシナリオをロード・リセット・履歴取得する。Scenario Fixture はホスト側から `/scenarios` に read-only マウントし、Control API は同ディレクトリ配下の YAML のみをロードする。SDK 互換テストは Docker Compose 上の Node.js 24.20.0 LTS と Python 3.14.7 の各コンテナから実行する。
- 失敗時挙動: 未定義リクエストは対象プロトコルの AWS 形式の HTTP `500` / `UNEXPECTED_AWS_REQUEST` として fail-fast する。Scenario のロード失敗は Control API の `400` または `404` とし、現在の Scenario を破棄する。後続の AWS リクエストも `UNEXPECTED_AWS_REQUEST` として fail-fast する。
- 既存機能への影響: 現在のリポジトリには README と HLD のみで、実装済み機能はない。
- 未確定事項: Go 1.27.1、Node.js 24.20.0 LTS、Python 3.14.7 を使用する。Scenario の `match` は exact、文字列の `*` ワイルドカード、`contains`、正規表現、`optional` をサポートする。`optional` は「パラメータなしなら一致、あれば内側の matcher を評価」とする。`contains` と `regex` は各 matcher 名をキーとする YAML オブジェクトで表記する。複数一致時は `responses` の記述順で最初の定義を選ぶ。defaults はサーバー同梱の `defaults/*.yml` とし、Scenario Fixture が最優先で上書きする。成功 response は AWS API 出力 JSON のキー名で記述し、S3 の本文と追加ヘッダーは `body` / `headers` とする。Fixture error の `status` は任意で、未指定時は `400` とする。Scenario YAML は厳格に検証する。Control API の失敗応答と失敗時の Scenario 破棄を定義した。Reset は Scenario 定義と defaults を保持し、Request History と Sequence counter を初期化する。Sequence の終了後は最後の要素を継続して返す。Request History は request 全体と結果メタデータのみを公開する。Control API は認証なしで、専用クライアントライブラリは提供しない。`GET /__fixture/health` は scenario の有無にかかわらず起動完了時に `200` を返し、SDK テストはこれを待機する。HTTP は `net/http`、YAML は `go.yaml.in/yaml/v3` を使い、Web フレームワークは導入しない。Bedrock Runtime は `InvokeModel` と `Converse` の両方を対象とする。SDK テスト依存関係は lockfile / 完全固定 requirements で固定する。待受ポートは `PORT` で変更可能、未指定時は 4566。`make test` は Go 単体テストと Docker Compose SDK 互換テストを実行する。各 Operation の protocol mapping は HLD に明記した。request ID は `crypto/rand` による UUID v4。HLD の未確定事項は解消した。
- ユーザー確認が必要な項目: HLD 全体の合意。

## Plan

### 2026-09-07 00:00 : Issue #1 初期実装

- [ ] HLD の残る未確定事項を一つずつ合意する。
- [ ] 合意済み HLD に基づき、実装・テスト・Docker 構成を計画する。
- [ ] Plan の合意後に実装する。
- [ ] 関連テスト、Docker 起動、および main との差分を確認する。

## Review

### 2026-09-07 00:00 : Issue #1 初期実装

- 未着手。
