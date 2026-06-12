package completion

import (
	"fmt"

	"shoplazza-cli-v2/internal/cmdutil"

	"github.com/spf13/cobra"
)

// NewCmdCompletion generates shell completion scripts.
func NewCmdCompletion(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "completion <shell>",
		Short:  "Generate shell completion scripts",
		Hidden: true,
		Long: `Generate shell completion scripts for bash, zsh, fish, or powershell.

Installation:

  # zsh — add to ~/.zshrc:
  eval "$(shoplazza completion zsh)"

  # bash — add to ~/.bashrc or ~/.bash_profile:
  eval "$(shoplazza completion bash)"

  # fish — save to completions dir:
  shoplazza completion fish > ~/.config/fish/completions/shoplazza.fish

  # powershell — add to $PROFILE:
  shoplazza completion powershell | Out-String | Invoke-Expression`,
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			out := f.IOStreams.Out
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			default:
				return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish, powershell)", args[0])
			}
		},
	}
	return cmd
}
