# AWS SDK Fixture Server

## High Level Design

## 1. Overview

本システムは、ローカル開発・結合テスト向けの **AWS SDK-compatible Fixture Server** を提供する。

AWSサービスそのものをエミュレートすることは目的としない。

各言語のAWS SDKから送信されたHTTPリクエストを受信し、テスト実行時に指定された **Scenario Fixture** に基づいて、AWS SDK互換のHTTPレスポンスを返却する。

主な用途は以下とする。

* AWS SDKを含むアプリケーション結合テスト
* AWS API境界のテスト
* 正常系・異常系・リトライ処理のテスト
* AWSサービスを利用するアプリケーションのローカル実行
* CI環境における決定論的テスト

本システムはAWSサービスの内部状態や内部ロジックを再現しない。

---

# 2. Goals

## 2.1 Primary Goals

以下を実現する。

1. Dockerコンテナとして簡単に起動できる
2. 各言語のAWS SDKからAWS APIと同様の方法でアクセスできる
3. アプリケーション側はAWS SDKのendpoint overrideのみで利用できる
4. Scenario FixtureをYAMLで定義できる
5. テストケースごとにScenario Fixtureをロードして実行できる
6. 正常レスポンスをScenario Fixtureとして定義し、AWS SDK互換レスポンスとして返却できる
7. AWSエラーをScenario Fixtureとして定義し、AWS SDK互換エラーとして返却できる
8. 同一APIへの連続呼び出しに異なるレスポンスを返却できる
9. 呼び出されたAWS APIを記録できる
10. 想定したAWS API呼び出しが行われたことを検証できる
11. Scenario Fixtureに定義されていない想定外API呼び出しを検出できる

---

# 3. Non-Goals

以下は本システムでは実装しない。

* AWSサービス内部ロジックの再現
* AWSサービス状態の完全な保持
* IAM Policy評価
* AWS Network / VPC挙動の再現
* eventual consistencyの再現
* AWSサービス間連携の自動再現
* SQS visibility timeout等のサービス固有内部挙動
* S3の実ストレージとしての動作
* Cognito SRP認証の完全再現
* Cognito MFAの完全再現
* Cognito Hosted UIの再現
* AWS Consoleの再現
* LocalStack互換性
* AWSインフラ構成そのものの検証

本システムの位置付けは以下とする。

> This server does not emulate AWS services.
> It provides deterministic AWS SDK-compatible request/response fixtures.

---

# 4. Target Architecture

```text
+-------------------------+
| Application             |
|                         |
| AWS SDK                 |
+-----------+-------------+
            |
            | AWS HTTP Request
            v
+-------------------------+
| AWS Fixture Server      |
|                         |
| Request Router          |
|       |                 |
|       v                 |
| Protocol Decoder        |
|       |                 |
|       v                 |
| Scenario Matcher        |
|       |                 |
|       v                 |
| Response Builder        |
|       |                 |
|       v                 |
| Protocol Encoder        |
+-----------+-------------+
            |
            | AWS-compatible
            | HTTP Response
            v
         AWS SDK
```

Scenario FixtureはFixture Server本体と分離する。

```text
AWS Fixture Server
        +
Project-specific Scenario Fixtures
```

例:

```text
project/
  docker-compose.yml

  tests/
    aws/
      scenarios/
        happy-path.yml
        secret-not-found.yml
        s3-not-found.yml
        sqs-throttling.yml
```

---

# 5. Core Design Principles

## 5.1 Stateless by Default

AWSサービスの状態を内部で再現しない。

例えばS3について、以下のようなサービスエミュレーションは原則行わない。

```text
PutObject
↓
Fixture Server内部ストレージへ保存
↓
GetObject
↓
保存済みObjectを返却
```

代わりに、

```text
AWS Request
↓
Scenario Match
↓
Configured Response
```

のみを実行する。

Sequence Response等、テスト実行のために必要な最小限の一時状態のみ保持する。

---

## 5.2 Scenario-driven

AWS APIの結果はScenario Fixtureによって決定する。

同一Scenarioに対して決定論的な結果を返却できることを重視する。

---

## 5.3 AWS SDK Boundary Compatibility

AWSサービスそのものの互換性ではなく、以下の境界が成立することを保証対象とする。

```text
Application
    ↓
AWS SDK
    ↓
AWS-compatible HTTP Request
    ↓
Fixture Server
    ↓
AWS-compatible HTTP Response
    ↓
AWS SDK deserialize
    ↓
Application
```

---

## 5.4 Explicit Failure

Scenario Fixtureに一致しないAWS APIが呼び出された場合、暗黙の成功レスポンスを返さない。

例:

```text
UNEXPECTED_AWS_REQUEST

service: s3
operation: ListBuckets
```

