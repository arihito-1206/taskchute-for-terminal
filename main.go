package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

const commands = `add [MIN] TITLE | start [ID] | done | pause | ls | status
rename ID TITLE | estimate ID MIN|- | category ID NAME|- | note ID TEXT
code ID CODE|- | routine-code ID CODE|-
set-start ID HHMM | set-end ID HHMM|- | reset ID | move ID before|after ID
routine ID REPEAT [MIN] | routine-move ID up|down | routine-delete ID
REPEAT: daily[/N] | weekday[/N] | holiday[/N] | mon,wed[/N] | monthly:DAY[/N] | monthly:WEEK:WEEKDAY[/N]
section ID SECTION|- | routine-section ID SECTION|-
delete ID... | undo-delete | import-python PATH
backup | restore-backup`

func run() error {
	data := flag.String("data", "", "path to local JSON data (overrides TASKCHUTE_DATA and config.json)")
	day := flag.String("day", "", "date (YYYY-MM-DD; default today)")
	machine := flag.Bool("json", false, "machine-readable output")
	flag.Parse()
	if flag.Arg(0) == "help" {
		fmt.Println(commands)
		return nil
	}
	config, err := loadSettings(*data)
	if err != nil {
		return err
	}
	now := time.Now()
	if *day == "" {
		*day = now.Format(dateLayout)
	}
	if _, err = time.Parse(dateLayout, *day); err != nil {
		return errors.New("invalid --day (use YYYY-MM-DD)")
	}
	repo := repository{path: config.DataPath}
	app := application{store: repo, sections: config.Sections}
	if flag.NArg() == 0 || flag.Arg(0) == "shell" {
		if *machine {
			return errors.New("--json requires a command")
		}
		if flag.NArg() > 1 {
			return errors.New("shell takes no arguments")
		}
		initial, err := app.execute(*day, "status", nil, now)
		if err != nil {
			return err
		}
		pinned := *day != now.Format(dateLayout)
		_, err = tea.NewProgram(terminal{app: app, data: initial, day: *day, now: now, pinned: pinned, splash: true}).Run()
		return err
	}
	cmd, args := flag.Arg(0), flag.Args()[1:]
	if cmd == "backup" || cmd == "restore-backup" {
		if len(args) != 0 {
			return errors.New("backup commands take no arguments")
		}
		return repo.syncData(cmd == "restore-backup")
	}
	if cmd == "import-python" {
		if len(args) != 1 {
			return errors.New("use import-python PATH")
		}
		return repo.importFile(args[0])
	}
	s, err := app.execute(*day, cmd, args, now)
	if err != nil {
		return err
	}
	if *machine {
		return json.NewEncoder(os.Stdout).Encode(s.output(*day, now))
	}
	fmt.Println(summary(s, *day, now))
	if cmd == "ls" {
		fmt.Println("ID        STATE    TASK                           EST     START        END          ELAPSED")
		for _, t := range s.rows(*day) {
			codeLabel := ""
			if t.WorkCode != "" {
				codeLabel = "[" + terminalText(t.WorkCode, false) + "]"
			}
			fmt.Printf("%.8s  %s\n", terminalText(t.ID, false), row(t, *day, now, 28, codeLabel))
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, terminalText(err.Error(), true))
		os.Exit(1)
	}
}
