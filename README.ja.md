# go-backup-cleaner

[English](README.md) | 日本語

[![Test](https://github.com/ideamans/go-backup-cleaner/actions/workflows/test.yml/badge.svg)](https://github.com/ideamans/go-backup-cleaner/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ideamans/go-backup-cleaner.svg)](https://pkg.go.dev/github.com/ideamans/go-backup-cleaner)

大量のファイルコレクションから古いバックアップファイルを効率的に削除することで、ディスクの空き容量を維持するために設計されたGoパッケージです。ストレージ容量の限界に達した際に、最も古いバックアップファイルを賢く削除することで、指定された空きディスク容量を常に確保します。

## 機能

- **効率的なディスク容量管理** - 古いバックアップを削除して指定された空き容量を維持
- **大規模ファイルコレクションに最適化** - メモリ効率的なアルゴリズムで数百万ファイルを処理
- **並列処理** - 最大パフォーマンスのための並行ファイルスキャンと削除
- **スマートな削除** - 最新のバックアップを保持するため最も古いファイルから削除
- **ブロックサイズ対応** - 実際に解放されるディスク容量を正確に計算
- **柔軟な制約** - MinFreeSpace（推奨）、MaxUsagePercent、またはMaxSize
- **進捗監視** - 操作追跡のためのリアルタイムコールバック
- **クロスプラットフォーム** - Linux、macOS、Windows対応

## インストール

```bash
go get github.com/ideamans/go-backup-cleaner
```

## 使用方法

```go
package main

import (
    "log"
    cleaner "github.com/ideamans/go-backup-cleaner"
)

func main() {
    // 最小空き容量の要件を設定（推奨アプローチ）
    minFree := int64(10 * 1024 * 1024 * 1024) // 10GB
    config := cleaner.CleaningConfig{
        MinFreeSpace:    &minFree,
        RemoveEmptyDirs: true,
        Callbacks: cleaner.Callbacks{
            OnFileDeleted: func(info cleaner.FileDeletedInfo) {
                log.Printf("削除: %s (%d バイト)", info.Path, info.Size)
            },
        },
    }

    report, err := cleaner.CleanBackup("/path/to/backup", config)
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("%d ファイルを削除し、%d バイトを %v で解放しました",
        report.DeletedFiles, report.DeletedSize, report.TotalDuration)
}
```

## 設定オプション

### 容量指定（少なくとも1つ必須）

- `MinFreeSpace`: 最小空き容量（バイト単位）（推奨される主要オプション）
- `MaxUsagePercent`: 最大ディスク使用率（0-100）
- `MaxSize`: 最大総サイズ（バイト単位）（ディスク情報が利用できない場合の代替）

### オプション設定

- `TimeWindow`: ファイル集計の時間間隔（デフォルト: 5分）
- `RemoveEmptyDirs`: 空ディレクトリを削除するか（デフォルト: true）
- `Concurrency`: 並列処理の並行度（デフォルト: runtime.NumCPU()）
- `MaxConcurrency`: 最大並行度（デフォルト: 4）

#### 並列処理設定

このパッケージはファイルのスキャンと削除に並列処理を使用します。並列処理のレベルを制御できます：

- `Concurrency`: 希望する並列処理の並行度を指定します。0に設定すると、CPUコア数がデフォルトとして使用されます。
- `MaxConcurrency`: 並行度の最大値を制限します。デフォルトは4です。
- 実際の並行度は `config.ActualWorkerCount()` で取得でき、`min(Concurrency, MaxConcurrency)` を返します。

`MaxConcurrency` を4に制限する理由：

- ベンチマークの結果、4以上の並列ワーカーでは性能向上が限定的であることが判明
- ディスクI/Oがボトルネックとなり、過度な並列化は効果が薄い
- ほとんどのシステムで最適なリソース利用を提供

#### ブロックサイズ

クリーナーはディスク容量を計算する際に「ブロックサイズ」を考慮します。ブロックサイズとは、ファイルシステムが使用する最小割り当て単位のことです。ファイルがディスクに保存される際、実際のファイルサイズが小さくても、ブロックサイズの倍数の容量を占有します。例えば：

- 4KBブロックのファイルシステムで1KBのファイルは、実際には4KBのディスク容量を使用
- 同じシステムで5KBのファイルは8KB（2ブロック）を使用

このパッケージはファイルサイズと、ファイル削除時に実際に解放されるディスク容量の両方を正確に追跡し、精密な容量管理を実現します。

### コールバック

クリーニングプロセスを監視するためのコールバック：

- `OnStart`: クリーニング開始時に呼び出される
- `OnScanComplete`: ファイルスキャン完了後に呼び出される
- `OnDeleteStart`: 削除開始前に呼び出される
- `OnFileDeleted`: 各ファイル削除時に呼び出される
- `OnDirDeleted`: 各ディレクトリ削除時に呼び出される
- `OnComplete`: クリーニング完了時に呼び出される
- `OnError`: 致命的でないエラー時に呼び出される

## 動作原理

1. **スキャン**: バックアップディレクトリをスキャンしてすべてのファイルをカタログ化
2. **計算**: 指定に基づいて解放すべき容量を計算
3. **決定**: 時刻の閾値を決定 - これより古いファイルが削除される
4. **削除**: 最も古いファイルから並列で削除
5. **クリーンアップ**: 空ディレクトリを削除（有効な場合）

### クリーンアップ前のディスク容量確認

パッケージは、クリーンアップ操作を実行する前に利用可能なディスク容量を素早く確認するための便利な関数 `GetDiskFreeSpace` を提供しています：

```go
// 完全な操作を実行する前にクリーンアップが必要かチェック
freeSpace, err := cleaner.GetDiskFreeSpace("/path/to/backup")
if err != nil {
    log.Fatal(err)
}

// 空き容量が閾値以下の場合のみクリーンアップを実行
if freeSpace < requiredFreeSpace {
    report, err := cleaner.CleanBackup("/path/to/backup", config)
    // ...
}
```

これにより、ディスク容量が既に十分な場合に不必要なファイルスキャンを避けることができ、効率的な事前チェックが可能になります。

### 適切な容量指定の選び方

**MinFreeSpace（推奨）**: これはほとんどのユースケースにおいて最も直感的で推奨されるオプションです。特定の空きディスク容量を常に確保し、バックアップシステムの正常な動作を保証するために通常必要とされる要件を満たします。

**MaxUsagePercent**: 異なるサイズのボリューム間でパーセンテージベースのディスク使用ポリシーを維持したい場合に便利です。

**MaxSize**: ディスク使用量情報が利用できない場合（例：権限やOSの制限により）のフォールバックオプションとして最適です。このモードでは、総サイズが指定された制限以下になるまで古いファイルを削除します。これは以下の場合に有用です：

- ディスクアクセスが制限された環境
- ディスク使用量APIが利用できないネットワークストレージ
- 簡易的なクォータベースのクリーンアップ

注意：`MaxUsagePercent`と`MinFreeSpace`はディスク使用量情報を必要とし、ディスク使用量が利用できない場合は使用できません。

## テスト

テストの実行：

```bash
go test -v ./...
```

カバレッジ付きでテストを実行：

```bash
go test -v -cover ./...
```

## ライセンス

MITライセンス - 詳細はLICENSEファイルを参照してください。