テスト時にはfail-fastを基本とする。

Scenario Fixture に一致しない AWS リクエストは、対象サービスの protocol に応じた AWS 形式のエラー本文を HTTP `500` で返す。エラーコードは `UNEXPECTED_AWS_REQUEST` とし、AWS SDK がエラーとして deserialize できるようにする。

---

## 5.5 Scenario is Project-owned

Scenario FixtureはFixture Server本体には含めない。

Fixture ServerはAWS SDK互換境界を提供し、各プロジェクトがテスト対象に応じたScenario Fixtureを管理する。

---

# 6. Major Components

## 6.1 HTTP Server

責務:

* HTTP request受付
* request headers取得
* path/query取得
* request body取得
* AWS互換HTTP response返却

初期実装では単一HTTPポートを使用する。

Default:

```text
4566
```

待受ポートは `PORT` 環境変数で変更可能とし、未指定時は `4566` を使用する。

---

## 6.2 AWS Request Router

AWS SDKから受信したHTTP Requestから以下を識別する。

```text
service
operation
protocol
```

例:

Secrets Manager:

```text
x-amz-target:
  secretsmanager.GetSecretValue
```

S3:

```text
HTTP Method
Path
Query Parameter
```

等からoperationを解決する。

---

## 6.3 Protocol Adapter

AWSサービス間のwire protocol差異を吸収する。

初期対応候補:

```text
aws-json-1.0
aws-json-1.1
rest-json
rest-xml
```

初期対象 Operation の protocol は以下とする。

| Service / Operation | Protocol |
| --- | --- |
| Secrets Manager `GetSecretValue` | aws-json-1.1 |
| SQS `SendMessage` | aws-json-1.0 |
| S3 `GetObject` / `PutObject` / `HeadObject` | rest-xml |
| Bedrock Runtime `InvokeModel` / `Converse` | rest-json |

責務:

```text
HTTP Request
    ↓
Normalized Request
```

および、

```text
Normalized Response
    ↓
AWS-compatible HTTP Response
```

---

## 6.4 Scenario Loader

Scenario FixtureをYAMLからロードする。

責務:

* YAML parse
* Scenario validation
* Matcher definition生成
* Response definition生成
* Sequence初期化
* 現在のScenario切替

Scenario Loaderは起動時だけではなく、**テスト実行時にもScenarioを切り替えられること**を必須とする。

Scenario YAML はロード時に厳格に検証する。未知のフィールド、未知の matcher、同一 response 定義における `response`・`error`・`sequence` の複数指定、または空の `sequence` はロードエラーとする。

Fixture Server は session ごとに Scenario、Sequence counter、Request History を分離して保持する。異なる session の状態は相互に影響しない。

---

## 6.5 Scenario Matcher

Normalized RequestとScenario Fixtureを比較する。

AWS SDKが生成する動的なHTTP要素ではなく、意味的なAWS API parameterを中心にmatchする。

---

## 6.6 Response Builder

Scenario Fixtureで指定されたresponse/errorと共通defaultからNormalized Responseを生成する。

---

## 6.7 Request History

Fixture Serverへ送信されたNormalized Requestを保持する。

主用途:

* API呼び出し確認
* テスト失敗時の診断
* expected request verification

---

# 7. Internal Normalized Request Model

AWS protocol差異を内部表現へ変換する。

例:

```json
{
  "service": "secretsmanager",
  "operation": "GetSecretValue",
  "parameters": {
    "SecretId": "test/db"
  },
  "http": {
    "method": "POST",
    "path": "/"
  }
}
```

S3:

```json
{
  "service": "s3",
  "operation": "GetObject",
  "parameters": {
    "Bucket": "test-bucket",
    "Key": "foo.txt"
  },
  "http": {
    "method": "GET",
    "path": "/test-bucket/foo.txt"
  }
}
```

Scenario Matcherは可能な限りNormalized Requestのみを扱い、AWS wire protocolの詳細を意識しない。

---

# 8. Scenario Fixture Design

## 8.1 Basic Structure

```yaml
name: happy-path

responses:

  - service: secretsmanager
    operation: GetSecretValue

    match:
      SecretId: test/db

    response:
      Name: test/db
      SecretString: |
        {"host":"db","username":"test"}
```

---

## 8.2 S3 Example

```yaml
name: s3-get-object

responses:

  - service: s3
    operation: GetObject

    match:
      Bucket: test-bucket
      Key: foo.txt

    response:
      body: |
        hello world

      headers:
        Content-Type: text/plain
        ETag: '"abc123"'
```

Scenario Fixtureには必要な差分だけを書くことを基本とする。

AWS SDK互換性のために必要な定型header等はFixture Server側で補完する。

