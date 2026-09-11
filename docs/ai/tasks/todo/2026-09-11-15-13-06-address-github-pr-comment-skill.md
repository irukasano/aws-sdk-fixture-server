## HLD

### 2026-09-11 15:13 : address-github-pr-comment スキル作成

- 目的: PR 番号または Issue 番号を入力として、現在の `gh` ログインユーザーが未返信の行コメントを抽出し、コメント単位で妥当性を判定して、必要な修正・検証・コミット・返信・push までを安全に実行する `address-github-pr-comment` スキルを作成する。
- 変更対象: `.codex/skills/address-github-pr-comment/SKILL.md`、番号から対象 PR を解決する `.codex/bin/github-resolve-pr-from-number.sh`、未返信行コメントを抽出する `.codex/bin/github-list-unreplied-review-comments.sh`、レビュー thread へ返信する `.codex/bin/github-reply-to-review-thread.sh`、スキルに不可欠な補助リソース。
- 非変更対象: Issue #1 / PR #2 のコメントへの実対応、既存アプリケーションコード、レビュー会話の resolve 操作。
- 入出力:
  - スキルの入力は番号 1 個。`.codex/bin/github-resolve-pr-from-number.sh <番号>` は、まず `gh pr view` で照会し、成功すれば PR として直接対象にする。失敗した場合のみ `gh issue view` で Issue として照会し、`closedByPullRequestsReferences` で関連する open PR を探索する。解決した PR の番号・URL・タイトルを JSON で標準出力する。両方失敗、または Issue の PR 候補が 0 件もしくは複数件なら、変更・返信・push をせず停止して報告する。認証・ネットワークなどの取得障害では種別のフォールバックを行わず停止する。
  - スクリプトの入力は PR 番号 1 個。未返信スレッドを JSON Lines（1 スレッド 1 JSON オブジェクト）で標準出力する。各オブジェクトには、返信に必要な thread ID、PR 番号、ファイル・行情報、diff hunk、スレッド内コメント、最後のコメントの投稿者・本文・URL を含める。
  - `github-reply-to-review-thread.sh` の入力は review thread ID と返信本文。既存 thread への返信だけを `addPullRequestReviewThreadReply` mutation で投稿し、成功時に返信 URL を標準出力する。PR 本文や top-level comment の投稿・編集はサポートしない。
  - 正常終了時は 0。入力不正または対象 PR 不在、`gh` の認証・API・取得失敗時は非 0 で標準エラーへ原因を出力する。
- 運用方法:
  - 現在のリポジトリと `gh` の現在アカウントを使用し、`gh api user` で取得した login を `target_login` とする。
  - スクリプトは、未 resolve、起点が行コメントであり、最後のコメントの投稿者が `target_login` ではない review thread のみを返す。宛先・メンションの有無は判定しない。対象コメントが 0 件なら、スキルはその旨を報告して終了する。
  - スキルは各出力を順番に処理する。指摘が不当なら根拠を返信する。妥当だが対応不要なら根拠を返信する。妥当かつ対応が必要なら、コメント単位の subtask として todo への実装計画記録、red テスト作成、red テストの妥当性レビュー、実装、green 確認、実装レビュー、コミット、返信を順に行う。
  - 対応後も thread を resolve しない。全対象を完了して初めて push する。GitHub への返信と push はそれぞれ必要な作業完了後にのみ行う。
- 失敗時挙動: 妥当性判断に迷う場合、仕様変更またはスコープ確認が必要な場合、red テストまたは実装レビューで問題がある場合は、そのコメントの処理を停止してユーザーに確認する。対象 PR 解決、`gh` 認証・API 呼び出し、検証、コミット、返信、push の失敗時も以降の外部変更を行わず、失敗箇所と原因を報告する。
- 既存機能への影響: 新規スキルと補助スクリプト 3 本の追加のみを想定する。既存の Issue #1 / PR #2 およびアプリケーションコードにはこのタスクでは変更を加えない。
- 未確定事項: なし。
- ユーザー確認が必要な項目: 更新済み Plan の承認。
- 合意: スキルはリポジトリ内の `.codex/skills/address-github-pr-comment/` に配置する。番号からの PR 解決は `.codex/bin/github-resolve-pr-from-number.sh <番号>` に切り出し、入力が PR なら `gh pr view`、それ以外は `gh issue view --json closedByPullRequestsReferences` で関連 open PR を解決する。Issue に複数の open PR が関連付く場合は処理を停止し、候補を示してユーザーに選択を確認する。Issue 番号では関連付いた open PR だけを候補にする。GitHub 上で resolve 済みの行コメントスレッドは未返信候補から除外する。未返信抽出スクリプトは `github-list-unreplied-review-comments.sh <PR番号>` とし、未返信スレッドを JSON Lines で標準出力する。返信は `github-reply-to-review-thread.sh <thread-id> <本文>` により既存 review thread だけへ投稿し、PR 本文・top-level comment はサポートしない。

