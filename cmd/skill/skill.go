// Package skill exposes the Agent Skills embedded in the CLI binary through
// `shoplazza skills list` and `shoplazza skills read`. The content is compiled
// in at build time, so an agent always reads the guidance that matches the
// exact binary it is running — no filesystem install, no version skew.
package skill

import (
	"fmt"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/skillcontent"

	"github.com/spf13/cobra"
)

// NewCmdSkill creates the `skills` command group.
func NewCmdSkill() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Read the Agent Skills embedded in this CLI (list / read)",
		Long: `Read the Agent Skills compiled into this CLI binary.

The skills are the same guidance an agent installs into its workspace, but
served from the running binary — so they always match this exact CLI version.

  shoplazza skills list                      list every embedded skill
  shoplazza skills list <name>               list the files inside one skill
  shoplazza skills read <name>               print a skill's SKILL.md
  shoplazza skills read <name> <ref-path>    print a reference file in the skill

Start with 'shoplazza skills read shoplazza-common'.`,
		Annotations: map[string]string{cmdutil.AnnotationAuthFree: "true"},
	}
	cmd.AddCommand(newListCmd(), newReadCmd())
	return cmd
}

func reader() *skillcontent.Reader { return skillcontent.New(skillcontent.Embedded()) }

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [skill[/path]]",
		Short: "List embedded skills, or the files inside one skill",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := reader()
			format, jq := cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd)

			if len(args) == 0 {
				skills, err := r.List()
				if err != nil {
					return output.ErrInternal("read embedded skills: %v", err)
				}
				// Render as []map[string]any so --format table/pretty lay the
				// skills out in rows; json/jq see the same shape.
				rows := make([]map[string]any, len(skills))
				for i, s := range skills {
					rows[i] = map[string]any{"name": s.Name, "description": s.Description}
				}
				return output.PrintBody(cmd.OutOrStdout(), map[string]any{
					"skills": rows,
					"count":  len(rows),
				}, format, jq)
			}

			name, sub := skillcontent.SplitArg(args[0])
			entries, dir, err := r.ListPath(name, sub)
			if err != nil {
				return output.ErrValidation("%v", err).
					WithHint("run 'shoplazza skills list' to see the available skills")
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"skill":   name,
				"path":    dir,
				"entries": entries,
			}, format, jq)
		},
	}
}

func newReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read <skill>[/path] [ref-path]",
		Short: "Print a skill's SKILL.md, or a reference file inside it",
		Long: `Print a skill's content as raw Markdown (like cat).

  shoplazza skills read shoplazza-orders
  shoplazza skills read shoplazza-orders references/refunds.md
  shoplazza skills read shoplazza-orders/references/refunds.md`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, rel := skillcontent.SplitArg(args[0])
			if len(args) == 2 {
				if rel != "" {
					return output.ErrValidation("pass the reference path either after a slash or as a second argument, not both").
						WithHint(fmt.Sprintf("try 'shoplazza skills read %s %s'", name, args[1]))
				}
				rel = args[1]
			}

			r := reader()
			var (
				data []byte
				err  error
			)
			if rel == "" {
				data, err = r.ReadSkill(name)
			} else {
				data, _, err = r.ReadReference(name, rel)
			}
			if err != nil {
				return output.ErrValidation("cannot read skill %q: %v", args[0], err).
					WithHint("run 'shoplazza skills list' (or 'shoplazza skills list <skill>') to see what's available")
			}
			return writeRaw(cmd, data)
		},
	}
}

// writeRaw prints file content verbatim, guaranteeing a single trailing newline
// so terminal output isn't glued to the next prompt.
func writeRaw(cmd *cobra.Command, data []byte) error {
	w := cmd.OutOrStdout()
	if _, err := w.Write(data); err != nil {
		return output.ErrInternal("write skill content: %v", err)
	}
	if n := len(data); n == 0 || data[n-1] != '\n' {
		_, _ = fmt.Fprintln(w)
	}
	return nil
}
