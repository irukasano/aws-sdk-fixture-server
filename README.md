# aws-sdk-fixture-server

ローカル開発と CI 向けの、決定的な AWS SDK 互換 fixture server です。AWS サービスをエミュレートするものではありません。テストごとに読み込む YAML Scenario に基づき、AWS SDK が deserialize できる HTTP response または error を返します。

アプリケーションと AWS SDK の境界をテストするためのものであり、AWS の内部状態や実サービスの挙動を再現しません。

## 対応範囲

| サービス | operation |
| --- | --- |
| Secrets Manager | `GetSecretValue` |
| S3 | `GetObject`, `PutObject`, `HeadObject` |
| SQS | `SendMessage` |
| Bedrock Runtime | `InvokeModel`, `Converse` |
| Cognito IDP | `AdminGetUser`, `AdminCreateUser`, `AdminUpdateUserAttributes`, `ListUsers`, `InitiateAuth`, `RespondToAuthChallenge` |
| SES v2 | `SendEmail` |

Cognito IDP では、各 session / user pool 固有の OIDC discovery document、JWKS、RS256 で署名した JWT を提供します。JWT は `InitiateAuth` と `RespondToAuthChallenge` の `AuthenticationResult.AccessToken` / `IdToken` でのみ利用できます。

この表にない operation と、ロード済み Scenario に一致しない request は、成功を推測せず AWS 形式の `500 UNEXPECTED_AWS_REQUEST` で fail-fast します。

## 非目標と制約

- 実際の AWS 接続、メール送信、IAM policy 評価、VPC や eventual consistency は扱いません。
- S3、SQS、Cognito などのサービス内部状態は保存しません。Scenario は状態遷移の再現ではなく、定義済みの response / error を返します。
- Cognito SRP、MFA、Hosted UI / Managed Login、OAuth 認可フローは対象外です。ユーザー、認証、OTP、RefreshToken の状態も保持しません。
- `FixtureSession` は process の `AWS_ENDPOINT_URL` と `AWS_ENDPOINT_URL_` で始まる環境変数を一時的に変更します。同じ process では同時に複数の `FixtureSession` を利用しないでください。
- helper の endpoint 設定を利用する SDK client には個別の endpoint を明示指定しないでください。個別 endpoint は SDK の環境変数解決より優先され、session 固有 endpoint へ向かなくなります。

## 必要環境と起動

Docker と Docker Compose が必要です。Compose のテスト用コンテナは次のダミー credential と region を使います。

```text
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
AWS_REGION=us-east-1                # Python は AWS_DEFAULT_REGION
AWS_ENDPOINT_URL=http://aws-fixture:4566
```

fixture server は既定で host の `4566` 番ポートを公開します。稼働確認は次の endpoint で行えます。

```sh
curl -i http://localhost:4566/__fixture/health
```

ローカルの fixture server だけを起動する場合:

```sh
docker compose up --build aws-fixture
```

全 Compose service（fixture server と JavaScript / Python SDK integration test）を起動する場合:

```sh
docker compose up --build
```

検証コマンドは以下です。

```sh
make go-test   # Go の HTTP / protocol / OIDC-JWT 境界
make sdk-test  # Docker Compose 上の JavaScript と Python AWS SDK 境界
make test      # 上記の両方
```

`make sdk-test` は `aws-fixture` を build・起動してから SDK テストを実行します。テスト用 Scenario は `tests/aws/scenarios/` からコンテナ内の `/scenarios` に read-only で mount されます。

## 最短の利用例

まず server URL を設定し、Scenario を用意します。Compose を使う application container からは `http://aws-fixture:4566`、host からは `http://localhost:4566` を使います。

```sh
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_REGION=us-east-1
export AWS_ENDPOINT_URL=http://localhost:4566
```

`tests/aws/scenarios/sdk-core.yml` は Secrets Manager、S3、SQS、Bedrock Runtime の例を含みます。次のように project-owned Scenario を作成することもできます。

```yaml
# tests/aws/scenarios/secret.yml
name: secret
responses:
  - service: secretsmanager
    operation: GetSecretValue
    match: { SecretId: test/db }
    response:
      Name: test/db
      SecretString: '{"host":"db"}'
```

### JavaScript / TypeScript

`FixtureSession.start` の前に `AWS_ENDPOINT_URL` を fixture server URL に設定します。start 後は helper が process の endpoint を session 固有 URL に置き換えるため、application の SDK client は endpoint を指定せず通常どおり生成します。

```js
import { GetSecretValueCommand, SecretsManagerClient } from "@aws-sdk/client-secrets-manager";
import { FixtureSession } from "./fixture-session/index.mjs";

const serverUrl = process.env.AWS_ENDPOINT_URL;
const fixture = await FixtureSession.start({ serverUrl });
try {
  await fixture.loadScenario("/scenarios/secret.yml");

  const client = new SecretsManagerClient({ region: process.env.AWS_REGION });
  const result = await client.send(new GetSecretValueCommand({ SecretId: "test/db" }));
  console.log(result.SecretString); // {"host":"db"}

  console.log(await fixture.requests());
} finally {
  await fixture.destroy();
}
```

`packages/sdk/javascript/` の `FixtureSession` を application が参照できる場所へ配置します。`destroy()` は server 上の session を削除し、開始前の endpoint 環境変数を復元します。

### Python

Python でも SDK client に `endpoint_url` を渡しません。

