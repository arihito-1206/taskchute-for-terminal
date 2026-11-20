# TaskChute for Terminal

Go 1.26 と Bubble Tea v2 で動くローカル版です。macOS と Linux を対象とします。

```sh
go run .
go run . --json ls
go build -o tc .
```

保存先の優先順は `--data PATH` → 環境変数 `TASKCHUTE_DATA` → `config.json` の `data_path` → 従来の `tasks.json` です。オプションはコマンドより前に置きます。

`config.json` は OS のユーザー設定フォルダ内の `taskchute-for-terminal` に置きます。macOS では `~/Library/Application Support/taskchute-for-terminal/config.json` です。

```json
{
  "data_path": "/Users/yourname/Dev/git_repos/taskchute-data/tasks.json"
}
```

相対パスは設定ファイルのフォルダを基準に解決します。設定した保存先は TUI、CLI、`backup`、`restore-backup` に共通です。設定ファイルや `data_path` を省略した場合は、同じフォルダの `tasks.json` を使います。

## 操作

| キー | 動作 |
|---|---|
| `a` | 追加。例は `Dinner` または `30 Dinner` |
| `l`、`L` | 全件一覧の開閉、現在の時刻に対応するセクションの一覧 |
| `j/k`、`J/K` | タスクの選択、未着手・一時停止中のタスクの並べ替えとセクション間の移動 |
| `s` / Enter、`p` | 開始・完了、一時停止 |
| `e`、`r`、`c` | 見積、名前、カテゴリを変更 |
| `S`、`F` | 開始・終了時刻を HHMM で編集。終了の `-` は実行中に戻す |
| `n` | ノートを開く。Enter で保存、Ctrl+n で改行、Ctrl+w の後 h/l で履歴と入力欄を切替、Esc で閉じる |
| `R`、`v` | ルーチン設定、ルーチンの並べ替え |
| `b` | タスクのセクションを選ぶ。ルーチン一覧では既定のセクションを選ぶ |
| `dd` / `15dd`、`U` | 一覧の一括削除、表示中の日の直近の削除を復元 |
| `Alt+h/l` | 前日・翌日 |
| `:` / `i`、`?`、`q` | コマンド入力、ヘルプ、終了 |

`a` で見積を省略でき、`e` で `-` を入れると未設定に戻ります。ルーチンは `R` から `daily` または `mon,wed,fri` と入力します。後ろに分数を指定すると、今後の回の標準見積が変わります。キーで開いたコマンド入力の `@` は選択中の ID です。CLI では ID を直接指定します。

## セクションで一日を区切る

`config.json` がない場合や `sections` を省略した場合は、次の 4 区分を使います。

| セクション | ID | 時間帯 |
|---|---|---|
| Overnight | `overnight` | 00:00–06:00 |
| Morning | `morning` | 06:00–12:00 |
| Afternoon | `afternoon` | 12:00–18:00 |
| Evening | `evening` | 18:00–24:00 |

区切りを変えるには、`config.json` に `sections` を指定します。既に `data_path` がある場合は、同じ JSON オブジェクトに追加してください。`"sections": []` を指定するとセクションを無効にできます。

```json
{
  "sections": [
    {"id": "morning", "name": "午前", "start": "08:00", "end": "12:00"},
    {"id": "afternoon", "name": "午後", "start": "12:00", "end": "18:00"},
    {"id": "evening", "name": "夜", "start": "18:00", "end": "24:00"}
  ]
}
```

時刻はローカル時刻の `HH:MM` で指定し、開始は範囲に含み、終了は含みません。セクション間に空白の時間帯を設けられます。`--data` や `TASKCHUTE_DATA` で保存先を指定した場合も、セクションの定義と設定の検証には同じ `config.json` を使います。表示名の変更には `name` を使ってください。配置は `id` で保存するため、複数端末で同じデータを使う場合は同じセクション定義を設定してください。

タスクを選んで `b` を押すと、`j/k` と Enter でセクションを選べます。`a` で追加したタスクは未割り当てなので、全件一覧で選びます。`v` で開くルーチン一覧も、既定のセクションごとに上下の罫線で区切って表示します。`J/K` で並べ替え、境界を越えると既定のセクションも変わります。`b` で既定のセクションを直接指定できます。ルーチンから生成するタスクは既定のセクションを引き継ぎます。保存済みのタスクは個別に配置し直せます。

