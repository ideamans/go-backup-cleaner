# go-backup-cleaner

[English](README.md) | 日本語

[![Test](https://github.com/ideamans/go-backup-cleaner/actions/workflows/test.yml/badge.svg)](https://github.com/ideamans/go-backup-cleaner/actions/workflows/test.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ideamans/go-backup-cleaner.svg)](https://pkg.go.dev/github.com/ideamans/go-backup-cleaner)

容量指定に基づいてバックアップファイルをクリーニングするGoパッケージです。指定されたディスク使用量の目標を満たすために古いファイルを自動的に削除し、空のディレクトリをクリーンアップできます。

## 機能

- 複数の容量指定オプション（最大サイズ、最大使用率、最小空き容量）
- パフォーマンス向上のための並列ファイルスキャンと削除
- 大量ファイルセットでのメモリ効率を考慮した時間間隔での集計
- ブロックサイズを考慮した削除（実際に解放されるディスク容量）
- 進捗監視のためのコールバックシステム
- 空ディレクトリのクリーンアップ
- クロスプラットフォーム対応（Linux、macOS、Windows）

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
    // ディスク使用率の上限を80%に設定
    maxUsage := 80.0
    config := cleaner.CleaningConfig{
        MaxUsagePercent: &maxUsage,
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

- `MaxSize`: 最大総サイズ（バイト単位）
- `MaxUsagePercent`: 最大ディスク使用率（0-100）
- `MinFreeSpace`: 最小空き容量（バイト単位）

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

### 特殊なケース: ディスク使用量なしでのMaxSize

ディスク使用量情報が利用できない場合（例：権限やOSの制限により）でも、`MaxSize`が指定されていればクリーナーは動作できます。このモードでは、総サイズが指定された制限以下になるまで古いファイルを削除します。これは以下の場合に有用です：

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