```python
import os

import boto3
from fixture_session import FixtureSession

fixture = FixtureSession.start(server_url=os.environ["AWS_ENDPOINT_URL"])
try:
    fixture.load_scenario("/scenarios/secret.yml")

    client = boto3.client("secretsmanager")
    result = client.get_secret_value(SecretId="test/db")
    print(result["SecretString"])

    print(fixture.requests())
finally:
    fixture.destroy()
```

`packages/sdk/python/fixture_session/` を import path に置きます。`FixtureSession` は JavaScript 版と同じく endpoint 環境変数を session 用に設定・復元します。

## Scenario

Scenario は project が所有する YAML ファイルです。Control API では `/scenarios/<name>.yml` または `/scenarios/<name>.yaml` のみを読み込めます。

| フィールド | 意味 |
| --- | --- |
| `service` / `operation` | 対象の AWS service と operation |
| `match` | request parameter の一致条件。文字列の完全一致、`"*"`、`contains`、`regex` を使えます。省略時は全 request に一致します。 |
| `response` | 成功時の AWS SDK output shape |
| `error` | error response。`type`、`message`、必要に応じて `status` を指定します。 |
| `sequence` | 呼出順に選ぶ `response` / `error` の配列。末尾の結果は以後も繰り返します。 |
| `use_defaults` | `true` のとき、Scenario 自身にその `service + operation` がない request だけ server 同梱 default Scenario へ fallback します。 |

成功、エラー、sequence の最小例です。

```yaml
name: example
use_defaults: true
responses:
  - service: secretsmanager
    operation: GetSecretValue
    match: { SecretId: test/db }
    response: { Name: test/db, SecretString: '{"host":"db"}' }

  - service: sesv2
    operation: SendEmail
    match: { FromEmailAddress: sender@example.test }
    response: { MessageId: fixture-message }

  - service: sqs
    operation: SendMessage
    match: { QueueUrl: https://sqs.us-east-1.amazonaws.com/123/error }
    error: { type: ThrottlingException, message: retry later, status: 429 }

  - service: sqs
    operation: SendMessage
    match: { QueueUrl: https://sqs.us-east-1.amazonaws.com/123/sequence, MessageBody: { contains: hello } }
    sequence:
      - error: { type: ThrottlingException, message: retry once, status: 429 }
      - response: { MessageId: message-1 }
```

Scenario をロードしていない session は、`defaults/scenario/` に同梱する basic success response を利用します。Scenario をロード後は `use_defaults: true` を明示した場合だけ、未定義 operation に default Scenario を使います。同じ `service + operation` を Scenario に定義した場合、その `match` 不一致は default へ fallback せず fail-fast します。

## Control API

`{server}` は `http://localhost:4566`、`{sessionId}` は session 作成時の response に含まれる値です。

| HTTP method | endpoint | 用途 |
| --- | --- | --- |
| `GET` | `{server}/__fixture/health` | healthcheck。`200` を返します。 |
| `POST` | `{server}/__fixture/sessions` | session を作成。`sessionId`、AWS SDK 用 `endpoint`、Cognito 用 `issuer` を返します。 |
| `POST` | `{server}/__fixture/sessions/{sessionId}/scenario` | `{ "path": "/scenarios/example.yml" }` を渡して Scenario をロードします。sequence と request history も reset されます。 |
| `POST` | `{server}/__fixture/sessions/{sessionId}/reset` | sequence counter と request history を reset します。Scenario は維持されます。 |
| `GET` | `{server}/__fixture/sessions/{sessionId}/requests` | service、operation、parameter、HTTP method / path、結果種別などの request history を返します。 |
| `DELETE` | `{server}/__fixture/sessions/{sessionId}` | session、Scenario、sequence、request history を破棄します。 |

AWS SDK request は session 作成 response の `endpoint`、すなわち `{server}/__fixture/sessions/{sessionId}/aws` に送られます。通常は helper がこれを設定するため、直接組み立てる必要はありません。

## Cognito JWT / JWKS

session 作成 response と Scenario load response の `issuer` は、次の session 固有 URL です。

```text
http://{host}/__fixture/sessions/{sessionId}/oidc/{userPoolId}
```

OIDC discovery と JWKS は以下で取得します。

```text
GET {issuer}/.well-known/openid-configuration
GET {issuer}/.well-known/jwks.json
```

Scenario の `InitiateAuth` または `RespondToAuthChallenge` で JWT を返すには、`AuthenticationResult` に template を書きます。

```yaml
name: cognito-auth
responses:
  - service: cognito-idp
    operation: InitiateAuth
    response:
      AuthenticationResult:
        AccessToken:
          jwt:
            expires_in_seconds: 3600
            claims: { sub: fixture-user, email: fixture@example.test }
        IdToken:
          jwt:
            expires_in_seconds: 3600
            claims: { sub: fixture-user, email: fixture@example.test }
        TokenType: Bearer
        ExpiresIn: 3600
```

`claims.sub` と `expires_in_seconds` は必須です。server は現在時刻から `iat` / `exp`、session 固有 `iss`、`token_use` を設定して RS256 署名します。AccessToken には request の `ClientId` を `client_id` として、IdToken には同じ値を `aud` として設定します。`iss`、`iat`、`exp`、`token_use`、`client_id`、`aud` は予約済みのため Scenario に指定できません。負の `expires_in_seconds` は期限切れ JWT の fixture に使えます。

JWT と JWKS は application 側で issuer / JWKS URL を設定して検証してください。Fixture Server は、Scenario が明示する JWKS と署名鍵の整合性を検証しません。
