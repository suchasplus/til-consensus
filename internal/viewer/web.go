package viewer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "embed"

	"github.com/suchasplus/til-consensus/consensus"
)

//go:embed web_template.html
var webTemplateSource string

var webPageTemplate = template.Must(template.New("web-viewer").Parse(webTemplateSource))

type WebOptions struct {
	Host          string
	Port          int
	RenderOptions RenderOptions
}

type WebPageModel struct {
	Title          string
	RequestID      string
	Mode           string
	PrimaryResult  string
	TerminalState  string
	Goal           string
	URL            string
	APIPath        string
	InitialSection string
	ClaimVerdict   string
	Limit          int
	Verbose        bool
}

type WebServer struct {
	server *http.Server
	listen net.Listener
	url    string
}

func NewWebServer(bundle Bundle, options WebOptions) (*WebServer, error) {
	host := strings.TrimSpace(options.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	address := net.JoinHostPort(host, strconv.Itoa(options.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen web viewer: %w", err)
	}
	actualURL := "http://" + listener.Addr().String()
	handler, err := newWebHandler(bundle, options.RenderOptions, actualURL)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	return &WebServer{
		server: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
		listen: listener,
		url:    actualURL,
	}, nil
}

func (s *WebServer) URL() string {
	if s == nil {
		return ""
	}
	return s.url
}

func (s *WebServer) Serve(ctx context.Context) error {
	if s == nil || s.server == nil || s.listen == nil {
		return fmt.Errorf("web viewer server is not initialized")
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.Serve(s.listen)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
		err := <-errCh
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve web viewer: %w", err)
		}
		return nil
	}
}

func (s *WebServer) Close() error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Close()
}

func newWebHandler(bundle Bundle, defaults RenderOptions, baseURL string) (http.Handler, error) {
	pageModel := buildWebPageModel(bundle, baseURL, defaults)
	page, err := renderWebPage(pageModel)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/api/document", func(w http.ResponseWriter, r *http.Request) {
		options, err := parseWebRenderOptions(r.URL.Query(), defaults)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		doc := BuildDocument(bundle, options)
		body, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			http.Error(w, fmt.Sprintf("marshal web document: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(append(body, '\n'))
	})
	return mux, nil
}

func buildWebPageModel(bundle Bundle, baseURL string, defaults RenderOptions) WebPageModel {
	overview := buildOverview(bundle)
	limit := defaults.Limit
	if limit <= 0 {
		limit = 20
	}
	return WebPageModel{
		Title:          firstNonEmpty(overview.Goal, "til-consensus Web Viewer"),
		RequestID:      overview.RequestID,
		Mode:           overview.Mode,
		PrimaryResult:  overview.PrimaryResult,
		TerminalState:  overview.TerminalState,
		Goal:           overview.Goal,
		URL:            baseURL,
		APIPath:        "/api/document",
		InitialSection: inferWebSection(defaults.Sections),
		ClaimVerdict:   string(defaults.ClaimVerdict),
		Limit:          limit,
		Verbose:        defaults.Verbose,
	}
}

func inferWebSection(sections []string) string {
	values := normalizeSections(sections)
	switch {
	case len(values) == 0 || (len(values) == 1 && values[0] == SectionAll):
		return "all"
	case len(values) == 1 && values[0] == SectionOverview:
		return "overview"
	case len(values) == 1 && values[0] == SectionClaims:
		return "claims"
	case slicesContainAny(values, SectionChallenges, SectionVerifications, SectionArtifacts):
		return "evidence"
	case len(values) == 1 && values[0] == SectionObservations:
		return "observations"
	case len(values) == 1 && values[0] == SectionFollowups:
		return "followups"
	case len(values) == 1 && values[0] == SectionDebug:
		return "debug"
	case slicesContainAny(values, SectionRounds, SectionVotes, SectionStatements, SectionConvergence):
		return "workflow"
	default:
		return "files"
	}
}

func parseWebRenderOptions(query url.Values, defaults RenderOptions) (RenderOptions, error) {
	options := defaults
	if sections := parseRepeatedValues(query["section"]); len(sections) > 0 {
		expanded := make([]string, 0, len(sections))
		for _, section := range sections {
			if !isSupportedWebAPISection(section) {
				return RenderOptions{}, fmt.Errorf("unsupported web section: %s", section)
			}
			expanded = append(expanded, expandWebSection(section)...)
		}
		options.Sections = uniqueStrings(expanded)
	}
	if verdictRaw, ok := query["claim_verdict"]; ok {
		verdict := strings.TrimSpace(firstNonEmpty(verdictRaw...))
		if verdict == "" {
			options.ClaimVerdict = ""
		} else {
			if !isSupportedWebClaimVerdict(verdict) {
				return RenderOptions{}, fmt.Errorf("unsupported claim verdict filter: %s", verdict)
			}
			options.ClaimVerdict = consensus.ClaimVerdict(verdict)
		}
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return RenderOptions{}, fmt.Errorf("invalid limit: %s", raw)
		}
		options.Limit = limit
	}
	if raw := strings.TrimSpace(query.Get("verbose")); raw != "" {
		verbose, err := strconv.ParseBool(raw)
		if err != nil {
			return RenderOptions{}, fmt.Errorf("invalid verbose value: %s", raw)
		}
		options.Verbose = verbose
	}
	return options, nil
}

func parseRepeatedValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			item := strings.TrimSpace(part)
			if item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

func isSupportedWebAPISection(value string) bool {
	switch strings.TrimSpace(value) {
	case "",
		"evidence",
		"files",
		"workflow",
		SectionAll,
		SectionOverview,
		SectionClaims,
		SectionChallenges,
		SectionVerifications,
		SectionObservations,
		SectionFollowups,
		SectionDebug,
		SectionArtifacts,
		SectionRounds,
		SectionVotes,
		SectionStatements,
		SectionConvergence:
		return true
	default:
		return false
	}
}

func expandWebSection(value string) []string {
	switch strings.TrimSpace(value) {
	case "", SectionAll:
		return []string{SectionAll}
	case SectionOverview:
		return []string{SectionOverview}
	case SectionClaims:
		return []string{SectionClaims}
	case "evidence":
		return []string{SectionChallenges, SectionVerifications, SectionArtifacts}
	case SectionObservations:
		return []string{SectionObservations}
	case SectionFollowups:
		return []string{SectionFollowups}
	case SectionDebug:
		return []string{SectionDebug}
	case "files":
		return []string{SectionArtifacts}
	case "workflow":
		return []string{SectionRounds, SectionVotes, SectionStatements, SectionConvergence}
	default:
		return []string{strings.TrimSpace(value)}
	}
}

func isSupportedWebClaimVerdict(value string) bool {
	switch strings.TrimSpace(value) {
	case "",
		string(consensus.ClaimVerdictSupported),
		string(consensus.ClaimVerdictRefuted),
		string(consensus.ClaimVerdictInsufficientEvidence),
		string(consensus.ClaimVerdictUndetermined):
		return true
	default:
		return false
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func renderWebPage(model WebPageModel) (string, error) {
	var b strings.Builder
	if err := webPageTemplate.Execute(&b, model); err != nil {
		return "", fmt.Errorf("render web page: %w", err)
	}
	return b.String(), nil
}

func slicesContainAny(values []string, targets ...string) bool {
	for _, value := range values {
		for _, target := range targets {
			if value == target {
				return true
			}
		}
	}
	return false
}
