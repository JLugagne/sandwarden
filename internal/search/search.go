package search

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/JLugagne/bm25"
	"github.com/JLugagne/sandwarden/internal/store"
)

// Kind identifies what an indexed document comes from: a fleet config entity
// (sandbox, profile, cache) or a discovered catalog item (skill, command, kit).
type Kind string

// Indexed document kinds.
const (
	KindSkill   Kind = "skill"
	KindCommand Kind = "command"
	KindKit     Kind = "kit"
	KindSandbox Kind = "sandbox"
	KindProfile Kind = "profile"
	KindCache   Kind = "cache"
)

// Document is one indexable entry: a discovered skill, command or kit, or a
// fleet sandbox, profile or cache. Body carries the text read from a checkout
// (SKILL.md, the command file or spec.yaml); it is indexed but never returned
// to the UI.
type Document struct {
	Kind        Kind
	Store       string
	StoreName   string
	Name        string
	DisplayName string
	// Slug is the config directory slug used to route config entities.
	// It is not indexed, so a renamed entity cannot resurface through a stale slug.
	Slug        string
	Description string
	Plugin      string
	KitKind     string
	// Agent is the base agent a sandbox runs, or a kit requires.
	Agent string
	// Refs carries the short references of a config entity: profile and cache
	// slugs, allow and deny patterns, host paths.
	Refs []string
	Body string
}

// Result is one ranked hit.
type Result struct {
	Kind        Kind   `json:"kind"`
	Store       string `json:"store"`
	StoreName   string `json:"store_name"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	// Slug routes config entity hits; it is not part of the searchable text.
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description"`
	Plugin      string `json:"plugin,omitempty"`
	KitKind     string `json:"kit_kind,omitempty"`
	// Agent is set on sandbox hits.
	Agent   string  `json:"agent,omitempty"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet,omitempty"`
}

// Okapi parameters and snippet limits.
const (
	minQueryRunes = 2
	okapiK1       = 1.5
	okapiB        = 0.75
	maxBodyRunes  = 20000
	snippetRunes  = 160
)

// Index is an immutable BM25 index over the discovered catalogs and the
// fleet config entities. It is safe for concurrent queries.
type Index struct {
	bm    *bm25.BM25Okapi
	docs  []Document
	vocab []string
}

// Build collects the discovered catalogs, appends the caller's fleet config
// documents and indexes them. storePaths maps "skill:<slug>" and
// "kit:<slug>" to the store checkout directories.
func Build(ctx context.Context, st *store.Store, storePaths map[string]string, config []Document) (*Index, error) {
	docs, err := Collect(ctx, st, storePaths)
	if err != nil {
		return nil, err
	}
	docs = append(docs, config...)
	return New(docs)
}

// New builds an index over the given documents. Documents whose text yields
// no token are skipped.
func New(docs []Document) (*Index, error) {
	corpus := make([]string, 0, len(docs))
	indexed := make([]Document, 0, len(docs))
	vocabulary := make(map[string]struct{})
	for _, doc := range docs {
		text := doc.searchText()
		tokens := Tokenize(text)
		if len(tokens) == 0 {
			continue
		}
		corpus = append(corpus, text)
		indexed = append(indexed, doc)
		for _, token := range tokens {
			vocabulary[token] = struct{}{}
		}
	}
	if len(corpus) == 0 {
		return &Index{}, nil
	}
	engine, err := bm25.NewBM25Okapi(corpus, Tokenize, okapiK1, okapiB, nil)
	if err != nil {
		return nil, err
	}
	vocab := make([]string, 0, len(vocabulary))
	for token := range vocabulary {
		vocab = append(vocab, token)
	}
	sort.Strings(vocab)
	return &Index{bm: engine, docs: indexed, vocab: vocab}, nil
}

