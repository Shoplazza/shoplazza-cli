package cmdutil

import "github.com/spf13/cobra"

// GetFormat returns the --format flag value from the command (or its ancestors).
// Falls back to "json" if the flag is absent or empty.
func GetFormat(cmd *cobra.Command) string {
	f, err := cmd.Flags().GetString("format")
	if err != nil || f == "" {
		return "json"
	}
	return f
}

// IsDryRun returns true when --dry-run is set on the command (or its ancestors).
func IsDryRun(cmd *cobra.Command) bool {
	v, err := cmd.Flags().GetBool("dry-run")
	return err == nil && v
}

// GetJQ returns the --jq flag value from the command (or its ancestors).
// Empty string means "no filter — print the envelope as-is".
func GetJQ(cmd *cobra.Command) string {
	s, _ := cmd.Flags().GetString("jq")
	return s
}
