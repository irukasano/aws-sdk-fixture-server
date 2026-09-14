## HLD

### 2026-09-11 16:37:01 JST : Issue #4 Cognito User Pool API と JWT/JWKS Fixture

- 目的: GitHub Issue #4 と `docs/HLD/HLD.md` §17 に従い、Cognito User Pool の指定 AWS SDK API と、OIDC discovery / JWKS / 署名付き JWT の Fixture 境界を追加する。
- 変更対象: Cognito IDP の request routing・AWS JSON protocol response/error encoding・Scenario Fixture、session-scoped endpoint routing、OIDC discovery・JWKS・JWT 生成の公開境界、`defaults/config/` の HTTP response defaults と OIDC 基本設定、サービス単位の `defaults/scenario/<service>.yml` に置く server 同梱 default Scenario（Cognito、Secrets Manager、S3、SQS、Bedrock Runtime の全対応 Operation の基本成功応答）、JWT/JWKS の署名・claim を検証する Go 単体テスト、JavaScript / TypeScript・Python による Cognito API の AWS SDK 互換テスト、必要な固定依存と Docker テスト構成。
- 非変更対象: Cognito SRP・MFA・Hosted UI / Managed login（Cognito 提供のログイン画面、ブラウザのリダイレクト、OAuth 認可フロー）、IAM 評価、Cognito 内部状態やサービスロジックの再現、既存サービスの仕様変更。
- 入出力: session 固有 endpoint を経由した `AdminGetUser`、`AdminCreateUser`、`AdminUpdateUserAttributes`、`ListUsers`、`InitiateAuth`、`RespondToAuthChallenge` の Scenario 成功/error 応答を返す。Cognito User Pool API は `X-Amz-Target: AWSCognitoIdentityProviderService.<Operation>` と `application/x-amz-json-1.0` を使用する。加えて OIDC discovery / JWKS と、それらで検証可能なテスト用署名 JWT を提供する。`defaults/config/cognito-idp.yml` は既存の `service`、`protocol`、`defaults.response.headers` に加えて、`oidc.user_pool_id` と `oidc.signing_key.kid` / `oidc.signing_key.private_key_pem` を定義する。通常 Scenario の `oidc` 定義はこの基本設定を項目単位で上書きし、`oidc.jwks` を明示できる。最終的な OIDC 設定の `user_pool_id` は必須とする。テスト時の issuer は session start HTTP request の `Host` ヘッダーを使う `http://{host}/__fixture/sessions/{sessionId}/oidc/{userPoolId}` とし、その配下の `/.well-known/openid-configuration` と `/.well-known/jwks.json` を提供する。`POST /__fixture/sessions` は default OIDC 設定による issuer を、`POST /__fixture/sessions/{sessionId}/scenario` は Scenario の上書きを反映した issuer を、それぞれ既存の endpoint / scenario 応答に含める。helper はこの返却 issuer を保持する。discovery 応答は `issuer` と当該 session / user pool の `jwks_uri` だけを返し、`authorization_endpoint`・`token_endpoint`・Hosted UI 関連の endpoint は返さない。JWT の `iss` はこの issuer と完全一致させる。Scenario は JWT の `iss` 以外の claim と必須の `expires_in_seconds` を指定し、Fixture Server が session 固有 issuer と、現在時刻から算出した `iat` / `exp` を埋め込んで最終的な OIDC 設定の RSA 2048-bit テスト用秘密鍵で RS256 署名した JWT を返す。`expires_in_seconds` は負値を許可し、期限切れ JWT を表現できる。Fixture Server は出力先に応じて `AccessToken` には `token_use: access` と request の `ClientId` を値とする `client_id`、`IdToken` には `token_use: id` と request の `ClientId` を値とする `aud` を設定する。JWT claim template は `InitiateAuth` と `RespondToAuthChallenge` の `response.AuthenticationResult.AccessToken` / `IdToken` にだけ使用できる。`claims` は必須の `sub` に加えて任意の JSON 値を指定できるが、`iss`、`iat`、`exp`、`token_use`、`client_id`、`aud` は指定できない。JWKS は通常その秘密鍵から導出した公開鍵を返し、最終的な OIDC 設定が明示する `jwks` がある場合だけそれを返す。default Scenario には、この template の書き方と自動設定・予約 claim を説明する YAML コメントを記載する。
- 運用方法: アプリケーション独自のログイン画面から AWS SDK で `InitiateAuth` / `RespondToAuthChallenge` を呼び、返った JWT をアプリケーションが issuer の JWKS で検証するフローを対象とする。既存 FixtureSession helper と Control API の session lifecycle を継続して利用する。`defaults/config/` の HTTP response defaults と OIDC 基本設定、`defaults/scenario/` の server 同梱 default Scenario、テストがロードする通常 Scenario を区別する。通常 Scenario が未ロードなら default Scenario を使用する。通常 Scenario がロード済みで `use_defaults` が未指定または `false` なら通常 Scenario だけを使用する。`use_defaults: true` なら、通常 Scenario に同じ `service + operation` の定義がない request だけ default Scenario を照合する。通常 Scenario に当該 `service + operation` の定義があれば、その match 不一致は fail-fast とする。いずれの Scenario でも `defaults/config/` の HTTP response defaults を補完し、OIDC 設定は `defaults/config/` を基本として通常 Scenario の部分定義を上書きする。default Scenario は各対応 Operation に `match` を持たない基本成功 response を定義し、入力値を問わず一致させる。対応外 Operation は fail-fast を維持する。既存サービスの default Scenario は入力を反映しない固定の最小成功 payload を返し、保存状態を再現しない。default OIDC 設定は `user_pool_id: ap-northeast-1_test`、Repository 管理のテスト専用 RSA 2048-bit 秘密鍵と `kid: fixture-default-rs256` を使用する。default Cognito Scenario は同じ論理ユーザーとして `Username: fixture-user`、`Enabled: true`、`UserStatus: CONFIRMED`、属性なしを返す。認証成功応答は `TokenType: Bearer`、`ExpiresIn: 3600`、固定の `RefreshToken: refresh-token-will-accept-any-value`、および JWT template の `claims.sub: fixture-user`、`expires_in_seconds: 3600` を使用する。RefreshToken の更新・状態管理は行わない。Scenario の match / response / sequence / error は既存方式に従う。アプリケーションの JWT 検証設定には、テスト時に上記 issuer を設定する。JWT 生成は有効な Scenario の鍵・claim・session ID から応答を組み立てるだけで、Cognito の利用者・認証・トークン状態を保持しない。Fixture Server は明示 `jwks` と署名鍵の整合性を検証しない。
- 失敗時挙動: default Scenario またはロード済み通常 Scenario に一致しない request は、既存どおり protocol 互換の `500` / `UNEXPECTED_AWS_REQUEST` とする。有効な session に最終的な OIDC 設定がない OIDC discovery / JWKS request は `409 Conflict` / `OIDC_NOT_CONFIGURED` とする。存在しない session ID の OIDC request は `404` とする。JWT template の必須 `claims.sub`、`expires_in_seconds`、最終的な鍵設定が欠ける場合は通常 Scenario load を既存方式の `400 Bad Request` で失敗させる。server 同梱 defaults の YAML・スキーマ・RSA 鍵・JWT template が不正、必要な defaults file を読めない、または default Scenario 内で同じ service / operation が重複する場合はサーバーを起動失敗にする。`NewHandler(Config)` は `(http.Handler, error)` を返し、`main` はその error で起動失敗にする。Fixture Server は Scenario 内の JWT と JWKS の署名整合性を検証しない。不整合な JWT / JWKS Fixture は利用プログラム側の JWT 検証で失敗する。
- 既存機能への影響: session 分離を維持する。通常 Scenario がロード済みで `use_defaults` が未指定または `false` の既存 fail-fast は維持する。Scenario 未ロード時は新たに server 同梱 default Scenario が基本成功を返し、`use_defaults: true` の通常 Scenario は未定義の `service + operation` だけ default Scenario へ fallback する。
- 未確定事項: なし。JWT 生成用 HTTP API は設けない。OIDC 基本設定のテスト用秘密鍵は `defaults/config/cognito-idp.yml` から与え、通常の JWKS は当該秘密鍵から導出する。Cognito API の output shape は Scenario response に AWS API 出力 JSON のキー名で指定する。
- ユーザー確認が必要な項目: HLD 全体の承認。

