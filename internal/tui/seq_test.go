package tui

import (
	"reflect"

	tea "charm.land/bubbletea/v2"
)

// sequenceOf unpacks the message tea.Sequence produces: a slice of commands in an
// unexported type.
func sequenceOf(msg tea.Msg) []tea.Cmd {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeOf((tea.Cmd)(nil)) {
		return nil
	}
	out := make([]tea.Cmd, v.Len())
	for i := range out {
		out[i], _ = v.Index(i).Interface().(tea.Cmd)
	}
	return out
}