## Plan

### 2026-09-11 15:13 : address-github-pr-comment スキル作成

- [x] `skill-creator` の規約に従い、`address-github-pr-comment` を `.codex/skills/` に初期化する。`agents/openai.yaml` は通常の暗黙起動を維持し、スキル名を含む既定プロンプトだけを設定する。
- [x] `.codex/bin/github-resolve-pr-from-number.sh` を作成する。入力を正の番号 1 個に制限し、`gh pr view` が成功すればその PR の番号・タイトル・URL を JSON で返す。PR 不在時だけ `gh issue view --json closedByPullRequestsReferences` を呼び、open PR 候補が 1 件の場合だけ同じ JSON を返す。候補 0 件・複数件、認証・API・入力エラーは標準エラーと非 0 で終了する。
- [x] `.codex/bin/github-list-unreplied-review-comments.sh` を作成する。引数を厳密に PR 番号 1 個に制限し、`gh auth status` と `gh api user` で認証と `target_login` を確認する。
- [x] `.codex/bin/github-reply-to-review-thread.sh` を作成する。thread ID と空でない本文 1 個を受け、`addPullRequestReviewThreadReply` mutation でその既存 thread にだけ返信する。成功時は返信 URL を標準出力し、入力・認証・API エラーは標準エラーと非 0 で終了する。PR 本文・top-level comment を扱う API は使用しない。
- [x] スクリプトが GraphQL で対象 PR の review thread をページング取得し、各 thread のコメントを全件取得する。未 resolve、起点コメントに `originalLine` があり、時系列上最後の投稿者が `target_login` でない thread だけを JSON Lines で返す。一般コメント、resolve 済み、最終投稿者が `target_login` の thread を除外する。認証・API・入力の失敗は標準エラーと非 0 で終了する。
- [x] `docs/ai/tasks/workspace/` に置く一時検証プログラムで `gh` をモックし、番号からの PR 解決、対象抽出、除外条件、review thread 返信 mutation、JSON / JSON Lines の必須フィールド、入力・API エラーの終了コードを確認した。検証プログラムは削除した。
- [x] `SKILL.md` に、`.codex/bin/github-resolve-pr-from-number.sh` による番号からの解決手順と、`.codex/bin/github-reply-to-review-thread.sh` による既存 thread への返信手順を記載した。Issue の open PR 候補が 0 件または複数件の場合の停止、未返信抽出スクリプト実行、0 件時の終了、コメント単位の判定と返信、resolve 禁止、全完了後の push を明記した。
- [x] `SKILL.md` に、妥当かつ対応が必要なコメントでのみ、todo への計画記録、red テスト、別担当による red テストレビュー、実装、green 検証、実装レビュー、コミット、返信をこの順で subtask 化することを明記する。妥当性不明、仕様変更・スコープ確認が必要、検証・レビュー・commit・返信・push 失敗時は停止して報告する。
- [x] `skill-creator` の `quick_validate.py` でスキル構造・frontmatter・未完了 scaffold を検証し、抽出スクリプトのモック検証を実行した。
- [x] 独立したエージェントに、現実的な PR レビュー対応依頼を与え、作成したスキルの指示が HLD の対象抽出・停止・resolve 禁止・返信・push 順序を満たすか確認させた。実運用の GitHub 変更は行わせなかった。
- [x] `git diff --check` と作業ツリー差分を確認し、結果を Review に記録した。`main` / `origin/main` ref がないため `git diff main...HEAD` は比較不能だった。コミットと push は本タスクでは行わない。

## Review

### 2026-09-11 15:46 : address-github-pr-comment スキル作成

- 実装: スキル定義と、PR 解決・未返信行コメント抽出・既存 review thread 専用返信の補助スクリプト 3 本を追加した。返信は `addPullRequestReviewThreadReply` のみを使用し、resolve、PR 本文、top-level comment は扱わない。
- 原因と修正: `closedByPullRequestsReferences` の nested 出力に `state` がないため、初期実装の `state == "OPEN"` 条件が Issue #1 の PR #2 を除外した。条件を削除し、候補確定後に `gh pr view` で title を補完した。独立レビューで返信手順の欠落と空 URL の成功扱いを検出し、thread 専用返信スクリプトと空 URL 拒否を追加した。
- 検証: `gh` モックで PR 直接解決、Issue 経由解決、候補複数、API・入力失敗、未解決行コメントの抽出・除外、返信成功・失敗・空 URL 拒否を確認した。実データで Issue #1 と PR #2 の双方が PR #2 に解決され、PR #2 の未返信行コメント 3 件を抽出できた。`bash -n`、`quick_validate.py`、`git diff --check` は成功した。独立レビューの指摘をすべて反映して再確認した。
- 差分: 今回の 6 ファイルだけが未追跡。ローカルと origin に `main` ref がないため `main...HEAD` 比較は実行不能だった。コミット、push、GitHub への返信、resolve は行っていない。
