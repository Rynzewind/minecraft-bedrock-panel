package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/lxn/walk"
	"nethernet-multi-server/internal/sessionhistory"
)

type SessionRow struct {
	Time, Event, Player, Account, Server, Duration, Reason, Alert string
}

type sessionTableModel struct {
	walk.TableModelBase
	rows []SessionRow
}

func (m *sessionTableModel) RowCount() int { return len(m.rows) }
func (m *sessionTableModel) Value(r, c int) any {
	if r < 0 || r >= len(m.rows) {
		return ""
	}
	v := m.rows[r]
	return []string{v.Time, v.Event, v.Player, v.Account, v.Server, v.Duration, v.Reason, v.Alert}[c]
}
func (m *sessionTableModel) set(rows []SessionRow) {
	m.rows = append([]SessionRow(nil), rows...)
	m.PublishRowsReset()
}

func BuildSessionRows(events []sessionhistory.Event) []SessionRow {
	sorted := append([]sessionhistory.Event(nil), events...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Time.Before(sorted[j].Time) })
	eventKey := func(event sessionhistory.Event) string {
		if event.ID != "" {
			return event.ID
		}
		return fmt.Sprintf("%d/%s/%s", event.Time.UnixNano(), event.ConsoleGamertag, event.Type)
	}
	clustered := map[string]bool{}
	for i, event := range sorted {
		if event.Type != sessionhistory.Disconnected {
			continue
		}
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Time.Sub(event.Time) > 5*time.Second {
				break
			}
			if sorted[j].Type == sessionhistory.Disconnected && sorted[j].ConsoleGamertag != event.ConsoleGamertag {
				clustered[eventKey(event)] = true
				clustered[eventKey(sorted[j])] = true
			}
		}
	}
	rows := make([]SessionRow, 0, len(events))
	for _, event := range events {
		duration := ""
		if event.Type == sessionhistory.Disconnected {
			duration = (time.Duration(event.DurationMS) * time.Millisecond).Round(time.Second).String()
		}
		alert := ""
		if clustered[eventKey(event)] {
			alert = "SIMULTANEOUS DROP"
		}
		account := event.AccountLabel
		if event.AccountGamertag != "" {
			account = fmt.Sprintf("%s (%s)", account, event.AccountGamertag)
		}
		rows = append(rows, SessionRow{event.Time.Local().Format("02/01/2006 15:04:05.000"), string(event.Type), event.ConsoleGamertag, account, event.Server, duration, event.Reason, alert})
	}
	return rows
}