// Search returns the documents that match query, best first. Terms match
// whole words and, from two characters on, word prefixes, so type-ahead
// ("kub") already finds "kubernetes". A blank or one-character query returns
// no results.
func (ix *Index) Search(query string, limit int) []Result {
	if ix == nil || ix.bm == nil {
		return nil
	}
	if utf8.RuneCountInString(strings.TrimSpace(query)) < minQueryRunes {
		return nil
	}
	terms := ix.expand(Tokenize(query))
	if len(terms) == 0 {
		return nil
	}
	scores, err := ix.bm.GetScores(terms)
	if err != nil {
		return nil
	}
	ranked := make([]int, 0, len(scores))
	for i, score := range scores {
		if score > 0 {
			ranked = append(ranked, i)
		}
	}
	if len(ranked) == 0 {
		return ix.fallback(terms, limit)
	}
	sort.SliceStable(ranked, func(i, j int) bool { return scores[ranked[i]] > scores[ranked[j]] })
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	results := make([]Result, 0, len(ranked))
	for _, i := range ranked {
		results = append(results, ix.result(i, scores[i], terms))
	}
	return results
}

// expand adds every indexed word that extends a query term, so partial words
// search like type-ahead. Terms shorter than two characters match exactly.
func (ix *Index) expand(terms []string) []string {
	seen := make(map[string]struct{}, len(terms))
	expanded := make([]string, 0, len(terms))
	add := func(term string) {
		if _, ok := seen[term]; ok {
			return
		}
		seen[term] = struct{}{}
		expanded = append(expanded, term)
	}
	for _, term := range terms {
		add(term)
		if utf8.RuneCountInString(term) < 2 {
			continue
		}
		for _, word := range ix.vocab {
			if len(word) > len(term) && strings.HasPrefix(word, term) {
				add(word)
			}
		}
	}
	return expanded
}

// searchText is the indexed representation of one document. The name,
// display name and store are repeated so a metadata match outranks a
// body-only match; the agent and references of config entities are indexed
// next to the description and the body.
func (d Document) searchText() string {
	var builder strings.Builder
	boost := strings.TrimSpace(strings.Join([]string{d.Name, d.DisplayName, d.StoreName}, "\n"))
	for range 3 {
		builder.WriteString(boost)
		builder.WriteString("\n")
	}
	builder.WriteString(d.Description)
	builder.WriteString("\n")
	builder.WriteString(d.Agent)
	builder.WriteString("\n")
	builder.WriteString(d.Plugin)
	builder.WriteString("\n")
	builder.WriteString(d.refsText())
	builder.WriteString("\n")
	builder.WriteString(d.Body)
	return builder.String()
}

// refsText joins the document references into one indexable block, bounded
// like a body so a long rule list cannot bloat the index.
func (d Document) refsText() string {
	if len(d.Refs) == 0 {
		return ""
	}
	return truncateRunes(strings.Join(d.Refs, "\n"), maxBodyRunes)
}

// snippet returns a short window of the document text around the first
// matched term, or an empty string when the match is neither in the body nor
// in the references. The window is bounded, so a long value is never dumped.
func (d Document) snippet(terms []string) string {
	text := strings.TrimSpace(d.Body + "\n" + d.refsText())
	body := strings.Join(strings.Fields(text), " ")
	if body == "" {
		return ""
	}
	runes := []rune(body)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	haystack := string(lower)
	best := -1
	for _, term := range terms {
		index := strings.Index(haystack, term)
		if index < 0 {
			continue
		}
		position := utf8.RuneCountInString(haystack[:index])
		if best < 0 || position < best {
			best = position
		}
	}
	if best < 0 {
		return ""
	}
	start := best - snippetRunes/3
	if start < 0 {
		start = 0
	}
	end := start + snippetRunes
	if end > len(runes) {
		end = len(runes)
	}
	window := string(runes[start:end])
	if start > 0 {
		window = "…" + window
	}
	if end < len(runes) {
		window += "…"
	}
	return strings.TrimSpace(window)
}