`response` および `sequence[].response` のフィールドは、対象 AWS API の出力 JSON と同じキー名で記述する。S3 の本文は `body`、追加 HTTP ヘッダーは `headers` で記述する。

---

# 9. Response Defaults

AWS SDK互換性に必要な定型HTTP responseはScenario Fixture側へ毎回記述しない。

Fixture Server側でprotocol/service/operation単位のdefaultを持つ。

例:

```yaml
service: secretsmanager
protocol: aws-json-1.1

defaults:

  response:
    headers:
      Content-Type: application/x-amz-json-1.1
      x-amzn-requestid: ${request_id}
```

S3:

```yaml
service: s3
protocol: rest-xml

defaults:

  response:
    headers:
      x-amz-request-id: ${request_id}
```

マージ順序:

```text
Protocol Default
      ↓
Service Default
      ↓
Operation Default
      ↓
Scenario Fixture
```

Scenario Fixture側を最優先とする。

defaults は Fixture Server に同梱する `defaults/*.yml` として管理する。Scenario Fixture は `/scenarios` から read-only でロードし、defaults とは別に扱う。マージ後は Scenario Fixture の値を最優先とする。

---

# 10. Error Response

AWSエラーもScenario Fixtureとして定義可能とする。

例:

```yaml
responses:

  - service: secretsmanager
    operation: GetSecretValue

    match:
      SecretId: missing-secret

    error:
      type: ResourceNotFoundException
      message: Secret not found
```

Fixture Server側が対象protocolに応じてAWS SDKがdeserialize可能なHTTP responseへ変換する。

`error.status` は任意で指定できる。指定がない場合の HTTP status は `400` とする。

```yaml
error:
  type: ThrottlingException
  message: throttled
  status: 429
```

例:

```text
HTTP 400
Content-Type: application/x-amz-json-1.1
```

Body:

```json
{
  "__type": "ResourceNotFoundException",
  "message": "Secret not found"
}
```

---

# 11. Sequence Responses

AWS SDKやアプリケーション側のretry処理をテストするため、同一requestに対して複数responseを順番に返却できること。

例:

```yaml
responses:

  - service: sqs
    operation: SendMessage

    match:
      QueueUrl: "*"

    sequence:

      - error:
          type: ThrottlingException
          message: throttled

      - error:
          type: ThrottlingException
          message: throttled

      - response:
          MessageId: test-message-001
```

挙動:

```text
1st call → ThrottlingException
2nd call → ThrottlingException
3rd call → Success
```

Sequence counterはScenario切替またはreset時に初期化する。

すべての sequence 要素を返却した後は、最後の要素を以後の一致リクエストに対して継続して返す。

---

# 12. Scenario Loading and Test Execution

Scenario FixtureはDocker起動時に固定するだけではなく、**テストケースごとにロードして実行できること**を必須とする。

想定フロー:

```text
Fixture Server 起動
        ↓
Test Case A
        ↓
happy-path.yml load
        ↓
Test実行
        ↓
Fixture Server reset
        ↓
Test Case B
        ↓
secret-not-found.yml load
        ↓
Test実行
```

これにより、Fixture Serverコンテナ自体は複数テストケースで使い回す。

---

# 13. Test Control API

テストコードからFixture Serverを制御するための最小限のControl APIを提供する。

一般的な運用管理APIではなく、**テスト用Scenario制御API**として位置付ける。

初期実装では Control API に認証を設けない。ローカルまたはテスト専用の Docker ネットワークでの利用を前提とし、AWS API と同じポートの `__fixture` パスで提供する。

初期実装では TypeScript / JavaScript および Python 向けの Fixture helper を提供する。helper は session の lifecycle を管理し、session 固有 endpoint をテスト process の AWS endpoint 環境変数へ一時設定する。

## 13.0 Session Lifecycle

```http
POST   /__fixture/sessions
DELETE /__fixture/sessions/{sessionId}
```

`POST /__fixture/sessions` は Fixture Server が生成した session ID と session 固有 endpoint を返す。`DELETE /__fixture/sessions/{sessionId}` は該当 session の Scenario、Sequence counter、Request History を破棄する。helper は start 時に endpoint 環境変数を session 固有 endpoint へ設定し、destroy 時に元の値を復元する。

helper は start 時に現在の `AWS_ENDPOINT_URL` と、`AWS_ENDPOINT_URL_` で始まるすべての環境変数を保存する。次に `AWS_ENDPOINT_URL_` で始まる環境変数を一時削除し、`AWS_ENDPOINT_URL` だけを session 固有 endpoint に設定する。destroy 時には保存した環境変数を正確に復元する。これにより対応サービスの追加時に service-specific endpoint 設定の漏れを生じさせない。

