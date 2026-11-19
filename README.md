# TaskChute for Terminal

Go 1.26 と Bubble Tea v2 で動くローカル版です。macOS と Linux を対象とします。

```sh
go run .
go run . --json ls
go build -o tc .
```

実績は OS のユーザー設定フォルダ内の `taskchute-for-terminal/tasks.json` に保存します。別の保存先は `--data PATH` または `TASKCHUTE_DATA` で指定できます。オプションはコマンドより前に置きます。

## 操作

| キー | 動作 |
|---|---|
| `a` | 追加。例は `Dinner` または `30 Dinner` |
| `l`、`j/k`、`J/K` | 一覧、選択、未着手タスクの並べ替え |
| `s` / Enter、`p` | 開始・完了、一時停止 |
| `e`、`r`、`c` | 見積、名前、カテゴリを変更 |
| `S`、`F` | 開始・終了時刻を HHMM で編集。終了の `-` は実行中に戻す |
| `n` | ノートを開く。Enter で保存、Ctrl+n で改行、Ctrl+w の後 h/l で履歴と入力欄を切替、Esc で閉じる |
| `R`、`v` | ルーチン設定、ルーチンの並べ替え |
| `dd` / `15dd`、`U` | 一覧の一括削除、表示中の日の直近の削除を復元 |
| `Alt+h/l` | 前日・翌日 |
| `:` / `i`、`?`、`q` | コマンド入力、ヘルプ、終了 |

`a` で見積を省略でき、`e` で `-` を入れると未設定に戻ります。ルーチンは `R` から `daily` または `mon,wed,fri` と入力します。後ろに分数を指定すると、今後の回の標準見積が変わります。キーで開いたコマンド入力の `@` は選択中の ID です。CLI では ID を直接指定します。

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
