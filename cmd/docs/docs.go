// Package docscmd implements `shoplazza docs search` and `shoplazza docs get`.
package docscmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/build"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/docs"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

const (
	maxLimit        = 20
	defaultMaxChars = 20000
)

// Swapped in tests.
var newClient = func() *docs.Client { return docs.NewClient(build.DisplayVersion()) }

func NewCmdDocs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Search and read the shoplazza.dev developer docs",
		Long: `Search and read the developer docs on www.shoplazza.dev (guides, API reference,
changelog). No login needed.

For this CLI's own commands, use 'shoplazza schema' (offline).`,
		Annotations: map[string]string{cmdutil.AnnotationAuthFree: "true"},
	}
	cmd.AddCommand(newSearchCmd(), newGetCmd())
	return cmd
}

func newSearchCmd() *cobra.Command {
	var (
		query, locale, apiVersion string
		limit                     int
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search the developer docs",
		Long: `Search the developer docs. Each result is one matching passage (url, title,
content, score), best first; a page can appear more than once.

Examples:
  shoplazza docs search --query "webhook hmac verify" --locale en
  shoplazza docs search --query "应用代理 签名" --locale zh-CN
  shoplazza docs search --query "product-create" --api-version 202601`,
		Args: noPositional("--query"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			req := docs.SearchRequest{Query: strings.TrimSpace(query)}
			if req.Query == "" {
				return invalidFlag("--query", "--query is empty").
					WithHint(`pass short search terms, e.g. --query "webhook hmac"`)
			}
			if cmd.Flags().Changed("locale") {
				if !validLocale(locale) {
					return invalidFlag("--locale", "invalid --locale %q", locale).
						WithHint("valid values: " + strings.Join(docs.Locales, ", "))
				}
				req.Locale = locale
			}
			req.APIVersion = docs.NormalizeAPIVersion(apiVersion)
			if cmd.Flags().Changed("limit") {
				if limit < 1 || limit > maxLimit {
					return invalidFlag("--limit", "--limit must be between 1 and %d, got %d", maxLimit, limit).
						WithHint(fmt.Sprintf("pass a number from 1 to %d (10 when omitted)", maxLimit))
				}
				req.Limit = limit
			}

			res, err := newClient().Search(cmd.Context(), req)
			if err != nil {
				return searchError(err)
			}
			if results, _ := res["results"].([]any); len(results) == 0 {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
					"hint: nothing relevant found; try fewer words, the exact API or field name, or the other --locale")
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), res, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Search terms, e.g. an API or field name (required)")
	cmd.Flags().StringVar(&locale, "locale", "", "Docs language: en|zh-CN (default: zh-CN if the query has Chinese, else en)")
	cmd.Flags().StringVar(&apiVersion, "api-version", "", "API version, e.g. 202601 or 2026-01 (default: the site's default)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max results, 1-20 (default 10)")
	_ = cmd.MarkFlagRequired("query")
	return cmd
}

func newGetCmd() *cobra.Command {
	var (
		pageRef, outFile string
		maxChars         int
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Read a developer-docs page as Markdown",
		Long: `Read one docs page as Markdown. A page longer than --max-chars is cut there
(truncated: true; total_chars counts the whole page); --output saves the whole
page instead. --format pretty prints the Markdown as is.

Examples:
  shoplazza docs get --url https://www.shoplazza.dev/docs/app/building-blocks/app-proxy/overview
  shoplazza docs get --url /zh-CN/api/product --format pretty
  shoplazza docs get --url /api/product-create --output product-create.md`,
		Args: noPositional("--url"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if maxChars < 1 && outFile == "" {
				return invalidFlag("--max-chars", "--max-chars must be a positive number of characters, got %d", maxChars).
					WithHint(fmt.Sprintf("omit it for %d, or pass --output <file> for the whole page", defaultMaxChars))
			}
			pageURL, err := docs.NormalizePageURL(pageRef)
			if err != nil {
				return invalidFlag("--url", "%v", err).
					WithHint("pass a " + docs.SiteURL + "/... URL or a site path such as /docs/app/...")
			}
			page, err := newClient().GetPage(cmd.Context(), pageURL)
			if err != nil {
				return getError(err, pageURL)
			}

			format := cmdutil.GetFormat(cmd)
			total := utf8.RuneCountInString(page.Markdown)
			if outFile != "" {
				file, err := filepath.Abs(outFile)
				if err == nil {
					err = fsx.WriteFileAtomic(file, []byte(page.Markdown), 0o644)
				}
				if err != nil {
					return invalidFlag("--output", "cannot write %s: %v", outFile, fileCause(err)).
						WithHint("pass a file path in an existing, writable directory")
				}
				return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{
					"url": page.URL, "title": page.Title, "locale": page.Locale,
					"file": file, "total_chars": total,
				}, format, "")
			}

			md, truncated := docs.Truncate(page.Markdown, maxChars)
			if truncated {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"hint: page truncated to %d of %d characters; pass --output <file> for the whole page, or raise --max-chars\n",
					utf8.RuneCountInString(md), total)
			}
			if format == output.FormatPretty {
				return writeRaw(cmd, md)
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{
				"url": page.URL, "title": page.Title, "locale": page.Locale,
				"markdown": md, "total_chars": total, "truncated": truncated,
			}, format, "")
		},
	}
	cmd.Flags().StringVar(&pageRef, "url", "", "Page URL or site path, e.g. /docs/app/... (required)")
	cmd.Flags().IntVar(&maxChars, "max-chars", defaultMaxChars, "Max characters to return")
	cmd.Flags().StringVar(&outFile, "output", "", "Save the whole page to this file")
	_ = cmd.MarkFlagRequired("url")
	return cmd
}

