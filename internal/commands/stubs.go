package commands

import "github.com/spf13/cobra"

func newStub(use string) *cobra.Command {
	return &cobra.Command{
		Use:    use,
		Hidden: true,
		RunE:   func(*cobra.Command, []string) error { return nil },
	}
}

// These are placeholders for commands that will be implemented in later
// tasks. As each command is implemented in its own file, REMOVE its line
// from here and the constructor from this file's exports.
func NewList() *cobra.Command      { return newStub("list") }
func NewExport() *cobra.Command    { return newStub("export") }
func NewFolders() *cobra.Command   { return newStub("folders") }
func NewSchema() *cobra.Command    { return newStub("schema") }
func NewSQL() *cobra.Command       { return newStub("sql") }