`FixtureSession.start()` の session 作成に失敗した場合、helper は環境変数を変更しない。`destroy()` の session 削除に失敗した場合も、helper は環境変数を必ず復元して削除エラーを呼び出し元へ返す。

```json
{
  "sessionId": "…",
  "endpoint": "http://aws-fixture:4566/__fixture/sessions/{sessionId}/aws"
}
```

session 固有 endpoint の `/aws` prefix は Control API と AWS API を分離する。Fixture Server は AWS SDK request の path からこの prefix を外し、通常の AWS routing を行う。

AWS SDK request は session 固有 endpoint の URL path から session ID を識別する。Fixture Server は session ID を用いて request を対応する runtime state に振り分ける。custom session header は使用しない。

存在しない session ID を指定した Control API は `404 Not Found` を返す。AWS SDK request の endpoint に session ID がない、または session ID が存在しない場合は、対象 protocol に応じた AWS 形式の HTTP `400` を返す。エラーコードは `INVALID_FIXTURE_SESSION` とする。AWS JSON protocol では `__type`、REST/XML では `<Code>` にこの値を設定する。

並列実行は OS process 単位でのみ対応する。helper が process 全体の endpoint 環境変数を設定するため、同一 process 内で複数 session を同時に開始して AWS SDK request を実行してはならない。

JavaScript / TypeScript の SDK 互換テストは Node.js 組み込み test runner を `node --test` の process isolation で実行する。複数 test file の並列実行は許可するが、同一 process 内で並列化する `concurrency: true` の test / subtest は使用しない。ほかの test runner を使用する場合も、同等の process isolation を維持する。

Python の SDK 互換テストは `pytest` を使用する。並列実行する場合は `pytest-xdist` の worker process に限定し、同一 process 内で複数 session を同時利用しない。

`FixtureSession` helper は TypeScript / JavaScript と Python のローカルパッケージとして本リポジトリに含める。SDK 互換テストはこの helper を直接利用する。初期実装では npm および PyPI への公開は行わない。

TypeScript / JavaScript helper は `FixtureSession.start({ serverUrl })`、`loadScenario(path)`、`reset()`、`requests()`、`destroy()` を提供する。Python helper は同じ責務を `start`、`load_scenario`、`reset`、`requests`、`destroy` で提供する。

helper の実装と unit test はそれぞれ `packages/sdk/javascript` および `packages/sdk/python` に置く。`tests/sdk/javascript` と `tests/sdk/python` は helper を利用して AWS SDK との互換性を検証する用途に限定する。

## 13.5 Health Check

```http
GET /__fixture/health
```

サーバーがリクエストを受け付け可能な場合は `200 OK` を返す。Scenario のロード有無には依存しない。Docker Compose の JavaScript / TypeScript および Python テストコンテナは、このエンドポイントが `200 OK` を返してからテストを開始する。

## 13.1 Load Scenario

```http
POST /__fixture/sessions/{sessionId}/scenario
```

Request:

```json
{
  "path": "/scenarios/secret-not-found.yml"
}
```

処理:

```text
現在のScenario破棄
↓
指定Scenario読込
↓
Sequence counter初期化
↓
Request History初期化
```

Response:

```json
{
  "scenario": "secret-not-found",
  "loaded": true
}
```

---

## 13.2 Reset

```http
POST /__fixture/sessions/{sessionId}/reset
```

以下を初期化する。

```text
current scenario runtime state
sequence counters
request history
```

Scenario定義そのものを保持するか破棄するかは実装時に決定するが、初期実装では保持してruntime stateのみresetする方式を推奨する。

初期実装では Scenario 定義および defaults を保持する。`POST /__fixture/sessions/{sessionId}/reset` は Request History と Sequence counter を初期化し、同一 Scenario を最初の呼び出しから再実行できる状態に戻す。Scenario 全体を切り替える場合は `POST /__fixture/sessions/{sessionId}/scenario`、破棄する場合は `DELETE /__fixture/sessions/{sessionId}` を使用する。

---

## 13.3 Request History

```http
GET /__fixture/sessions/{sessionId}/requests
```

例:

```json
[
  {
    "service": "secretsmanager",
    "operation": "GetSecretValue",
    "parameters": {
      "SecretId": "test/db"
    }
  }
]
```

テストコードからAPI呼び出し内容を検証できる。

Request History は Normalized Request の `service`、`operation`、`parameters`、HTTP の `method` と `path` を返す。加えて、返却結果のメタデータとして `status`、`kind`（`response`、`error`、`unexpected`）、適用した `responseIndex`、およびエラー時の `errorType` を返す。レスポンス本文とヘッダーは保存・公開しない。

---

## 13.4 Verification

