# **実装指示書: gh-secret-sync**

## **1\. 概要 (Overview)**

本ドキュメントは、GitHubの個人/組織リポジトリ群に対して、SOPSで暗号化された設定ファイルからSecretおよびEnvironmentを一括同期・適用・編集するためのCLIツール gh-secret-sync の実装指示書です。

## **2\. システム要件 & 依存関係 (Dependencies)**

* **言語 / ランタイム:** Go 1.21 以上  
* **必須ライブラリ:**  
  * go.mozilla.org/sops/v3 (SOPS復号ライブラリ)  
  * github.com/google/go-github/v60/github (GitHub API SDK)  
  * golang.org/x/oauth2 (認証)  
  * golang.org/x/crypto/nacl/box (GitHub Secret暗号化用 libsodium)  
  * gopkg.in/yaml.v3 (YAMLパース)  
* **外部依存:**  
  * edit サブコマンドでの再暗号化処理用に CLI ツール sops がシステム上（パスが通った状態）で実行可能であること。

## **3\. ディレクトリ構造 (Directory Structure)**

gh-secret-sync/  
├── go.mod  
├── go.sum  
├── main.go               \# サブコマンドの判定およびCLIエントリポイント  
└── pkg/  
    ├── config/  
    │   └── config.go     \# 設定ファイルのデータ構造定義、SOPS復号、一時ファイルを用いた編集ロジック  
    └── github/  
        └── client.go     \# GitHub API操作、NaCl BoxによるSecret暗号化

## **4\. 設定ファイル構造 (Config File Schema)**

SOPSで暗号化・復号されるYAMLのフォーマット定義です（短いキー名を標準とし、後方互換キー名もサポート）。

# 暗号化対象のデータ構造例
repos:
  - owner/repo-a
  - owner/repo-b

secrets:
  GLOBAL_API_KEY: "secret-value-12345"

vars:
  LOG_LEVEL: "info"

envs:
  - name: production
    secrets:
      DATABASE_URL: "postgres://prod-db:5432/db"
    vars:
      API_ENDPOINT: "https://api.example.com"
  - name: staging
    secrets:
      DATABASE_URL: "postgres://staging-db:5432/db"
    vars:
      API_ENDPOINT: "https://staging-api.example.com"

## **5\. 機能要件および各モジュール設計**

### 5.1 CLI インターフェース (main.go)

* **サブコマンド:**  
  * init: SOPS用のAge鍵生成および .sops.yaml 初期設定ファイルの作成。
  * apply: 設定を復号し、GitHub API経由で各リポジトリに同期（Secrets および Variables）。  
  * edit: 設定ファイルを安全に復号・編集・再暗号化（存在しないファイルの場合はサンプルテンプレートを提示）。  
* **フラグオプション:**  
  * -o <path>: Age秘密鍵出力パス（init用、デフォルト: key.txt）
  * -sops-config <path>: .sops.yaml設定ファイルパス（init用、デフォルト: .sops.yaml）
  * -c <path>: 設定ファイルのパス指定（apply/edit用、デフォルト: config.sops.yaml）  
* **環境変数:**  
  * GITHUB_TOKEN: GitHub API操作に必要なパーソナルアクセストークン（PAT）。

### **5.2 設定ファイル・SOPS操作モジュール (pkg/config/config.go)**

* LoadSopsConfig(filePath string) (*Config, error)  
  * SOPSライブラリ（decrypt.File）を用いてメモリ上で直接復号。  
  * ディスクに一時ファイルを残さず yaml.Unmarshal を実行（キー名の簡略化 `repos`, `secrets`, `vars`, `envs` および旧形式との後方互換に対応）。  
* EditSopsFile(filePath string) error  
  * 復号結果をパーミッション 0600 の一時ファイルに書き出し。  
  * defer os.Remove(tmpPath) を使用し、異常終了時も一時ファイルを確実に消去。  
  * $EDITOR（未設定時は OS に応じて vi または notepad）を起動。  
  * エディタ終了後、sops --encrypt --in-place <tmpPath> を呼び出して再暗号化。  
  * 再暗号化された内容で指定された filePath を上書き。

### **5.3 GitHub API 操作モジュール (pkg/github/client.go)**

* **Secret の事前暗号化 (encryptSecret)**  
  * GitHub Actions Secret APIの要求に従い、指定の Public Key を基に golang.org/x/crypto/nacl/box (NaCl Anonymous Sealed Box) で平文を暗号化。  
* **SyncRepositorySecret(owner, repo, secretName, secretValue string) error**  
  1. GetRepoPublicKey でリポジトリ用 Public Key を取得。  
  2. Secret 値を暗号化。  
  3. CreateOrUpdateRepoSecret で適用。  
* **SyncRepositoryVariable(owner, repo, varName, varValue string) error**  
  1. CreateRepoVariable を試行し、エラーが発生した場合は UpdateRepoVariable で適用。  
* **SyncEnvironmentSecret(owner, repo, envName, secretName, secretValue string) error**  
  1. CreateUpdateEnvironment で Environment を確実に作成/更新。  
  2. Repositories.Get で Repository ID を取得。  
  3. GetEnvPublicKey で Environment 用 Public Key を取得。  
  4. Secret 値を暗号化。  
  5. CreateOrUpdateEnvSecret で適用。  
* **SyncEnvironmentVariable(owner, repo, envName, varName, varValue string) error**  
  1. CreateUpdateEnvironment で Environment を確実に作成/更新。  
  2. Repositories.Get で Repository ID を取得。  
  3. CreateEnvVariable を試行し、エラーが発生した場合は UpdateEnvVariable で適用。

## **6\. セキュリティ・品質要件 (Security Requirements)**

1. **メモリ管理とファイル安全策**  
   * apply 実行時は平文データを一切ストレージに書き出さないこと。  
   * edit 実行時の一時ファイルはパーミッション 0600 に固定し、処理終了時に必ず defer で削除すること。  
2. **トークン権限のチェック**  
   * 利用する GITHUB_TOKEN には以下の権限が必要であることをドキュメント化・メッセージ表示すること。  
     * Repository permissions: Secrets (Read & Write), Variables (Read & Write), Environments (Read & Write)

## **7\. テスト・検証手順 (Verification Steps)**

1. **依存関係の解決:**  
   go mod tidy

2. **設定ファイルの準備と編集テスト:**  
   go run main.go edit -c test.sops.yaml

3. **同期適用テスト:**  
   export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxx"  
   go run main.go apply -c test.sops.yaml

4. **GitHub 上の確認:**  
   * 指定したリポジトリの Settings > Secrets and variables > Actions (Secrets & Variables タブ) および Settings > Environments に値が正しく反映されているか確認する。

## **8\. 関連ドキュメント (Related Documents)**

* [ユーザーガイド & 設定・暗号化マニュアル (docs/usage.md)](file:///home/naou/Workspace/gh-sync-secret/docs/usage.md): コンフィグファイルの具体例、Age/SOPS パスワード暗号化手順、および CLI の使い方。