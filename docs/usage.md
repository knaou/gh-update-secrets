# gh-secret-sync ユーザーガイド & 設定・暗号化マニュアル

本ドキュメントでは、`gh-secret-sync` のコンフィグファイルの書き方、内包する SOPS / Age 暗号化機能 (`init`, `edit`) を用いた暗号化設定ファイルの作成・管理手順、および GitHub への一括同期 (`apply`) の使い方を解説します。

`gh-secret-sync` は SOPS 暗号化機能を内包しているため、手動で `sops` や `age-keygen` コマンドを直接実行することなく、すべて `gh-secret-sync` のサブコマンドを通じて暗号化・編集・適用を行うことができます。

---

## 1. コンフィグファイルの構成と具体例

`gh-secret-sync` では、同期対象の GitHub リポジトリと、適用する Repository Secrets / Variables および Environment Secrets / Variables を YAML 形式で定義します。
キー名は短く直感的な表記 (`repos`, `secrets`, `vars`, `envs`) をメインとして採用しています（旧キー名との後方互換性も保持されています）。

`docs/` ディレクトリ配下に以下のサンプル YAML ファイルを用意しています:
- [config.example.yaml](file:///home/naou/Workspace/gh-sync-secret/docs/config.example.yaml): 平文の設定ファイルサンプル
- [config.sops.example.yaml](file:///home/naou/Workspace/gh-sync-secret/docs/config.sops.example.yaml): SOPS 暗号化済みの設定ファイルサンプル
- [sops.example.yaml](file:///home/naou/Workspace/gh-sync-secret/docs/sops.example.yaml): Age/SOPS 暗号化ルール設定 (`.sops.yaml`) サンプル

### コンフィグファイル例 (`config.sops.yaml`)

```yaml
# 1. 同期対象のリポジトリ一覧 (owner/repo 形式)
repos:
  - my-org/service-a
  - my-org/service-b

# 2. 全対象リポジトリに共通して同期される Repository Secrets
secrets:
  GLOBAL_API_KEY: "sk_live_example_key_123456789"
  DATADOG_API_KEY: "dd_api_key_abcdef123456"

# 3. 全対象リポジトリに共通して同期される Repository Variables (非暗号の環境変数)
vars:
  LOG_LEVEL: "info"
  APP_ENV: "production"

# 4. リポジトリごとの Environment に同期される Environment Secrets および Variables
# (対象の Environment が GitHub 上に存在しない場合は自動作成されます)
envs:
  - name: production
    secrets:
      DATABASE_URL: "postgres://prod_user:secure_prod_pass@prod-db.internal:5432/prod_db"
      REDIS_URL: "redis://prod-redis.internal:6379"
    vars:
      API_ENDPOINT: "https://api.example.com"

  - name: staging
    secrets:
      DATABASE_URL: "postgres://stage_user:secure_stage_pass@stage-db.internal:5432/stage_db"
      REDIS_URL: "redis://stage-redis.internal:6379"
    vars:
      API_ENDPOINT: "https://staging-api.example.com"
```

---

## 2. 暗号化設定ファイルの作成手順 (`gh-secret-sync` による操作)

`gh-secret-sync` は **SOPS (Secrets OPerationS)** および **Age** による暗号化に対応しています。
外部の `sops` / `age-keygen` コマンドを直接入力することなく、`gh-secret-sync` のサブコマンド（`init` や `edit`）を利用して一連のセットアップを行えます。

### Step 1. SOPS / Age 鍵と設定の初期化 (`init`)

最初に `gh-secret-sync init` を実行し、暗号化に使用する Age 秘密鍵 (`key.txt`) と、公開鍵を組み込んだ `.sops.yaml` を生成します。

```bash
# デフォルト (key.txt および .sops.yaml) で初期化
gh-secret-sync init
```

> **出力例:**
> ```text
> Successfully generated SOPS Age key!
>   Public key:  age1ql3wvd0x9ctfcganw9f3zgjfc8eswwx9q76r8wre24m3nvwvuemsvespdu
>   Key file:    key.txt (chmod 0600)
>   SOPS config: .sops.yaml
> 
> To use this key with gh-secret-sync and SOPS, set:
>   export SOPS_AGE_KEY_FILE=key.txt
> ```

鍵生成後、環境変数 `SOPS_AGE_KEY_FILE` に鍵ファイルのパスを設定してください:

```bash
export SOPS_AGE_KEY_FILE="$(pwd)/key.txt"
```

### Step 2. 暗号化設定ファイルの作成・編集 (`edit`)

`gh-secret-sync edit` コマンドを使用して、暗号化設定ファイル (`config.sops.yaml`) を作成・編集します。

```bash
# 暗号化設定ファイルの作成・編集
gh-secret-sync edit -c config.sops.yaml
```

**動作フロー:**
1. 指定されたファイルが存在しない場合は、サンプルテンプレートが自動で用意されます。
2. 復号された平文がパーミッション `0600` の一時ファイルに書き出されます。
3. `$EDITOR`（未設定時は `vi` または `notepad`）が起動し、安全に編集できます。
4. 保存してエディタを閉じると、自動的に SOPS により再暗号化されて保存されます。
5. 編集終了後、一時ファイルは確実に自動削除されます。

---

## 3. `gh-secret-sync` の使い方

### 前提条件・環境変数

実行に必要な環境変数を設定します。

1. **`GITHUB_TOKEN` の設定**
   GitHub API を操作するために Personal Access Token (PAT) を設定します。
   
   **必要な権限 (Scopes):**
   - Repository permissions:
     - **Secrets**: Read & Write
     - **Variables**: Read & Write
     - **Environments**: Read & Write

   ```bash
   export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
   ```

2. **復号鍵（`SOPS_AGE_KEY_FILE` または `SOPS_AGE_PASSPHRASE`）の設定**

   - 鍵ファイルを使用する場合:
     ```bash
     export SOPS_AGE_KEY_FILE="$(pwd)/key.txt"
     ```
   - パスフレーズを使用する場合:
     ```bash
     export SOPS_AGE_PASSPHRASE="your-secure-passphrase"
     ```

3. **外部ツール依存に関する注意**
   - `edit` サブコマンドでの再暗号化処理のために、システムに CLI ツール `sops` がインストールされ、実行可能（パスが通った状態）である必要があります。

---

### サブコマンド解説と実行例

#### 1. `init` サブコマンド
Age 鍵の生成および `.sops.yaml` 設定ファイルの初期化を行います。

```bash
# デフォルト設定で初期化 (key.txt, .sops.yaml)
gh-secret-sync init

# カスタムパスを指定して初期化
gh-secret-sync init -o mykey.txt -sops-config .sops.yaml
```

#### 2. `edit` サブコマンド
設定ファイルを安全に復号してエディタで編集し、自動で再暗号化して保存します。

```bash
# デフォルトファイル (config.sops.yaml) を編集
gh-secret-sync edit

# 指定したファイルを編集
gh-secret-sync edit -c my-config.sops.yaml
```

#### 3. `apply` サブコマンド
暗号化された設定ファイルをメモリ上で復号し、対象となる全 GitHub リポジトリへ Secrets および Variables を一括同期します。

```bash
# デフォルト設定ファイル (config.sops.yaml) を適用
gh-secret-sync apply

# カスタムパスの設定ファイルを指定して適用
gh-secret-sync apply -c my-config.sops.yaml
```

**実行出力例:**
```text
Starting resource synchronization from config.sops.yaml...

Note: Ensure your GITHUB_TOKEN has the following repository permissions:
  - Secrets: Read & Write
  - Variables: Read & Write
  - Environments: Read & Write

Processing repository: my-org/service-a
  -> Syncing repo secret: GLOBAL_API_KEY
  -> Syncing repo secret: DATADOG_API_KEY
  -> Syncing repo variable: LOG_LEVEL
  -> Syncing repo variable: APP_ENV
  -> Processing environment: production
     -> Syncing env secret: DATABASE_URL
     -> Syncing env secret: REDIS_URL
     -> Syncing env variable: API_ENDPOINT
  -> Processing environment: staging
     -> Syncing env secret: DATABASE_URL
     -> Syncing env secret: REDIS_URL
     -> Syncing env variable: API_ENDPOINT

Processing repository: my-org/service-b
  ...
Successfully synchronized all secrets and variables.
```

---

## 4. セキュリティに関する推奨事項

1. **`config.sops.yaml` のバージョン管理**:
   暗号化済みの `.sops.yaml` や `config.sops.yaml` は Git 管理に含めて問題ありません。

2. **秘密鍵・平文ファイルの管理**:
   鍵ファイル (`key.txt`) や一時平文ファイルは絶対に変更・コミットされないよう `.gitignore` に追加してください。

   ```gitignore
   # .gitignore 例
   key.txt
   config.yaml
   *.tmp
   ```
