package products

import (
	"strconv"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// parsePrice parses a price flag value: a number >= 0. flag names the flag in
// error messages (e.g. "--price").
func parsePrice(flag, s string) (float64, error) {
	p, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, output.ErrValidation("%s must be a number, got %q", flag, s)
	}
	if p < 0 {
		return 0, output.ErrValidation("%s must be >= 0, got %v", flag, p)
	}
	return p, nil
}
