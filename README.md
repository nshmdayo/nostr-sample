# Nostr Sample Relay

Go言語で実装されたNostrリレーサーバーのサンプルです。

## 特徴

- WebSocketベースのNostrプロトコル実装
- 基本的なNIP（Nostr Implementation Possibilities）をサポート
- 署名検証
- イベントの保存と配信
- サブスクリプション管理
- リレー情報の提供

## サポートしているNIP

- NIP-01: Basic protocol flow description
- NIP-02: Contact List and Petnames
- NIP-09: Event Deletion
- NIP-11: Relay Information Document
- NIP-12: Generic Tag Queries
- NIP-15: End of Stored Events Notice
- NIP-16: Event Treatment
- NIP-20: Command Results
- NIP-22: Event `created_at` Limits

## 必要な環境

- Go 1.21以上

## インストールと実行

### 1. 依存関係のインストール

```bash
go mod download
```

### 2. サーバーの起動

```bash
go run main.go
```

サーバーは以下のエンドポイントで起動します：
- WebSocket: `ws://localhost:8080/ws`
- リレー情報: `http://localhost:8080/`

### 3. テストクライアントの実行

別のターミナルでテストクライアントを実行できます：

```bash
go run client/main.go
```

## API

### WebSocket エンドポイント

`ws://localhost:8080/ws`

サポートしているメッセージタイプ：

- `EVENT`: イベントの投稿
- `REQ`: イベントの購読
- `CLOSE`: 購読の終了

### リレー情報

`GET http://localhost:8080/`

- ブラウザ: HTML形式でリレー情報を表示
- `Accept: application/nostr+json`: JSON形式でリレー詳細情報を返却

## Dockerを使用した実行

### ビルド

```bash
docker build -t nostr-relay .
```

### 実行

```bash
docker run -p 8080:8080 nostr-relay
```

## 開発

### プロジェクト構造

```
.
├── main.go          # メインサーバー実装
├── client/
│   └── main.go      # テストクライアント
├── go.mod           # Go modules設定
├── go.sum           # 依存関係のハッシュ
├── Dockerfile       # Docker設定
└── README.md        # このファイル
```

### 主要コンポーネント

- `NostrServer`: メインサーバー構造体
- `Client`: WebSocket接続を管理
- `Subscription`: クライアントのサブスクリプションを管理
- イベントフィルタリングとブロードキャスト機能

## ライセンス

このプロジェクトはサンプル実装であり、学習目的で使用してください。