package secrets

import (
	"regexp"
	"sort"
	"strings"
)

type Category string

const (
	CategoryProvider Category = "provider"
	CategoryAWS      Category = "aws"
	CategoryGitHub   Category = "github"
	CategoryStripe   Category = "stripe"
	CategorySlack    Category = "slack"
	CategoryGCP      Category = "gcp"
	CategoryPEM      Category = "pem"
)

type Match struct {
	Start         int
	End           int
	Category      Category
	IsProviderKey bool
}

type Detector struct {
	patterns []categoryPattern
}

type categoryPattern struct {
	re       *regexp.Regexp
	category Category
}

func NewDetector() *Detector {
	return &Detector{
		patterns: []categoryPattern{
			// Provider keys (hard-block tier). Specific prefixes first so
			// Anthropic/OpenRouter keys aren't classified as the generic OpenAI form.
			{
				regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{20,}`),
				CategoryProvider,
			},
			{regexp.MustCompile(`sk-or-[A-Za-z0-9_\-]{20,}`), CategoryProvider},
			{regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`), CategoryProvider},

			// AWS
			{regexp.MustCompile(`AKIA[A-Z0-9]{16}`), CategoryAWS},
			{
				regexp.MustCompile(`(?i)aws_secret_access_key\s*=\s*\S+`),
				CategoryAWS,
			},

			// GitHub
			{regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36}`), CategoryGitHub},
			{regexp.MustCompile(`github_pat_[A-Za-z0-9_]{82}`), CategoryGitHub},

			// Stripe
			{regexp.MustCompile(`sk_live_[A-Za-z0-9]{24,}`), CategoryStripe},
			{regexp.MustCompile(`pk_live_[A-Za-z0-9]{24,}`), CategoryStripe},
			{regexp.MustCompile(`sk_test_[A-Za-z0-9]{24,}`), CategoryStripe},
			{regexp.MustCompile(`pk_test_[A-Za-z0-9]{24,}`), CategoryStripe},

			// Slack
			{
				regexp.MustCompile(`xox[bpoars]-[A-Za-z0-9-]{10,}`),
				CategorySlack,
			},

			// PEM private key blocks (multi-line, BEGIN through END).
			{
				regexp.MustCompile(
					`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`,
				),
				CategoryPEM,
			},
		},
	}
}

// Detect scans content for known credential formats and returns every match
// with its byte range, category, and whether it is a flux-provider key
// (the hard-block tier). Matches fully contained within a longer match are
// removed; overlapping but non-contained matches are preserved.
func (d *Detector) Detect(content string) []Match {
	var matches []Match
	for _, p := range d.patterns {
		for _, idx := range p.re.FindAllStringIndex(content, -1) {
			matches = append(matches, Match{
				Start:         idx[0],
				End:           idx[1],
				Category:      p.category,
				IsProviderKey: p.category == CategoryProvider,
			})
		}
	}

	matches = append(matches, detectGCPServiceAccount(content)...)

	return dedupe(matches)
}

// detectGCPServiceAccount uses a structural heuristic: the input must contain
// both `"type": "service_account"` and a `"private_key": "..."` value pair.
// The match range covers the private_key value so the redactor scrubs the
// actual secret rather than the surrounding JSON metadata.
var (
	gcpTypeMarker = regexp.MustCompile(`"type"\s*:\s*"service_account"`)
	gcpPrivateKey = regexp.MustCompile(`(?s)"private_key"\s*:\s*"[^"]+"`)
)

func detectGCPServiceAccount(content string) []Match {
	if !gcpTypeMarker.MatchString(content) {
		return nil
	}
	if !strings.Contains(content, `"private_key"`) {
		return nil
	}
	var out []Match
	for _, idx := range gcpPrivateKey.FindAllStringIndex(content, -1) {
		out = append(out, Match{
			Start:    idx[0],
			End:      idx[1],
			Category: CategoryGCP,
		})
	}
	return out
}

func dedupe(matches []Match) []Match {
	if len(matches) == 0 {
		return matches
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Start != matches[j].Start {
			return matches[i].Start < matches[j].Start
		}
		return matches[i].End > matches[j].End
	})

	out := []Match{matches[0]}
	for _, m := range matches[1:] {
		last := out[len(out)-1]
		if m.Start >= last.Start && m.End <= last.End {
			continue
		}
		out = append(out, m)
	}
	return out
}
