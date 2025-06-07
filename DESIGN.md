# go-backup-cleaner 設計書

## 概要

バックアップファイル群のクリーニングを支援するGolangパッケージ。指定された容量制限に基づいて、古いファイルから順に削除し、空ディレクトリも自動的に削除する。

## パッケージ構造

```
go-backup-cleaner/
├── cleaner.go          // メインのクリーニングロジック
├── config.go           // 設定構造体
├── report.go           // レポート構造体
├── callback.go         // コールバック関連
├── disk.go             // ディスク情報インターフェース
├── scanner.go          // ファイルスキャン関連
├── deleter.go          // ファイル削除関連
└── cleaner_test.go     // テスト
```

## 主要なAPI

### メイン関数

```go
func CleanBackup(dirPath string, config CleaningConfig) (CleaningReport, error)
```

## 型定義

### CleaningConfig

```go
type CleaningConfig struct {
    // 容量指定（いずれか1つ以上必須）
    MaxSize           *int64   // 最大サイズ（バイト単位）
    MaxUsagePercent   *float64 // ディスク使用率の上限（0-100）
    MinFreeSpace      *int64   // 最小空き容量（バイト単位）
    
    // オプション設定
    TimeWindow        time.Duration // ファイル集計の時間間隔（デフォルト: 5分）
    RemoveEmptyDirs   bool          // 空ディレクトリを削除するか（デフォルト: true）
    
    // 並列処理設定
    Concurrency       int      // 並列処理の並行度（デフォルト: runtime.NumCPU()）
    MaxConcurrency    int      // 最大並行度の制限（デフォルト: 4）
    
    // コールバック
    Callbacks         Callbacks
    
    // 依存性注入
    DiskInfo          DiskInfoProvider // nilの場合はデフォルト実装を使用
}
```

#### 並列処理設定について

- `Concurrency`: 並列処理の並行度を指定します。0の場合はCPU数が使用されます。
- `MaxConcurrency`: 並行度の上限を設定します。デフォルトは4です。
- 実際の並行度は `config.ActualWorkerCount()` で取得できます（`min(Concurrency, MaxConcurrency)`を返します）。

MaxConcurrencyを4に制限している理由：
- ベンチマークの結果、4以上に増やしても性能向上が限定的であることが判明
- ディスクI/Oがボトルネックとなるため、過度な並列化は効果が薄い
- システムリソースを効率的に利用するための最適値

### DiskInfoProvider

```go
type DiskInfoProvider interface {
    GetDiskUsage(path string) (*DiskUsage, error)
    GetBlockSize(path string) (int64, error)
}

type DiskUsage struct {
    Total     uint64
    Free      uint64
    Used      uint64
    UsedPercent float64
}

// デフォルト実装
type DefaultDiskInfoProvider struct{}
```

### Callbacks

```go
type Callbacks struct {
    OnStart         func(info StartInfo)
    OnScanComplete  func(info ScanCompleteInfo)
    OnDeleteStart   func(info DeleteStartInfo)
    OnFileDeleted   func(info FileDeletedInfo)
    OnDirDeleted    func(info DirDeletedInfo)
    OnComplete      func(info CompleteInfo)
}

// コールバック情報構造体
type StartInfo struct {
    TargetDir     string
    CurrentUsage  DiskUsage
    TargetSize    int64 // 削除すべきサイズ
}

type ScanCompleteInfo struct {
    ScannedFiles  int
    TotalSize     int64
    BlockSize     int64
    TimeThreshold time.Time // 削除対象の閾値
    ScanDuration  time.Duration
}

type DeleteStartInfo struct {
    EstimatedFiles int
    EstimatedSize  int64
}

type FileDeletedInfo struct {
    Path      string
    Size      int64
    BlockSize int64
    ModTime   time.Time
}

type DirDeletedInfo struct {
    Path string
}

type CompleteInfo struct {
    DeletedFiles    int
    DeletedSize     int64
    DeletedBlockSize int64
    DeletedDirs     int
    DeleteDuration  time.Duration
}
```

### CleaningReport

```go
type CleaningReport struct {
    // 削除統計
    DeletedFiles     int
    DeletedSize      int64 // 実ファイルサイズ
    DeletedBlockSize int64 // ブロック単位のサイズ
    DeletedDirs      int
    
    // 処理時間
    ScanDuration     time.Duration
    DeleteDuration   time.Duration
    TotalDuration    time.Duration
    
    // その他の情報
    ScannedFiles     int
    TimeThreshold    time.Time // 削除対象となった時刻の閾値
    BlockSize        int64
}
```

## 内部構造体

### fileInfo (内部使用)

```go
type fileInfo struct {
    path      string
    size      int64
    blockSize int64
    modTime   time.Time
}

// 時間間隔ごとの集計用
type timeSlot struct {
    time      time.Time
    files     []fileInfo
    totalSize int64
    totalBlockSize int64
}

// 並列処理用のワーカータスク
type scanTask struct {
    path string
}

// 削除されたファイルの親ディレクトリ管理
type deletedDirs struct {
    mu   sync.Mutex
    dirs map[string]struct{} // Setとして使用
}
```