初期実装ではRequest Historyを取得してテスト側で検証できれば成立する。

将来的には以下のような専用APIを追加してもよい。

```http
POST /__fixture/verify
```

例:

```json
{
  "service": "s3",
  "operation": "PutObject",
  "match": {
    "Bucket": "test",
    "Key": "foo.txt"
  },
  "times": 1
}
```

専用verification APIは初期必須要件とはしない。

---

# 14. Request Matching

MatcherはHTTP request全体を厳密比較しない。

通常、以下はmatch対象外とする。

```text
Authorization
X-Amz-Date
User-Agent
Content-Length
SDK version metadata
Request ID
```

意味のあるAWS API parameterを中心に比較する。

例:

```yaml
match:
  Bucket: test
  Key: foo.txt
```

Matcherの初期候補:

```text
exact
wildcard
contains
regex
optional
```

例:

```yaml
match:
  QueueUrl: "*"

  MessageBody:
    contains: patientId
```

初期実装では複雑なmatcher DSLを作り込みすぎず、exact matchを中心に開始する。

初期実装で対応する matcher は、exact、文字列の `*` ワイルドカード、`contains`、正規表現、および `optional` とする。

`optional` は対象パラメータが存在しない場合も一致とし、存在する場合は内側に指定した matcher を評価する。表記は以下とする。

```yaml
match:
  ClientRequestToken:
    optional:
      regex: '^[a-z0-9-]+$'
```

`contains` および `regex` は以下の表記とする。

```yaml
match:
  MessageBody:
    contains: patientId
  QueueUrl:
    regex: '^https://.+/queue$'
```

複数の response 定義が同じリクエストに一致する場合、`responses` の記述順で最初に一致した定義を使用する。

---

# 15. Initial Target Services

初期実装では以下を優先する。

## 15.1 Secrets Manager

初期Operation:

```text
GetSecretValue
```

追加候補:

```text
DescribeSecret
PutSecretValue
```

---

## 15.2 S3

初期Operation:

```text
GetObject
PutObject
HeadObject
```

追加候補:

```text
DeleteObject
ListObjectsV2
```

---

## 15.3 SQS

初期Operation:

```text
SendMessage
```

追加候補:

```text
ReceiveMessage
DeleteMessage
```

---

## 15.4 Bedrock Runtime

Bedrock RuntimeもFixture方式の対象とする。

初期対象は **非Streaming APIのみ** とする。

初期候補:

```text
InvokeModel
Converse
```

例:

```yaml
responses:

  - service: bedrock-runtime
    operation: Converse

    match:
      modelId: test-model

    response:
      output:
        message:
          role: assistant
          content:
            - text: fixture response
```

以下は初期対象外とする。

```text
ConverseStream
InvokeModelWithResponseStream
```

Streaming APIはAWS EventStream等のprotocol対応が必要になるため、後続対応とする。

---

## 15.5 Amazon SES

Amazon SES は API v2 の `SendEmail` だけを Fixture 方式の対象とする。SES API v1 は対象外とする。

`SendEmail` は REST/JSON の `POST /v2/email/outbound-emails` としてルーティングする。Fixture Server は `Content.Simple`、`Content.Raw`、`Content.Template` のいずれかを業務的に検証せず、正規化した入力を Scenario の照合と Request History に渡す。一致した Scenario が定義する成功またはエラー応答を返す。

server 同梱の default Scenario は入力値を問わず `MessageId: fixture-message` を返す。

通常の Scenario Fixture は `MessageRejected`・HTTP 400 のエラー応答を定義できる。Fixture Server はこれを SES API v2 形式で返し、Go 単体テストで検証する。

SES の正規化済みリクエスト全体は、既存の session 内 Request History に記録する。これは Control API または helper からテストが検証するための記録であり、実メールの送信履歴ではない。

JavaScript と Python の AWS SDK 互換テストは、default Scenario の成功応答と、Scenario Fixture が定義する `MessageRejected` エラー応答を `SendEmail` 経由でそれぞれ検証する。

対象外の SES API v2 Operation、および通常 Scenario に一致しない request は、既存サービスと同じ AWS 形式の HTTP `500` / `UNEXPECTED_AWS_REQUEST` を返す。

実メール送受信、DNS、SES identity 検証、sandbox 状態、メール本文の妥当性検証、OTP の生成・状態管理・有効期限・照合は行わない。

---

# 16. Later Target Services

初期実装後の追加候補とする。

```text
SNS
EventBridge
Cognito User Pool API
Cognito JWT/JWKS
Bedrock Streaming API
```

---

# 17. Cognito Support

Cognitoは初期実装対象には含めず、後続対応とする。

Cognito対応時は以下の2領域を分離する。

## 17.1 Cognito AWS SDK API

例:

