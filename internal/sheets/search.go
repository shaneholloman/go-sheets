package sheets

import (
	"fmt"
	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
)

func (m *model) startSearchPrompt(direction int) tea.Cmd {
	m.clearNormalPrefixes()
	m.mode = commandMode
	if direction < 0 {
		m.promptKind = searchBackwardPrompt
	} else {
		m.promptKind = searchForwardPrompt
	}
	m.commandPending = true
	m.commandBuffer = ""
	m.commandCursor = 0
	return tea.Batch(
		m.editCursor.Focus(),
		m.editCursor.SetMode(cursor.CursorBlink),
	)
}

func (m *model) executeSearchPrompt(query string, promptKind promptKind) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		query = m.searchQuery
	}
	if query == "" {
		return nil
	}

	direction := 1
	if promptKind == searchBackwardPrompt {
		direction = -1
	}
	m.searchQuery = query
	m.searchDirection = direction
	if !m.search(query, direction, 1) {
		m.commandMessage = fmt.Sprintf("pattern not found: %s", query)
		m.commandError = true
	}
	return nil
}

func (m *model) search(query string, direction, count int) bool {
	if count <= 0 {
		count = 1
	}
	origin := cellKey{row: m.selectedRow, col: m.selectedCol}
	current := origin
	targets := count
	last := cellKey{}
	for matched := 0; matched < targets; matched++ {
		ref, ok := m.findNextSearchMatchFrom(query, direction, current)
		if !ok {
			return false
		}
		last = ref
		current = ref
	}
	if last != origin {
		m.recordJumpFrom(origin)
		m.goToCell(last.row, last.col)
	}
	return true
}

func (m model) findNextSearchMatchFrom(query string, direction int, startCell cellKey) (cellKey, bool) {
	query = strings.ToLower(query)
	totalCells := m.rowCount * totalCols
	start := startCell.row*totalCols + startCell.col
	for step := 1; step <= totalCells; step++ {
		index := (start + direction*step + totalCells) % totalCells
		row := index / totalCols
		col := index % totalCols
		if m.matchesSearch(row, col, query) {
			return cellKey{row: row, col: col}, true
		}
	}
	return cellKey{}, false
}

func (m model) matchesSearch(row, col int, query string) bool {
	value := m.cellValue(row, col)
	if value == "" {
		return false
	}
	if strings.Contains(strings.ToLower(value), query) {
		return true
	}
	return strings.Contains(strings.ToLower(m.displayValue(row, col)), query)
}

func (m *model) repeatSearch(count int, reverse bool) {
	if m.searchQuery == "" || m.searchDirection == 0 {
		return
	}
	direction := m.searchDirection
	if reverse {
		direction *= -1
	}
	if !m.search(m.searchQuery, direction, count) {
		m.commandMessage = fmt.Sprintf("pattern not found: %s", m.searchQuery)
		m.commandError = true
	}
}