// Collect lists every skill, command and kit with the body read from its
// checkout. Missing or unreadable files only cost the body.
func Collect(ctx context.Context, st *store.Store, storePaths map[string]string) ([]Document, error) {
	skills, err := st.ListAllSkillItems(ctx)
	if err != nil {
		return nil, err
	}
	kits, err := st.ListAllKitItems(ctx)
	if err != nil {
		return nil, err
	}
	docs := make([]Document, 0, len(skills)+len(kits))
	for _, item := range skills {
		root := storePaths["skill:"+item.Store]
		docs = append(docs, Document{
			Kind:        Kind(item.Kind),
			Store:       item.Store,
			Name:        item.Name,
			Description: item.Description,
			Plugin:      item.Plugin,
			Body:        skillBody(root, item.RelPath, item.Kind),
		})
	}
	for _, item := range kits {
		root := storePaths["kit:"+item.Store]
		docs = append(docs, Document{
			Kind:        KindKit,
			Store:       item.Store,
			Name:        item.Name,
			DisplayName: item.DisplayName,
			Description: item.Description,
			KitKind:     item.Kind,
			Body:        readBody(filepath.Join(root, filepath.FromSlash(item.RelPath), "spec.yaml")),
		})
	}
	return docs, nil
}

// skillBody reads the markdown of one skill or command. Skill items point at
// their directory (SKILL.md inside), commands at the markdown file itself.
func skillBody(root, relPath, kind string) string {
	if root == "" || relPath == "" {
		return ""
	}
	path := filepath.Join(root, filepath.FromSlash(relPath))
	if kind == string(KindSkill) {
		path = filepath.Join(path, "SKILL.md")
	}
	return readBody(path)
}

// readBody reads a checkout file, truncating it to maxBodyRunes and dropping
// invalid UTF-8.
func readBody(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return truncateRunes(strings.ToValidUTF8(string(data), ""), maxBodyRunes)
}

// Tokenize lowercases text and splits it on every non letter or digit rune.
func Tokenize(text string) []string {
	tokens := make([]string, 0, 32)
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(string(current)))
		current = current[:0]
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

// result builds the UI payload for one indexed document.
func (ix *Index) result(index int, score float64, terms []string) Result {
	doc := ix.docs[index]
	return Result{
		Kind:        doc.Kind,
		Store:       doc.Store,
		StoreName:   doc.StoreName,
		Name:        doc.Name,
		DisplayName: doc.DisplayName,
		Slug:        doc.Slug,
		Description: doc.Description,
		Plugin:      doc.Plugin,
		KitKind:     doc.KitKind,
		Agent:       doc.Agent,
		Score:       score,
		Snippet:     doc.snippet(terms),
	}
}

// fallback matches raw substrings when BM25 has nothing to rank. Okapi drops
// terms whose IDF is zero, so a query made only of terms present in every
// document (a one-item catalog, for example) would otherwise return nothing.
func (ix *Index) fallback(terms []string, limit int) []Result {
	type hit struct {
		index int
		score int
	}
	hits := make([]hit, 0, len(ix.docs))
	for i, doc := range ix.docs {
		text := strings.ToLower(doc.searchText())
		score := 0
		for _, term := range terms {
			if strings.Contains(text, term) {
				score++
			}
		}
		if score > 0 {
			hits = append(hits, hit{index: i, score: score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].index < hits[j].index
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	results := make([]Result, 0, len(hits))
	for _, h := range hits {
		results = append(results, ix.result(h.index, float64(h.score), terms))
	}
	return results
}

// Fingerprint digests the indexed fields of documents so callers rebuild a
// cached index only when the documents actually changed. The body is left out
// on purpose: config documents are built from the in-memory fleet and carry
// no body, and catalog bodies are covered by the store fingerprint.
func Fingerprint(docs []Document) string {
	h := fnv.New64a()
	for _, doc := range docs {
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00",
			doc.Kind, doc.Name, doc.DisplayName, doc.Slug, doc.Description, doc.Agent, strings.Join(doc.Refs, "\x1f"))
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// truncateRunes cuts text down to its first limit runes.
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