```text
AdminGetUser
AdminCreateUser
AdminUpdateUserAttributes
ListUsers
InitiateAuth
RespondToAuthChallenge
```

これらは通常のScenario Fixture方式で扱う。

例:

```yaml
- service: cognito-idp
  operation: AdminGetUser

  match:
    UserPoolId: ap-northeast-1_test
    Username: user@example.com

  response:
    Username: user@example.com
    Enabled: true
    UserStatus: CONFIRMED
```

---

## 17.2 Cognito JWT / JWKS

アプリケーションがCognito JWTを検証する場合には、AWS SDK APIとは別にOIDC/JWT境界への対応が必要となる。

将来対応候補:

```text
/.well-known/openid-configuration
/.well-known/jwks.json
```

テスト用秘密鍵を利用し、以下を含むJWTを生成可能とする。

```text
iss
sub
exp
iat
token_use
client_id / aud
kid
signature
```

---

# 18. AWS SDK Configuration

アプリケーション側では、Fixture Server を使用するテスト時に endpoint を Fixture Server へ向ける。提供する TypeScript / JavaScript または Python helper は session 固有 endpoint を process の endpoint 環境変数へ設定するため、アプリケーション内で生成する AWS SDK client も該当 session に向く。

AWS SDK client の作成時に endpoint を明示指定するアプリケーションは FixtureSession の対象外とする。明示指定された endpoint は環境変数より優先されるため、helper はこれを上書きしない。

例:

```text
AWS_ENDPOINT_URL=http://aws-fixture:4566
```

またはAWS SDKが対応している場合はサービス単位で指定する。

例:

```text
AWS_ENDPOINT_URL_S3=http://aws-fixture:4566
AWS_ENDPOINT_URL_SECRETS_MANAGER=http://aws-fixture:4566
```

ローカルテスト用Credential:

```text
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
AWS_REGION=ap-northeast-1
```

Fixture Serverは初期実装ではCredentialの妥当性やIAM権限を評価しない。

---

# 19. Docker Execution

Fixture ServerはDockerコンテナとしてローカル起動できること。

配布方法やDocker Registryについては本HLDの対象外とする。

想定Repository:

```text
aws-fixture-server/
  Dockerfile
  src/
  defaults/
  tests/
```

Build:

```bash
docker build -t aws-fixture-server .
```

Run:

```bash
docker run \
  --rm \
  -p 4566:4566 \
  -v ./tests/aws/scenarios:/scenarios:ro \
  aws-fixture-server
```

コンテナ起動後、各テストケースがControl APIを利用してScenario Fixtureをロードする。

例:

```text
POST /__fixture/sessions/{sessionId}/scenario
```

```json
{
  "path": "/scenarios/happy-path.yml"
}
```

`path` は `/scenarios` 配下の YAML ファイルに限定する。Docker 実行時はホスト側の Scenario Fixture ディレクトリを `/scenarios` へ read-only でマウントする。

Scenario のロードに失敗した場合、Control API は以下を返す。

- 無効な JSON、無効な YAML、Fixture スキーマ不正、または `/scenarios` 外のパス: `400 Bad Request`
- `/scenarios` 配下だが存在しない YAML: `404 Not Found`
- 応答本文: `{"error":"...", "message":"..."}`

ロード失敗時は現在の Scenario を破棄する。以後の AWS リクエストは `UNEXPECTED_AWS_REQUEST` として fail-fast する。テストコードは Control API の非 2xx 応答を検出して、その時点でテストを失敗させる。

---

# 20. Example docker-compose.yml

```yaml
services:

  aws-fixture:
    build:
      context: ./aws-fixture-server

    ports:
      - "4566:4566"

    volumes:
      - ./tests/aws/scenarios:/scenarios:ro

  app:
    build: .

    environment:
      AWS_ACCESS_KEY_ID: test
      AWS_SECRET_ACCESS_KEY: test
      AWS_REGION: ap-northeast-1
      AWS_ENDPOINT_URL: http://aws-fixture:4566

    depends_on:
      - aws-fixture
```

Test Runnerから、

```text
aws-fixture
↓
Scenario load
↓
app test
↓
Request verification
```

の順で実行する。

---

# 21. Repository Structure

想定構成:

```text
aws-fixture-server/

  src/

    server/
      http-server.*

    router/
      aws-router.*

    protocols/
      aws-json-1.0.*
      aws-json-1.1.*
      rest-json.*
      rest-xml.*

    services/
      secretsmanager.*
      s3.*
      sqs.*
      bedrock-runtime.*

    scenario/
      loader.*
      matcher.*
      sequence.*

    response/
      builder.*

    fixture-control/
      api.*

    history/
      request-history.*

  defaults/
    secretsmanager.yml
    s3.yml
    sqs.yml
    bedrock-runtime.yml

  tests/

  Dockerfile

  docker-compose.yml

  README.md
```