`l` で一日の一覧を開くと、時間帯ごとの見出しが上下の罫線に挟まれて並びます。タスクを選んで `J/K` で動かすと、同じセクション内の順番を変更できます。端からさらに動かすと、空のセクションも含めて隣のセクションへ移ります。セクションを直接選ぶには `b` を使います。

日次タスクを別のセクションに移す場合は、タスクの日付でまだ終了していないセクションを選べます。終了済みのセクションから移したり、未割り当てに戻したりできます。

配置がない実行中・完了済みのタスクは、最初の開始時刻が属するセクションに 1 行だけ表示されます。配置済みのタスクは、実際の開始時刻にかかわらず配置先に表示されます。配置を変えても、記録済みの開始・終了時刻は保持されます。

`L` を押した時点の時刻でセクションを選び、その一覧を表示します。次のセクションに入ったら、再度 `L` を押すと切り替わります。`l` で全件一覧に戻れます。

`l` と `L` のセクション見出しには、枠の長さからタスクの見積合計を引いた空き時間や超過時間が出ます。実行中・完了済みのタスクも元の見積で計算し、見積なしの件数を併記します。

工数コードを設定したタスクには、一覧で 🔖 が付きます。タスクを選ぶと、直下の `└ Code` 行にコードが表示されます。

## Python 版の実績を引き継ぐ

```sh
go run . import-python /path/to/python/prototype/data/tasks.json
```

Python の JSON を読み、Go 版の保存先にコピーします。保存先にデータがある場合は、新しいファイルを `--data` で指定します。Python 版と Go 版を同じファイルへ書き込ませず、それぞれの保存先を使います。

## ノートの履歴を編集

`Ctrl+w` を押してから `h` で履歴へ。 `j/k` でコメントを選び、`e` で本文を編集、Enter で保存します。`dd` で選んだコメントを削除できます。最初の `d` のあと `Esc` で取消。矢印キーは履歴の表示をスクロールします。`Ctrl+w` の後に `l` で入力欄へ戻ります。

## GitHub にバックアップ

GitHub にログ専用の **Private** リポジトリを作り、README を付けて初期化してから各端末にクローンしてください。GitHub への認証と Git のコミット用ユーザー設定が必要です。ログの入ったコミット履歴も共有されるので、Private 設定を保ってください。

既存データを使う場合、1台目で一度だけ取り込みます。

```sh
go run . --data "$HOME/taskchute-data/tasks.json" import-python "$HOME/Library/Application Support/taskchute-for-terminal/tasks.json"
```

`import-python` は現行 Go 版と Python 版の保存形式を読み込めます。クローン先の実際のパスを指定してください。

```sh
# 作業を終えた端末で TUI を閉じてから実行
go run . --data "$HOME/taskchute-data/tasks.json" backup

# 次に使う端末で実行してから TUI を起動
go run . --data "$HOME/taskchute-data/tasks.json" restore-backup
go run . --data "$HOME/taskchute-data/tasks.json"
```

`backup` は `tasks.json` をコミットして `origin` に push します。`restore-backup` はローカルの変更がない場合に履歴を取り込みます。使う端末は一度に1台です。両端末で編集した際は自動統合せず、バックアップを止めます。

## 学習するときの入口

1. `main.go` の `tea.NewProgram(...).Run()` で画面が始まります。
2. `tui.go` の `Update` でイベントを受けます。まず `j` を押して、`normalKey` で `selected` が変わり、`render` が選択行に `>` を付けるところを追えます。
3. `s` の場合は `application.go` の `application.execute` を介して `domain.go` の `state.apply` を呼び、記録します。CLI も同じ入口を使います。
4. タイマーは `tea.Tick` の通知で画面を更新します。経過時間は開始時刻との時刻差から計算されます。

ノートは TUI の上に重ねて表示します。カテゴリ入力にはアイコン付きの例が表示されます。同期、Web、公開配布は今後の検討事項です。

タスク追加中は名前の一部を入力すると履歴の候補を最大5件表示します。上下キーで選択し、Tab で名前を補完、Enter で新しく登録します。