## Plan

### 2026-09-14 12:10 : PR #5 review - JavaScript Cognito required inputs

- [ ] InitiateAuth の `AuthParameters` と RespondToAuthChallenge の `ChallengeResponses` が fixture request history に到達する RED test を追加する。
- [ ] 独立レビュー、最小修正、SDK integration と Go test、scope-only commit、review thread reply を実施する。

### 2026-09-14 12:00 : PR #5 review - defaults root compatibility

- [x] `DefaultsRoot` 未指定時に bundled `defaults` を読む既存契約を復元する。
- [x] 最小の RED test で zero-value `Config` の default Scenario と、test 内で OIDC 設定を空にした 409 を確認する。
- [x] 独立レビュー、Go test、差分確認、scope-only commit、review thread reply を実施する。

### 2026-09-11 16:37:01 JST : Issue #4 Cognito User Pool API と JWT/JWKS Fixture

- [x] HLD の未確定仕様を一件ずつ合意し、HLD 全体の承認を得た。
- [x] `implementation-workflow` と `commit-workflow` を再読し、既存 server / helper / SDK test / Docker 構成を調査した。
- [x] HLD と本 Plan の承認を得た。
- [ ] 承認後、production code を変更しない担当に、Cognito / OIDC / JWT / default Scenario / `use_defaults` の最小 red test を追加させ、Docker の Go test で red 証跡を保存する。
- [ ] 別担当に red test を HLD / Plan と照合させ、6 Cognito Operation、OIDC failure、default fallback、非目標、意図した未実装による失敗をレビューさせる。指摘を解消後も red であることを確認する。
- [ ] `defaults/*.yml` を `defaults/config/` に移し、HTTP response defaults と OIDC 基本設定を厳格にロードする。`defaults/scenario/<service>.yml` の全対応 Operation 基本成功 response を起動時に検証・集約し、重複・欠損・不正 YAML / schema / RSA 鍵 / JWT template では起動を失敗させる。`NewHandler(Config)` を `(http.Handler, error)` に変更し、`main` と既存 caller を更新する。
- [ ] runtime state を default Scenario と通常 Scenario に対応させる。通常 Scenario 未ロード時の default 使用、`use_defaults: false` の strict fail-fast、`use_defaults: true` の `service + operation` 単位 fallback、sequence counter の source 分離を実装する。
- [ ] Cognito IDP の target 正規化、aws-json-1.0 response/error、6 Operation の Scenario response を実装する。`defaults/config/cognito-idp.yml` の OIDC 基本設定と通常 Scenario の部分上書きを解決する。
- [ ] session-scoped OIDC discovery / JWKS routes、session start / Scenario load 応答の issuer、RS256 JWT template の claim 注入・署名、明示 JWKS override、OIDC 409 / 404 を実装する。Hosted UI / Managed login、Cognito 状態、RefreshToken 更新は実装しない。
- [ ] JavaScript / Python FixtureSession helper に Control API の issuer を保持・Scenario load 後に更新する処理と unit test を追加する。JavaScript SDK 依存を固定し、両言語から default Scenario の全既存 / Cognito Operation 成功と、6 Cognito Operation の Scenario error deserialize を検証する。
- [ ] 各意味のある変更後に関連 Go / helper / SDK test を実行して green にし、Docker 内で Go format を実行する。
- [ ] 完全検証として `make go-test`、helper unit test を含む `make sdk-test`、`make test`、`git diff --check`、`git diff master...HEAD` と worktree diff を実行する。Go test では discovery / JWKS、JWT 署名成功・不整合 JWKS による失敗・必須 claim・期限切れ、`AccessToken` / `IdToken` ごとの `token_use` 自動設定と template による上書き拒否、固定 token 文字列をそのまま返すケースと JWT template から生成するケース、Scenario 未ロード時の default Scenario・strict fail-fast・`use_defaults` fallback を検証する。
- [ ] HLD / Plan に対する差分を独立レビューし、Review に原因・実装・red / green 証跡・完全検証・`master` 差分を記録する。
- [ ] `commit-workflow` により、対象ファイルだけを `refs #4` 付きで commit する。push は行わない。

## Review

### 2026-09-11 16:37:01 JST : Issue #4 Cognito User Pool API と JWT/JWKS Fixture

- 実装: Cognito IDP six operations、session-scoped OIDC discovery/JWKS、RS256 JWT template、bundled default Scenario と strict defaults validation、SDK helper issuer を実装した。
- RED: `NewHandler` API 移行後、未実装 Cognito/OIDC/default/JWT の行動失敗を確認した。
- GREEN: `make go-test`、JS/Python helper unit test、Docker の JS/Python SDK integration test、`git diff --check` を通過した。