---

# 22. Smithy Integration

将来的にはAWS Smithyモデルの利用を検討する。

目的:

```text
service definition
operation definition
input shape
output shape
protocol metadata
error definition
```

等をSmithyモデルから取得し、serviceごとの手動定義を減らす。

想定:

```text
AWS Smithy Model
       ↓
Service Metadata
       ↓
Protocol Adapter
       ↓
Scenario Engine
```

ただし初期実装ではSmithyへの依存を必須としない。

まず少数サービス・少数Operationを実装し、内部抽象化が妥当であることを確認する。

---

# 23. Test Strategy

Fixture Server自身について以下をテストする。

## 23.1 Unit Test

```text
Request routing
Request normalization
Scenario loading
Scenario matching
Response generation
Sequence behavior
Error generation
Request history
```

---

## 23.2 AWS SDK Compatibility Test

実際のAWS SDKを使用してFixture Serverとの通信をテストする。

最低限:

```text
JavaScript / TypeScript
Python
```

追加候補:

```text
Go
Java
PHP
```

HTTP clientから直接Fixture Serverを呼び出すテストだけでは不十分とする。

最低限、

```text
AWS SDK
↓
Fixture Server
↓
AWS SDK deserialize
```

までをCompatibility Testとして確認する。

---

# 24. Example Integration Test Flow

```text
docker compose up
       ↓
Fixture Server startup
       ↓
Test Case start
       ↓
Scenario Fixture load
       ↓
Application integration test
       ↓
Application
       ↓
AWS SDK
       ↓
Fixture Server
       ↓
Scenario Match
       ↓
Fixture Response
       ↓
AWS SDK
       ↓
Application Assertion
       ↓
Request History Verification
       ↓
Fixture Server Reset
       ↓
Next Test Case
```

---

# 25. Initial Implementation Scope

最初の実装では機能を限定する。

## 25.1 Required Features

```text
Docker startup

Scenario YAML loading

Test-time Scenario switching

Fixture Control API

Request history

Unexpected request detection

Normal response

AWS error response

Sequence response
```

---

## 25.2 Required AWS Operations

### Secrets Manager

```text
GetSecretValue
```

### S3

```text
GetObject
PutObject
HeadObject
```

### SQS

```text
SendMessage
```

### Bedrock Runtime

```text
InvokeModel
Converse
```

ただしBedrock Streaming APIは対象外。

初期実装では `InvokeModel` と `Converse` の両方を実装する。

---

# 26. Implementation Order

## Step 1

HTTP Server、Scenario Loader、Fixture Control APIを実装する。

以下が成立すること。

```text
Fixture Server起動
↓
POST /__fixture/sessions/{sessionId}/scenario
↓
Scenario YAMLロード
```

---

## Step 2

Secrets Manager `GetSecretValue` を実装する。

最初の技術的成立条件:

```text
AWS SDK
↓
GetSecretValue
↓
Fixture Server
↓
Scenario Match
↓
AWS-compatible Response
↓
AWS SDK deserialize成功
```

---

## Step 3

AWS error responseを実装する。

例:

```text
ResourceNotFoundException
```

AWS SDK側で適切なException / Errorとして認識されることを確認する。

---

## Step 4

Request HistoryとUnexpected Request Detectionを実装する。

---

## Step 5

S3を実装する。

```text
GetObject
PutObject
HeadObject
```

REST/XML系protocol処理を確認する。

---

## Step 6

Sequence Responseを実装する。

---

## Step 7

SQS `SendMessage` を実装する。

---

## Step 8

Bedrock Runtimeの非Streaming APIを実装する。

```text
InvokeModel
Converse
```

---

# 27. Key Risks

## 27.1 AWS Protocol Complexity

AWSサービスごとにwire protocolが異なる。

対策:

```text
AWS Service Emulatorを作らない
Protocol Adapterを限定する
対応Operationを明示する
```

---

## 27.2 Fixture Complexity

テストケースごとに巨大なFixtureを持つと管理不能になる可能性がある。

対策:

```text
Scenario単位で管理
共通defaultはFixture Server側で保持
Scenarioには差分のみ定義
```

将来的にはScenario間のextends/include等も検討可能だが、初期実装で複雑な継承機構は必須としない。

---

## 27.3 AWS SDK Changes

AWS SDK version更新によってHTTP requestの非本質的部分が変化する可能性がある。

対策:

以下は原則matcher対象としない。

```text
Authorization
User-Agent
Date
SDK metadata
Content-Length
```

意味的なAWS API parameterをmatch対象とする。

---

