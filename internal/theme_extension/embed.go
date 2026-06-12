package theme_extension

import "embed"

// templatesFS bundles the basic/embed te project templates. "all:" includes
// dotfiles (e.g. .gitignore) the templates carry.
//
//go:embed all:templates/basic all:templates/embed
var templatesFS embed.FS