## 処理フロー

1. **初期化と検証**
   - 設定の検証（容量指定が少なくとも1つあるか）
   - DiskInfoProviderの初期化（nilの場合はデフォルト実装）
   - ディスク使用状況の取得

2. **削除サイズの計算**
   - 現在のディスク使用状況から、削除すべきサイズを計算
   - MaxSize, MaxUsagePercent, MinFreeSpaceから最も厳しい条件を採用

3. **ファイルスキャン（第1回走査・並列）**
   - 並列でディレクトリを走査
   - TimeWindowごとに更新日時とサイズを集計
   - ブロックサイズを考慮したサイズ計算

4. **削除閾値の計算**
   - 古い時間スロットから順に、削除サイズに達するまで累積
   - 削除対象となる更新日時の閾値を決定

5. **ファイル削除（第2回走査・並列）**
   - 並列でディレクトリを走査
   - 閾値より古いファイルを削除
   - 削除したファイルの親ディレクトリを記録

6. **空ディレクトリ削除（第3回走査・逐次）**
   - 削除したファイルがあったディレクトリから開始
   - 深さ優先で空ディレクトリを削除（設定による）
   - 親ディレクトリも空になったら再帰的に削除

7. **レポート生成**
   - 削除統計と処理時間をまとめて返却

## エラーハンドリング

```go
var (
    ErrNoCapacitySpecified = errors.New("no capacity limit specified")
    ErrInvalidConfig      = errors.New("invalid configuration")
    ErrDirectoryNotFound  = errors.New("directory not found")
    ErrInsufficientSpace  = errors.New("cannot free enough space")
)
```

## 使用例

```go
// 基本的な使用例
config := CleaningConfig{
    MaxUsagePercent: &percent80,
    TimeWindow: 10 * time.Minute, // デフォルトは5分
    Callbacks: Callbacks{
        OnFileDeleted: func(info FileDeletedInfo) {
            log.Printf("Deleted: %s (%d bytes)", info.Path, info.Size)
        },
        OnError: func(info ErrorInfo) {
            log.Printf("Error [%v]: %v", info.Type, info.Error)
        },
        // 他のコールバックはnilでOK
    },
}

report, err := CleanBackup("/backup", config)
if err != nil {
    log.Fatal(err)
}

log.Printf("Deleted %d files, freed %d bytes", report.DeletedFiles, report.DeletedSize)
```

## テストシナリオ

1. **基本シナリオ**
   - 複数の日付にまたがるファイル群
   - 指定容量に達するまで古いファイルを削除

2. **削除サイズ計算のテスト**
   - MaxSizeのみ指定した場合の計算
   - MaxUsagePercentのみ指定した場合の計算
   - MinFreeSpaceのみ指定した場合の計算
   - 複数指定時の最も厳しい条件の選択
   - 現在の使用量が既に条件を満たしている場合
   - 削除しても条件を満たせない場合のエラー

3. **エッジケース**
   - 空ディレクトリの処理
   - シンボリックリンクの扱い
   - アクセス権限がないファイル
   - ファイル削除中のエラーハンドリング

4. **モックを使用したテスト**
   - DiskInfoProviderのモック実装
   - 様々なディスク使用状況のシミュレーション
   - ブロックサイズの異なる環境のテスト

## CI/CD

GitHub Actionsを使用して、以下の環境でテストを実行：

```yaml
# .github/workflows/test.yml
name: Test

on:
  push:
    branches: [ "*" ]
  pull_request:
    branches: [ "*" ]

jobs:
  test:
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
        go: ['1.21', '1.22']
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
      - run: go test -v ./...
```

各OS固有のファイルシステムの挙動（特にブロックサイズの取得）をテストで確認。

## 実装上の注意点

1. **並行性**
   - ファイルスキャンとファイル削除は並列処理（Concurrency/MaxConcurrencyで制御）
   - 空ディレクトリ削除は逐次処理（ディレクトリ階層の整合性を保つため）
   - sync.Poolを使用してfileInfo構造体を再利用
   - チャネルを使用したワーカープール実装
   - 並行度は最大4に制限（ベンチマークによる最適化）

2. **メモリ効率**
   - 時間間隔での集計によりメモリ使用量を抑制
   - 削除したディレクトリはSetで管理し重複を除去
   - 大量ファイルでもOOMにならない設計

3. **エラー処理**
   - ファイル削除失敗は個別にログに記録し、処理は継続
   - エラーはOnErrorコールバックで通知
   - 致命的なエラー（ディスク情報取得失敗など）のみ処理を中断
   - 並列処理でのエラーは適切に集約

4. **プラットフォーム対応**
   - ブロックサイズの取得はOS依存（syscall使用）
   - Windows/Linux/macOSでの動作確認
   - GitHub ActionsでマルチプラットフォームCI

## 依存ライブラリ

- `github.com/shirou/gopsutil/v3/disk` - ディスク使用状況の取得
- 標準ライブラリのみで実装も検討（psutilが使えない場合）
