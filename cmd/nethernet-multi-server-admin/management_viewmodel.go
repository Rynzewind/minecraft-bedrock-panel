package main

import (
	"fmt"
	"github.com/lxn/walk"
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/supervisor"
	"sort"
)

type ServerRow struct {
	ID, Name, State                                            string
	PID, Players                                               int
	Uptime, Pending                                            string
	CanStart, CanStop, CanRestart, NeedsDisconnectConfirmation bool
}

type serverTableModel struct {
	walk.TableModelBase
	rows []ServerRow
}

func (m *serverTableModel) RowCount() int { return len(m.rows) }
func (m *serverTableModel) Value(r, c int) any {
	if r < 0 || r >= len(m.rows) {
		return ""
	}
	x := m.rows[r]
	switch c {
	case 0:
		return x.Name
	case 1:
		return x.State
	case 2:
		return x.Players
	case 3:
		return x.PID
	case 4:
		return x.Pending
	}
	return ""
}
func (m *serverTableModel) set(rows []ServerRow) {
	// A full native reset clears selection and recalculates adaptive row metrics.
	// Polls normally change only cell values, so preserve native items by ID/order.
	sameItems := len(rows) == len(m.rows)
	if sameItems {
		for i := range rows {
			if rows[i].ID != m.rows[i].ID {
				sameItems = false
				break
			}
		}
	}
	if !sameItems {
		m.rows = append([]ServerRow(nil), rows...)
		m.PublishRowsReset()
		return
	}
	for i, row := range rows {
		if row != m.rows[i] {
			m.rows[i] = row
			m.PublishRowChanged(i)
		}
	}
}

type packTableModel struct {
	walk.TableModelBase
	rows []PackRow
}

func (m *packTableModel) RowCount() int { return len(m.rows) }
func (m *packTableModel) Value(r, c int) any {
	if r < 0 || r >= len(m.rows) {
		return ""
	}
	x := m.rows[r]
	switch c {
	case 0:
		return x.Name
	case 1:
		return x.Type
	case 2:
		return x.Version
	case 3:
		return x.UUID
	case 4:
		return x.Creative
	case 5:
		return x.Survival
	}
	return ""
}
func (m *packTableModel) set(rows []PackRow) {
	m.rows = append([]PackRow(nil), rows...)
	m.PublishRowsReset()
}

func BuildServerRow(s supervisor.Status) ServerRow {
	r := ServerRow{ID: s.ID, Name: s.Name, State: string(s.State), PID: s.PID, Players: s.Players}
	if s.State == supervisor.ExternallyRunning {
		r.State = string(supervisor.Running)
	}
	r.CanStart = s.State == supervisor.Stopped || s.State == supervisor.Failed
	r.CanStop = s.State == supervisor.Running
	r.CanRestart = s.State == supervisor.Running
	r.NeedsDisconnectConfirmation = s.Players > 0 && (r.CanStop || r.CanRestart)
	if !s.Started.IsZero() {
		r.Uptime = fmt.Sprint(s.Started)
	}
	if s.Pending {
		r.Pending = "Pending restart"
	}
	return r
}
func BuildServerRows(statuses []supervisor.Status) []ServerRow {
	rows := make([]ServerRow, 0, len(statuses))
	for _, s := range statuses {
		rows = append(rows, BuildServerRow(s))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

type PackRow struct{ UUID, Name, Type, Version, Creative, Survival string }

func BuildPackRows(packs []contentpacks.InventoryItem) []PackRow {
	out := make([]PackRow, 0, len(packs))
	for _, p := range packs {
		out = append(out, PackRow{UUID: p.UUID, Name: p.Name, Type: string(p.Type), Version: fmt.Sprintf("%d.%d.%d", p.Version[0], p.Version[1], p.Version[2]), Creative: assignmentText(p.Servers["creative"]), Survival: assignmentText(p.Servers["survival"])})
	}
	return out
}

func assignmentText(state contentpacks.AssignmentState) string {
	current := "Disabled"
	if state.Enabled {
		current = "Enabled"
	}
	if state.Pending == nil {
		return current
	}
	next := "disable"
	if *state.Pending {
		next = "enable"
	}
	return current + " (pending " + next + ")"
}

func accountRowIndex(rows []AccountRow, id string) int {
	for i, row := range rows {
		if row.ID == id {
			return i
		}
	}
	return -1
}
func playerRowIndex(rows []PlayerRow, xuid string) int {
	for i, row := range rows {
		if row.XUID == xuid {
			return i
		}
	}
	return -1
}
func serverRowIndex(rows []ServerRow, id string) int {
	for i, row := range rows {
		if row.ID == id {
			return i
		}
	}
	return -1
}
func packRowIndex(rows []PackRow, uuid string) int {
	for i, row := range rows {
		if row.UUID == uuid {
			return i
		}
	}
	return -1
}

func selectedPackUUIDs(rows []PackRow, indexes []int) []string {
	out := make([]string, 0, len(indexes))
	for _, index := range indexes {
		if index >= 0 && index < len(rows) {
			out = append(out, rows[index].UUID)
		}
	}
	return out
}

func packIndexesByUUID(rows []PackRow, uuids []string) []int {
	selected := make(map[string]bool, len(uuids))
	for _, uuid := range uuids {
		selected[uuid] = true
	}
	out := make([]int, 0, len(uuids))
	for i, row := range rows {
		if selected[row.UUID] {
			out = append(out, i)
		}
	}
	return out
}
