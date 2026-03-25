# Autotech Writer

本システムは、Go言語によるデータ収集と、PythonによるLLMを用いたデータ処理・判定処理を連携させたパイプラインシステムです。
外部API（X APIなど）からデータを取得・整形してPostgreSQLに保存し、FastAPIベースのPythonバックエンドでLLMを活用したテキスト解析や評価を行います。

## リポジトリ構成とファイルの説明

ディレクトリ直下の主なファイル・ディレクトリは以下の役割を持っています。

- **`go-collector/`**
  - 外部のAPIからデータを収集し、PostgresSQLに保存するGoコンポーネントです。保存時に **PostgreSQL LISTEN/NOTIFY** を通じて Python 側へ即時通知を送ります。
- **`python-llm/`**
  - FastAPIを用いたバックエンドです。DB通知を受けてLLMパイプラインを起動し、抽出・生成を行います。**LINE Messaging API** による有人承認フローや、**指数バックオフリトライ** 機能を搭載しています。
- **`db/`**
  - PostgreSQLの初期化およびマイグレーション（`db/migrations/`）を管理します。
- **`testdata/`**
  - GoやPythonの各種自動テスト（TDDなどで利用）に用いるモックデータやシードデータが格納されています。
- **`docker-compose.yml`**
  - 本システム（`go-collector`, `python-llm`, `db`）をローカル環境で立ち上げるためのDocker Compose構成ファイルです。
- **`docker-compose.test.yml`**
  - テスト環境を立ち上げるためのDocker Compose構成ファイルです。テスト用のDBコンテナ等の起動に用いられます。
- **`.env.example`**
  - 本システムで必要となる環境変数（データベース接続情報や各種APIキー設定など）のテンプレートファイルです。ローカル構築時にはこれをコピーして`.env`を作成します。
- **`doc/`**
  - システム構成やデータフロー図などの設計ドキュメントが格納されています。詳細は [data_flow.md](doc/data_flow.md) を参照してください。

## 起動方法 (ローカル環境)

1. `.env.example` をコピーして `.env` を作成し、必要な環境変数（APIキー等）を設定します。
2. Docker Composeを利用してコンテナを起動します。

```bash
docker compose up --build
```
