// Package state — Global state store.
package state

var globalAppState *AppState

// GetGlobalState returns the global app state.
func GetGlobalState() *AppState {
	if globalAppState == nil {
		globalAppState = NewAppState()
	}
	return globalAppState
}
