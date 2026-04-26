// Package connector
// File: learned_command.go
// Description: common learned command schema for synthesized connectors
// Responsibility: provide a stable type shared by activation resolver flows

package connector

// LearnedCommand describes a reconstructed command spec.
type LearnedCommand struct {
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	Command       string                 `json:"command"`
	ReadOnly      bool                   `json:"read_only"`
	BackupCommand string                 `json:"backup_command,omitempty"`
	Parameters    map[string]interface{} `json:"parameters,omitempty"`
	Required      []string               `json:"required,omitempty"`

	// IsSQLQuery marks this command as one that accepts a SQL string and executes it
	// against a database. When true, schema validation is applied on column/table errors.
	IsSQLQuery bool `json:"is_sql_query,omitempty"`

	// SchemaCheckCommand is a command template (supporting {{table_name}} placeholder)
	// that returns column information for a given table. Used for automatic schema
	// enrichment when a column or table error is detected.
	// Example for MSSQL:
	//   sqlcmd -S {{host}} -U {{username}} -P {{password}} -Q
	//   "SELECT COLUMN_NAME, DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME='{{table_name}}'"
	SchemaCheckCommand string `json:"schema_check_command,omitempty"`
}