// noPositional points at flag; cobra.NoArgs would report an unknown command.
func noPositional(flag string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return output.ErrValidation("unexpected argument %q", args[0]).
			WithSubtype(output.SubtypeInvalidArgument).
			WithHint(fmt.Sprintf("pass it with %s, e.g. %s %s %q", flag, cmd.CommandPath(), flag, strings.Join(args, " ")))
	}
}

func validLocale(l string) bool {
	for _, v := range docs.Locales {
		if l == v {
			return true
		}
	}
	return false
}

func invalidFlag(flag, format string, args ...any) *output.ExitError {
	return output.ErrValidation(format, args...).
		WithSubtype(output.SubtypeInvalidFlagValue).
		WithParam(flag)
}

// A 400 invalid_request is a bad flag value; its message lists the valid ones.
func searchError(err error) error {
	if errors.Is(err, docs.ErrSearchAPI) {
		return output.ErrValidation("%v", err).
			WithHint("set " + docs.EnvSearchAPI + " to an http(s) URL, or unset it to use the default")
	}
	var he *docs.HTTPError
	if !errors.As(err, &he) {
		return transportError(err)
	}
	e := output.ErrHTTP(he.StatusCode, errorBody(he.Body), he.TraceID).WithEndpoint(he.Method, he.Path)
	switch {
	case he.StatusCode == http.StatusBadRequest && e.Detail.Code == "invalid_request":
		return output.ErrValidation("%s", e.Detail.Message).
			WithSubtype(output.SubtypeInvalidFlagValue).
			WithHint("re-run with one of the accepted values the message lists").
			WithEndpoint(he.Method, he.Path).
			WithStatus(he.StatusCode).
			WithRequestID(he.TraceID)
	case he.StatusCode == http.StatusTooManyRequests:
		return e.WithHint(retryHint(he.RetryAfter))
	}
	return e
}

func getError(err error, pageURL string) error {
	if errors.Is(err, docs.ErrOffSite) {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // drop Go's `Get "<url>":` prefix
		}
		return invalidFlag("--url", "%s: %v", pageURL, err).
			WithHint("read pages on " + docs.SiteHost + " only")
	}
	if errors.Is(err, docs.ErrNotMarkdown) {
		return output.Errorf(output.ExitAPI, output.TypeAPI, "%s has no Markdown copy: the site returned HTML", pageURL).
			WithHint("run 'shoplazza docs search' for the page, or answer from the passages")
	}
	if errors.Is(err, docs.ErrTooManyRedirects) {
		return output.Errorf(output.ExitAPI, output.TypeAPI, "%s: %v", pageURL, docs.ErrTooManyRedirects).
			WithHint("the docs site keeps redirecting this page; run 'shoplazza docs search' for its current URL")
	}
	var he *docs.HTTPError
	if !errors.As(err, &he) {
		return transportError(err)
	}
	switch he.StatusCode {
	case http.StatusNotFound:
		return output.Errorf(output.ExitAPI, output.TypeAPI, "page not found: %s", pageURL).
			WithHint("check the URL, or run 'shoplazza docs search' again: the page may have moved or been removed").
			WithEndpoint(he.Method, he.Path).
			WithStatus(he.StatusCode)
	case http.StatusTooManyRequests:
		return output.ErrHTTP(he.StatusCode, errorBody(he.Body), "").WithEndpoint(he.Method, he.Path).
			WithHint(retryHint(he.RetryAfter))
	}
	return output.ErrHTTP(he.StatusCode, errorBody(he.Body), "").WithEndpoint(he.Method, he.Path)
}

const maxErrorBodyChars = 300

// errorBody drops an HTML or long error page (a CDN or gateway answering), so
// the message falls back to the status code.
func errorBody(body string) string {
	b := strings.TrimSpace(body)
	if json.Valid([]byte(b)) {
		return body
	}
	if strings.HasPrefix(b, "<") || utf8.RuneCountInString(b) > maxErrorBodyChars {
		return ""
	}
	return body
}

// A failed request is always network-class: http.Client.Do returns a
// *url.Error, which is a net.Error whatever it wraps.
func transportError(err error) error {
	if errors.Is(err, docs.ErrBadResponse) {
		return output.Errorf(output.ExitAPI, output.TypeAPI, "%v", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("%v", err)
	}
	return output.ErrInternal("%v", err)
}

func retryHint(retryAfter string) string {
	if s, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && s >= 0 {
		return fmt.Sprintf("rate limited; retry in %d seconds", s)
	}
	return "rate limited; retry later"
}

// fileCause drops the path: the write goes through a temp file.
func fileCause(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err
	}
	return err
}

func writeRaw(cmd *cobra.Command, md string) error {
	w := cmd.OutOrStdout()
	if _, err := fmt.Fprint(w, md); err != nil {
		return output.ErrInternal("write page: %v", err)
	}
	if !strings.HasSuffix(md, "\n") {
		_, _ = fmt.Fprintln(w)
	}
	return nil
}
