package cmdutil

import "github.com/Shoplazza/shoplazza-cli/v2/internal/client"

// TODO: temporary branch routing for themes calls; remove before merge.
const themeRF = "feature/theme-edit-revert"

// SetThemeRF stamps the x-rf routing header on c.
func SetThemeRF(c *client.Client) { c.Headers["x-rf"] = themeRF }