## 27.4 Accidental Emulator Growth

S3内部保存、SQS queue state、IAM評価等を追加すると、AWS emulatorと同様の複雑性へ向かう。

対策:

以下を設計原則として維持する。

> No AWS service state simulation.

---

## 27.5 Over-generalized Fixture DSL

AWS protocolの全差異をYAMLだけで表現しようとすると、Fixture定義自体が複雑なプログラミング言語化する可能性がある。

対策:

```text
YAML
  = Scenario / Match / Expected Response

Fixture Server Code
  = AWS Protocol Handling
```

と責務を分離する。

---

# 28. Acceptance Criteria

初期実装完了条件:

1. DockerでFixture Serverを起動できる
2. テストコードからScenario Fixtureをロードできる
3. Scenario切替時にSequence/Historyを初期化できる
4. AWS SDK for JavaScriptからSecrets Managerを呼び出せる
5. `GetSecretValue` の正常レスポンスをScenario Fixtureで定義できる
6. `ResourceNotFoundException` をScenario Fixtureで定義できる
7. AWS SDK側で正常レスポンス・エラーともに正しくdeserializeされる
8. S3 `GetObject` をAWS SDKから実行できる
9. S3 `PutObject` requestをRequest Historyで確認できる
10. SQS `SendMessage` をAWS SDKから実行できる
11. Sequence Responseで一時エラー後の成功を表現できる
12. Bedrock Runtime `InvokeModel` または `Converse` をFixture化できる
13. Scenario Fixtureに存在しないAWS API呼び出しを検出できる
14. Fixture Serverコンテナを再起動せず、複数テストケースでScenarioを切り替えて利用できる

---

# 29. Design Summary

本システムはAWS Emulatorではない。

設計の中心は以下とする。

```text
Application
   ↓
AWS SDK
   ↓
AWS-compatible HTTP boundary
   ↓
Normalized Request
   ↓
Scenario Matcher
   ↓
Scenario Fixture
   ↓
AWS-compatible HTTP Response
   ↓
AWS SDK
```

AWSサービス内部の挙動を再現するのではなく、

```text
Request
   ↓
Fixture Match
   ↓
Configured Response
```

に限定する。

Scenario Fixtureはプロジェクト側で管理し、Fixture Serverコンテナはテストケース間で使い回す。

各テストケースはControl APIを使用して、

```text
Scenario Load
↓
Test Execution
↓
Request Verification
↓
Reset
```

を行う。

これにより、

* 軽量
* 高速
* 決定論的
* Dockerで簡単に利用可能
* 言語非依存
* AWS SDKそのものを含む結合テストが可能
* テストケースごとに自由にAWS境界条件を切り替え可能

なローカルAWS境界テスト基盤を実現する。

# 30. 言語

Go 言語とする。

初期実装の Go バージョンは、2026-09-07 時点の現行安定版である Go 1.27.1 とする。

HTTP Server は Go 標準ライブラリの `net/http` で実装する。Scenario Fixture と defaults の YAML 読み込みには `go.yaml.in/yaml/v3` を使用する。Web フレームワークは導入せず、対応する AWS wire protocol は本サーバーで限定実装する。

AWS response の request ID は、Go 標準ライブラリの `crypto/rand` で 16 byte を生成し、UUID v4 の version と variant bit を設定して `8-4-4-4-12` 形式へ整形する。UUID 用の外部依存は導入しない。

# 31. SDK 互換テスト実行環境

AWS SDK 互換テストは Docker で再現可能に実行する。Fixture Server のコンテナに加え、以下のテスト用コンテナを Docker Compose で起動する。

| 対象 | 実行環境 |
| --- | --- |
| JavaScript / TypeScript | Node.js 24.20.0 LTS |
| Python | Python 3.14.7 |

各テスト用コンテナは、Fixture Server コンテナに対して AWS SDK を用いた互換テストを実行する。

SDK 互換テストの依存関係は固定する。JavaScript / TypeScript は `package.json` と lockfile、Python は完全固定した `requirements.txt` で AWS SDK を含む依存関係を管理する。

JavaScript / TypeScript の SDK 互換テストには AWS SDK for JavaScript v3 の各 `@aws-sdk/client-*` パッケージを使用する。Python の SDK 互換テストには `boto3` を使用する。各バージョンは実装時点の現行安定版を lockfile または `requirements.txt` に固定する。

ルートの `docker-compose.yml` は Fixture Server、JavaScript / TypeScript SDK テスト、Python SDK テストの 3 サービスを定義する。

`make test` を初期実装の標準検証入口とする。Go の単体テストを実行した後、Docker Compose 上で JavaScript / TypeScript と Python の AWS SDK 互換テストを実行する。CI も同じコマンドを使用する。
